package beacon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/nikkolasg/slog"
	bolt "go.etcd.io/bbolt"
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

// Iterate over items in sorted key order. This starts from the
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

// boldStore implements the Store interface using the kv storage boltdb (native
// golang implementation). Internally, Beacons are stored as JSON-encoded in the
// db file.
type boltStore struct {
	sync.Mutex
	db  *bolt.DB
	len int
}

var beaconBucket = []byte("beacons")

// BoltFileName is the name of the file boltdb writes to
const BoltFileName = "drand.db"

// NewBoltStore returns a Store implementation using the boltdb storage engine.
func NewBoltStore(folder string, opts *bolt.Options) (Store, error) {
	dbPath := path.Join(folder, BoltFileName)
	db, err := bolt.Open(dbPath, 0660, opts)
	if err != nil {
		return nil, err
	}
	var baseLen = 0
	// create the bucket already
	err = db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(beaconBucket)
		if err != nil {
			return err
		}
		baseLen += bucket.Stats().KeyN
		return nil
	})

	return &boltStore{
		db:  db,
		len: baseLen,
	}, err
}

func (b *boltStore) Len() int {
	var length = 0
	b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		length = bucket.Stats().KeyN
		return nil
	})
	return length
}

func (b *boltStore) Close() {
	if err := b.db.Close(); err != nil {
		slog.Debugf("boltdb store: %s", err)
	}
}

// Put implements the Store interface. WARNING: It does NOT verify that this
// beacon is not already saved in the database or not.
func (b *boltStore) Put(beacon *Beacon) error {
	err := b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		key := roundToBytes(beacon.Round)
		buff, err := beacon.Marshal()
		if err != nil {
			return err
		}
		return bucket.Put(key, buff)
	})
	if err != nil {
		return err
	}
	return nil
}

// ErrNoBeaconSaved is the error returned when no beacon have been saved in the
// database yet.
var ErrNoBeaconSaved = errors.New("beacon not found in database")

// Last returns the last beacon signature saved into the db
func (b *boltStore) Last() (*Beacon, error) {
	var beacon *Beacon
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		cursor := bucket.Cursor()
		_, v := cursor.Last()
		if v == nil {
			return ErrNoBeaconSaved
		}
		b := &Beacon{}
		if err := b.Unmarshal(v); err != nil {
			return err
		}
		beacon = b
		return nil
	})
	return beacon, err
}

// Get returns the beacon saved at this round
func (b *boltStore) Get(round uint64) (*Beacon, error) {
	var beacon *Beacon
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		v := bucket.Get(roundToBytes(round))
		if v == nil {
			return ErrNoBeaconSaved
		}
		b := &Beacon{}
		if err := b.Unmarshal(v); err != nil {
			return err
		}
		beacon = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	return beacon, err
}

func (b *boltStore) Del(round uint64) error {
	return b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		return bucket.Delete(roundToBytes(round))
	})
}

func (b *boltStore) Cursor(fn func(Cursor)) {
	b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		c := bucket.Cursor()
		fn(&boltCursor{Cursor: c})
		return nil
	})
}

type boltCursor struct {
	*bolt.Cursor
}

func (c *boltCursor) First() *Beacon {
	k, v := c.Cursor.First()
	if k == nil {
		return nil
	}
	b := new(Beacon)
	if err := b.Unmarshal(v); err != nil {
		return nil
	}
	return b
}

func (c *boltCursor) Next() *Beacon {
	k, v := c.Cursor.Next()
	if k == nil {
		return nil
	}
	b := new(Beacon)
	if err := b.Unmarshal(v); err != nil {
		return nil
	}
	return b
}

func (c *boltCursor) Seek(round uint64) *Beacon {
	k, v := c.Cursor.Seek(roundToBytes(round))
	if k == nil {
		return nil
	}
	b := new(Beacon)
	if err := b.Unmarshal(v); err != nil {
		return nil
	}
	return b
}

func (c *boltCursor) Last() *Beacon {
	k, v := c.Cursor.Last()
	if k == nil {
		return nil
	}
	b := new(Beacon)
	if err := b.Unmarshal(v); err != nil {
		return nil
	}
	return b
}

// CallbackStore is a store that calls a registered list of methods each time
// a new beacon is added.
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

func (c *CallbackStore) AddCallback(fn func(*Beacon)) {
	c.Lock()
	defer c.Unlock()
	c.cbs = append(c.cbs, fn)
}

func roundToBytes(r uint64) []byte {
	var buff bytes.Buffer
	binary.Write(&buff, binary.BigEndian, r)
	return buff.Bytes()
}

func printStore(s Store) string {
	time.Sleep(1 * time.Second)
	var out = ""
	s.Cursor(func(c Cursor) {
		for b := c.First(); b != nil; b = c.Next() {
			out += fmt.Sprintf("%s\n", b)
		}
	})
	return out
}
