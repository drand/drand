package beacon

import (
	"sync"

	"github.com/drand/drand/chain"
)

// CallbackStore is an interface that allows to register callbacks that gets
// called each time a new beacon is inserted
type CallbackStore interface {
	chain.Store
	AddCallback(id string, fn func(*chain.Beacon))
	RemoveCallback(id string)
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
