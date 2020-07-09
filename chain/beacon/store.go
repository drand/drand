package beacon

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/drand/drand/chain"
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

var errPreviousRound = errors.New("round already inserted")

func (a *appendStore) Put(b *chain.Beacon) error {
	a.Lock()
	defer a.Unlock()
	if b.Round < a.last.Round+1 {
		return errPreviousRound
	} else if b.Round != a.last.Round+1 {
		return fmt.Errorf("invalid round inserted: last %d, new %d", a.last.Round, b.Round)
	}
	if !bytes.Equal(a.last.Signature, b.PreviousSig) {
		return fmt.Errorf("invalid previous signature")
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
}

func newDiscrepancyStore(s chain.Store, l log.Logger, group *key.Group) chain.Store {
	return &discrepancyStore{
		Store: s,
		l:     l,
		group: group,
	}
}

func (d *discrepancyStore) Put(b *chain.Beacon) error {
	if err := d.Store.Put(b); err != nil {
		return err
	}
	actual := time.Now().UnixNano()
	expected := chain.TimeOfRound(d.group.Period, d.group.GenesisTime, b.Round) * 1e9
	discrepancy := float64(actual-expected) / float64(time.Millisecond)
	metrics.BeaconDiscrepancyLatency.Set(float64(actual-expected) / float64(time.Millisecond))
	d.l.Info("NEW_BEACON_STORED", b.String(), "time_discrepancy_ms", discrepancy)
	return nil
}

// callbackStores keeps a list of functions to notify on new beacons
type callbackStore struct {
	chain.Store
	cbs map[string]func(*chain.Beacon)
	sync.Mutex
}

// NewCallbackStore returns a Store that calls the given callback in a goroutine
// each time a new Beacon is saved into the given store. It does not call the
// callback if there has been any errors while saving the beacon.
func NewCallbackStore(s chain.Store) CallbackStore {
	return &callbackStore{
		Store: s,
		cbs:   make(map[string]func(*chain.Beacon)),
	}
}

// Put stores a new beacon
func (c *callbackStore) Put(b *chain.Beacon) error {
	if err := c.Store.Put(b); err != nil {
		return err
	}
	if b.Round != 0 {
		go func() {
			c.Lock()
			defer c.Unlock()
			for _, cb := range c.cbs {
				cb(b)
			}
		}()
	}
	return nil
}

// AddCallback registers a function to call
func (c *callbackStore) AddCallback(id string, fn func(*chain.Beacon)) {
	c.Lock()
	defer c.Unlock()
	c.cbs[id] = fn
}

func (c *callbackStore) RemoveCallback(id string) {
	c.Lock()
	defer c.Unlock()
	delete(c.cbs, id)
}
