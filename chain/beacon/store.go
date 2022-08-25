package beacon

import (
	"bytes"
	"fmt"
	"runtime"
	"sync"
	"time"

	clock "github.com/jonboulle/clockwork"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/common"
	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
)

// CallbackStore is an interface that allows to register callbacks that gets
// called each time a new beacon is inserted
type CallbackStore interface {
	chain.Store
	AddCallback(id string, fn func(*chain.Beacon))
	RemoveCallback(id string)
}

// appendStore is a store that only appends new block with a round +1 from the
// last block inserted and with the corresponding previous signature
type appendStore struct {
	chain.Store
	last *chain.Beacon
	sync.Mutex
}

func newAppendStore(s chain.Store) chain.Store {
	last, _ := s.Last()
	return &appendStore{
		Store: s,
		last:  last,
	}
}

func (a *appendStore) Put(b *chain.Beacon) error {
	a.Lock()
	defer a.Unlock()
	if b.Round != a.last.Round+1 {
		return fmt.Errorf("invalid round inserted: last %d, new %d", a.last.Round, b.Round)
	}
	if err := a.Store.Put(b); err != nil {
		return err
	}
	a.last = b
	return nil
}

// schemeStore is a store that run different checks depending on what scheme is being used.
type schemeStore struct {
	chain.Store
	sch  scheme.Scheme
	last *chain.Beacon
	sync.Mutex
}

func NewSchemeStore(s chain.Store, sch scheme.Scheme) chain.Store {
	last, _ := s.Last()
	return &schemeStore{
		Store: s,
		last:  last,
		sch:   sch,
	}
}

func (a *schemeStore) Put(b *chain.Beacon) error {
	a.Lock()
	defer a.Unlock()

	// If the scheme is unchained, previous signature is set to nil. In that case,
	// relationship between signature in the previous beacon and previous signature
	// on the actual beacon is not necessary. Otherwise, it will be checked.
	if a.sch.DecouplePrevSig {
		b.PreviousSig = nil
	} else if !bytes.Equal(a.last.Signature, b.PreviousSig) {
		if pb, err := a.Get(b.Round - 1); err != nil || !bytes.Equal(pb.Signature, b.PreviousSig) {
			return fmt.Errorf("invalid previous signature for %d or "+
				"previous beacon not found in database. Err: %w", b.Round, err)
		}
	}

	if err := a.Store.Put(b); err != nil {
		return err
	}

	a.last = b
	return nil
}

// discrepancyStore is used to log timing information about the rounds
type discrepancyStore struct {
	chain.Store
	l     log.Logger
	group *key.Group
	clock clock.Clock
}

func newDiscrepancyStore(s chain.Store, l log.Logger, group *key.Group, cl clock.Clock) chain.Store {
	return &discrepancyStore{
		Store: s,
		l:     l,
		group: group,
		clock: cl,
	}
}

func (d *discrepancyStore) Put(b *chain.Beacon) error {
	if err := d.Store.Put(b); err != nil {
		return err
	}

	beaconID := common.GetCanonicalBeaconID(d.group.ID)

	actual := d.clock.Now().UnixNano()
	expected := chain.TimeOfRound(d.group.Period, d.group.GenesisTime, b.Round) * 1e9
	discrepancy := float64(actual-expected) / float64(time.Millisecond)

	metrics.BeaconDiscrepancyLatency.WithLabelValues(beaconID).Set(float64(actual-expected) / float64(time.Millisecond))
	metrics.LastBeaconRound.WithLabelValues(beaconID).Set(float64(b.GetRound()))
	metrics.GroupSize.WithLabelValues(beaconID).Set(float64(d.group.Len()))
	metrics.GroupThreshold.WithLabelValues(beaconID).Set(float64(d.group.Threshold))
	d.l.Infow("", "NEW_BEACON_STORED", b.String(), "time_discrepancy_ms", discrepancy)
	return nil
}

// callbackStores keeps a list of functions to notify on new beacons
type callbackStore struct {
	chain.Store
	sync.Mutex
	done      chan bool
	callbacks map[string]func(*chain.Beacon)
	newJob    chan cbPair
}

type cbPair struct {
	cb func(*chain.Beacon)
	b  *chain.Beacon
}

// NewCallbackStore returns a Store that uses a pool of worker to dispatch the
// beacon to the registered callbacks. The callbacks are not called if the "Put"
// operations failed.
func NewCallbackStore(s chain.Store) CallbackStore {
	cbs := &callbackStore{
		Store:     s,
		callbacks: make(map[string]func(*chain.Beacon)),
		newJob:    make(chan cbPair, CallbackWorkerQueue),
		done:      make(chan bool, 1),
	}
	cbs.runWorkers(runtime.NumCPU())
	return cbs
}

// Put stores a new beacon
func (c *callbackStore) Put(b *chain.Beacon) error {
	if err := c.Store.Put(b); err != nil {
		return err
	}
	if b.Round != 0 {
		c.Lock()
		defer c.Unlock()
		for _, cb := range c.callbacks {
			c.newJob <- cbPair{
				cb: cb,
				b:  b,
			}
		}
	}
	return nil
}

// AddCallback registers a function to call
func (c *callbackStore) AddCallback(id string, fn func(*chain.Beacon)) {
	c.Lock()
	defer c.Unlock()
	c.callbacks[id] = fn
}

func (c *callbackStore) RemoveCallback(id string) {
	c.Lock()
	defer c.Unlock()
	delete(c.callbacks, id)
}

func (c *callbackStore) Close() {
	c.Store.Close()
	close(c.done)
}

func (c *callbackStore) runWorkers(n int) {
	for i := 0; i < n; i++ {
		go c.runWorker()
	}
}

func (c *callbackStore) runWorker() {
	for {
		select {
		case newJob := <-c.newJob:
			newJob.cb(newJob.b)
		case <-c.done:
			return
		}
	}
}
