package boltdb

import (
	"errors"
	"io"
	"path"
	"sync"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
	bolt "go.etcd.io/bbolt"
)

// boldStore implements the Store interface using the kv storage boltdb (native
// golang implementation). Internally, Beacons are stored as JSON-encoded in the
// db file.
type boltStore struct {
	sync.Mutex
	db *bolt.DB
}

var beaconBucket = []byte("beacons")

// BoltFileName is the name of the file boltdb writes to
const BoltFileName = "drand.db"

// BoltStoreOpenPerm is the permission we will use to read bolt store file from disk
const BoltStoreOpenPerm = 0660

// NewBoltStore returns a Store implementation using the boltdb storage engine.
func NewBoltStore(folder string, opts *bolt.Options) (chain.Store, error) {
	dbPath := path.Join(folder, BoltFileName)
	db, err := bolt.Open(dbPath, BoltStoreOpenPerm, opts)
	if err != nil {
		return nil, err
	}
	// create the bucket already
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(beaconBucket)
		if err != nil {
			return err
		}
		return nil
	})

	return &boltStore{
		db: db,
	}, err
}

func (b *boltStore) Len() int {
	var length = 0
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		length = bucket.Stats().KeyN
		return nil
	})
	if err != nil {
		log.DefaultLogger().Warnw("", "boltdb", "error getting length", "err", err)
	}
	return length
}

func (b *boltStore) Close() {
	if err := b.db.Close(); err != nil {
		log.DefaultLogger().Debugw("", "boltdb", "close", "err", err)
	}
}

// Put implements the Store interface. WARNING: It does NOT verify that this
// beacon is not already saved in the database or not and will overwrite it.
func (b *boltStore) Put(beacon *chain.Beacon) error {
	err := b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		key := chain.RoundToBytes(beacon.Round)
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

// ForcePut implements the Store interface. WARNING: It does NOT verify that this
// beacon is not already saved in the database or not and will overwrite it.
func (b *boltStore) ForcePut(beacon *chain.Beacon) error {
	return b.Put(beacon)
}

// ErrNoBeaconSaved is the error returned when no beacon have been saved in the
// database yet.
var ErrNoBeaconSaved = errors.New("beacon not found in database")

// Last returns the last beacon signature saved into the db
func (b *boltStore) Last() (*chain.Beacon, error) {
	var beacon *chain.Beacon
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		cursor := bucket.Cursor()
		_, v := cursor.Last()
		if v == nil {
			return ErrNoBeaconSaved
		}
		b := &chain.Beacon{}
		if err := b.Unmarshal(v); err != nil {
			return err
		}
		beacon = b
		return nil
	})
	return beacon, err
}

// Get returns the beacon saved at this round
func (b *boltStore) Get(round uint64) (*chain.Beacon, error) {
	var beacon *chain.Beacon
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		v := bucket.Get(chain.RoundToBytes(round))
		if v == nil {
			return ErrNoBeaconSaved
		}
		b := &chain.Beacon{}
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
		return bucket.Delete(chain.RoundToBytes(round))
	})
}

func (b *boltStore) Cursor(fn func(chain.Cursor)) {
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		c := bucket.Cursor()
		fn(&boltCursor{Cursor: c})
		return nil
	})
	if err != nil {
		log.DefaultLogger().Warnw("", "boltdb", "error getting cursor", "err", err)
	}
}

// SaveTo saves the bolt database to an alternate file.
func (b *boltStore) SaveTo(w io.Writer) error {
	return b.db.View(func(tx *bolt.Tx) error {
		_, err := tx.WriteTo(w)
		return err
	})
}

type boltCursor struct {
	*bolt.Cursor
}

func (c *boltCursor) First() *chain.Beacon {
	k, v := c.Cursor.First()
	if k == nil {
		return nil
	}
	b := new(chain.Beacon)
	if err := b.Unmarshal(v); err != nil {
		return nil
	}
	return b
}

func (c *boltCursor) Next() *chain.Beacon {
	k, v := c.Cursor.Next()
	if k == nil {
		return nil
	}
	b := new(chain.Beacon)
	if err := b.Unmarshal(v); err != nil {
		return nil
	}
	return b
}

func (c *boltCursor) Seek(round uint64) *chain.Beacon {
	k, v := c.Cursor.Seek(chain.RoundToBytes(round))
	if k == nil {
		return nil
	}
	b := new(chain.Beacon)
	if err := b.Unmarshal(v); err != nil {
		return nil
	}
	return b
}

func (c *boltCursor) Last() *chain.Beacon {
	k, v := c.Cursor.Last()
	if k == nil {
		return nil
	}
	b := new(chain.Beacon)
	if err := b.Unmarshal(v); err != nil {
		return nil
	}
	return b
}
