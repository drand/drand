package beacon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/drand/drand/crypto"
	"sync"
	"time"

	clock "github.com/jonboulle/clockwork"

	"github.com/drand/drand/common"
	"github.com/drand/drand/common/key"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/internal/chain"
	dcontext "github.com/drand/drand/internal/context"
	"github.com/drand/drand/internal/metrics"
)

// CallbackFunc defines the callback type that's accepted by CallbackStore
type CallbackFunc func(b *common.Beacon, closed bool)

// CallbackStore is an interface that allows to register callbacks that gets
// called each time a new beacon is inserted
type CallbackStore interface {
	chain.Store
	AddCallback(id string, fn CallbackFunc)
	RemoveCallback(id string)
}

// appendStore is a store that only appends new block with a round +1 from the
// last block inserted and with the corresponding previous signature
type appendStore struct {
	chain.Store
	last *common.Beacon
	sync.Mutex
}

func newAppendStore(ctx context.Context, s chain.Store) (chain.Store, error) {
	last, err := s.Last(ctx)
	if err != nil {
		return nil, err
	}

	return &appendStore{
		Store: s,
		last:  last,
	}, nil
}

// ErrBeaconAlreadyStored is returned when we already have the value in the store
var ErrBeaconAlreadyStored = errors.New("beacon value already stored")

func (a *appendStore) Put(ctx context.Context, b *common.Beacon) error {
	ctx, span := metrics.NewSpan(ctx, "appendStore.Put")
	defer span.End()

	a.Lock()
	defer a.Unlock()

	if b.Round == a.last.Round {
		if bytes.Equal(a.last.Signature, b.Signature) {
			if bytes.Equal(a.last.PreviousSig, b.PreviousSig) {
				return fmt.Errorf("%w round %d", ErrBeaconAlreadyStored, b.Round)
			}

			return fmt.Errorf(
				"tried to store a duplicate beacon for round %d but the previous signature %#v was different %#v",
				b.Round, a.last.PreviousSig, b.PreviousSig)
		}
		return fmt.Errorf(
			"tried to store a duplicate beacon for round %d but the signature %#v was different %#v",
			b.Round, a.last.Signature, b.Signature)
	}

	if b.Round != a.last.Round+1 {
		return fmt.Errorf("invalid round inserted: last %d, new %d", a.last.Round, b.Round)
	}
	if err := a.Store.Put(ctx, b); err != nil {
		return err
	}
	a.last = b
	return nil
}

// schemeStore is a store that run different checks depending on what scheme is being used.
type schemeStore struct {
	chain.Store
	sch       *crypto.Scheme
	last      *common.Beacon
	isChained bool
	sync.Mutex
}

func NewSchemeStore(ctx context.Context, s chain.Store, sch *crypto.Scheme) (chain.Store, error) {
	last, err := s.Last(ctx)
	if err != nil {
		return nil, err
	}
	return &schemeStore{
		Store:     s,
		last:      last,
		sch:       sch,
		isChained: sch.Name == crypto.DefaultSchemeID,
	}, nil
}

