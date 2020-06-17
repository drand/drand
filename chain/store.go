package chain

import (
	"bytes"
	"encoding/binary"
	"sync"
)

// store contains all the definitions and implementation of the logic that
// stores and loads beacon signatures. At the moment of writing, it consists of
// a boltdb key/value database store.

// Store is an interface to store Beacons packets where they can also be
// retrieved to be delivered to end clients.
type Store interface {
	Len() int
	Put(*Beacon) error
	Last() (*Beacon, error)
	Get(round uint64) (*Beacon, error)
	Cursor(func(Cursor))
	Close()
	Del(round uint64) error
}

// Cursor iterates over items in sorted key order. This starts from the
// first key/value pair and updates the k/v variables to the
// next key/value on each iteration.
//
// The loop finishes at the end of the cursor when a nil key is returned.
//    for k, v := c.First(); k != nil; k, v = c.Next() {
//        fmt.Printf("A %s is %s.\n", k, v)
//    }
type Cursor interface {
	First() *Beacon
	Next() *Beacon
	Seek(round uint64) *Beacon
	Last() *Beacon
}

// CallbackStore keeps a list of functions to notify on new beacons
type CallbackStore struct {
	Store
	cbs []func(*Beacon)
	sync.Mutex
}

// NewCallbackStore returns a Store that calls the given callback in a goroutine
// each time a new Beacon is saved into the given store. It does not call the
// callback if there has been any errors while saving the beacon.
func NewCallbackStore(s Store) *CallbackStore {
	return &CallbackStore{Store: s}
}

// Put stores a new beacon
func (c *CallbackStore) Put(b *Beacon) error {
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
func (c *CallbackStore) AddCallback(fn func(*Beacon)) {
	c.Lock()
	defer c.Unlock()
	c.cbs = append(c.cbs, fn)
}

// RoundToBytes provides a byte serialized form of a round number
func RoundToBytes(r uint64) []byte {
	var buff bytes.Buffer
	_ = binary.Write(&buff, binary.BigEndian, r)
	return buff.Bytes()
}