func (a *schemeStore) Put(ctx context.Context, b *common.Beacon) error {
	ctx, span := metrics.NewSpan(ctx, "schemeStore.Put")
	defer span.End()
	a.Lock()
	defer a.Unlock()

	// If the scheme is unchained, previous signature is set to nil. In that case,
	// relationship between signature in the previous beacon and previous signature
	// on the actual beacon is not necessary. Otherwise, it will be checked.
	if a.isChained {
		// in chained mode it should keep the consistency between prev signature and signature
		if !bytes.Equal(a.last.Signature, b.PreviousSig) {
			return fmt.Errorf("invalid previous signature for %d: %x != %x",
				b.Round,
				a.last.Signature,
				b.PreviousSig)
		}
	} else {
		// we're in unchained mode, we don't need the previous signature
		b.PreviousSig = nil
	}

	if err := a.Store.Put(ctx, b); err != nil {
		return err
	}
	// we update the last beacon as being this one. Note that this kinda assume we're operating on an appendStore...
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

func (d *discrepancyStore) Put(ctx context.Context, b *common.Beacon) error {
	ctx, span := metrics.NewSpan(ctx, "discrepancyStore.Put")
	defer span.End()

	// When computing time_discrepancy, time.Now() should be obtained as close as
	// possible to receiving the beacon, before any other storage layer interaction.
	// When moved after store.Put(), the value will include the time it takes
	// the storage layer to store the value, making it inaccurate.
	actual := d.clock.Now()

	if err := d.Store.Put(ctx, b); err != nil {
		return err
	}

	storageTime := d.clock.Now()

	expected := chain.TimeOfRound(d.group.Period, d.group.GenesisTime, b.Round) * 1e9
	discrepancy := float64(actual.UnixNano()-expected) / float64(time.Millisecond)

	beaconID := common.GetCanonicalBeaconID(d.group.ID)
	metrics.BeaconDiscrepancyLatency.WithLabelValues(beaconID).Set(discrepancy)
	metrics.LastBeaconRound.WithLabelValues(beaconID).Set(float64(b.GetRound()))
	metrics.GroupSize.WithLabelValues(beaconID).Set(float64(d.group.Len()))
	metrics.GroupThreshold.WithLabelValues(beaconID).Set(float64(d.group.Threshold))
	// in order to avoid spamming the logs, e.g. during syncing
	if !dcontext.IsSkipLogsFromContext(ctx) {
		d.l.Infow("",
			"NEW_BEACON_STORED", b.String(),
			"time_discrepancy_ms", discrepancy,
			"storage_time_ms", storageTime.Sub(actual).Milliseconds(),
		)
	}
	return nil
}

// callbackStores keeps a list of functions to notify on new beacons
type callbackStore struct {
	chain.Store
	sync.RWMutex
	l         log.Logger
	stopping  chan bool
	callbacks map[string]CallbackFunc
	newJob    map[string]chan cbPair
}

type cbPair struct {
	cb    CallbackFunc
	b     *common.Beacon
	close bool
}

// NewCallbackStore returns a Store that uses a pool of worker to dispatch the
// beacon to the registered callbacks. The callbacks are not called if the "Put"
// operations failed.
func NewCallbackStore(l log.Logger, s chain.Store) CallbackStore {
	cbs := &callbackStore{
		Store:     s,
		l:         l,
		callbacks: make(map[string]CallbackFunc),
		newJob:    make(map[string]chan cbPair),
		stopping:  make(chan bool, 1),
	}
	return cbs
}

// Put stores a new beacon
func (c *callbackStore) Put(ctx context.Context, b *common.Beacon) error {
	ctx, span := metrics.NewSpan(ctx, "callbackStore.Put")
	defer span.End()

	if err := c.Store.Put(ctx, b); err != nil {
		return err
	}

	if b.Round != 0 {
		c.RLock()
		defer c.RUnlock()
		for id, cb := range c.callbacks {
			j, ok := c.newJob[id]
			if !ok {
				continue
			}

			j <- cbPair{
				cb: cb,
				b:  b,
			}
		}
	}
	return nil
}

// AddCallback registers a function to call
func (c *callbackStore) AddCallback(id string, fn CallbackFunc) {
	c.Lock()
	defer c.Unlock()
	if jobChan, exists := c.newJob[id]; exists {
		c.l.Debugw("removing existing call back", "id", id, "reason", "to add a new one")
		jobChan <- cbPair{
			cb:    c.callbacks[id],
			b:     nil,
			close: true, // Signal we close this job
		}
		close(jobChan)
		delete(c.newJob, id)
	}

	c.l.Debugw("adding callback", "id", id)

	c.callbacks[id] = fn
	c.newJob[id] = make(chan cbPair, CallbackWorkerQueue)
	go c.runWorker(c.newJob[id])
}

func (c *callbackStore) RemoveCallback(id string) {
	c.Lock()
	defer c.Unlock()
	delete(c.callbacks, id)
	if _, exists := c.newJob[id]; exists {
		c.l.Debugw("removing existing call back", "id", id, "reason", "called RemoveCallback")
		close(c.newJob[id])
		delete(c.newJob, id)
	}
}

func (c *callbackStore) Close() error {
	close(c.stopping)
	return c.Store.Close()
}

func (c *callbackStore) runWorker(jobChan chan cbPair) {
	for {
		select {
		case <-c.stopping:
			return
		case newJob, ok := <-jobChan:
			if !ok {
				return
			}
			newJob.cb(newJob.b, newJob.close)
		}
	}
}
