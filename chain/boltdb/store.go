package boltdb

import (
	"context"
	goErrors "errors"
	"io"
	"math"
	"math/bits"
	"path"
	"sync"

	bolt "go.etcd.io/bbolt"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/errors"
	"github.com/drand/drand/log"
)

// BoltStore implements the Store interface using the kv storage boltdb (native
// golang implementation). Internally, Beacons are stored as JSON-encoded in the
// db file.
//
//nolint:gocritic// We do want to have a mutex here
type BoltStore struct {
	sync.Mutex
	db *bolt.DB

	log log.Logger
}

var beaconBucket = []byte("beacons")

// BoltFileName is the name of the file boltdb writes to
const BoltFileName = "drand.db"

// BoltStoreOpenPerm is the permission we will use to read bolt store file from disk
const BoltStoreOpenPerm = 0660

// NewBoltStore returns a Store implementation using the boltdb storage engine.
func NewBoltStore(l log.Logger, folder string, opts *bolt.Options) (*BoltStore, error) {
	dbPath := path.Join(folder, BoltFileName)
	db, err := bolt.Open(dbPath, BoltStoreOpenPerm, opts)
	if err != nil {
		return nil, err
	}
	// create the bucket already
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(beaconBucket)
		return err
	})

	return &BoltStore{
		log: l,
		db:  db,
	}, err
}

// Len uses the heuristic of finding the current round and adding one to guess the length of the store.
// This was due to limitations in bolt store which make the len call in stats being veeeeery slow
func (b *BoltStore) Len(c context.Context) (int, error) {
	lastBeacon, err := b.Last(c)

	if err != nil {
		if goErrors.Is(err, errors.ErrNoBeaconStored) {
			return 0, nil
		}
		return 0, err
	}

	if lastBeacon == nil {
		return 0, nil
	}

	if bits.UintSize == 32 && lastBeacon.Round > math.MaxInt32 {
		return 0, goErrors.New("integer overflow while calculating DB len")
	}

	return int(lastBeacon.Round + 1), nil
}

func (b *BoltStore) Close(context.Context) error {
	err := b.db.Close()
	if err != nil {
		b.log.Errorw("", "boltdb", "close", "err", err)
	}
	return err
}

// Put implements the Store interface. WARNING: It does NOT verify that this
// beacon is not already saved in the database or not and will overwrite it.
func (b *BoltStore) Put(_ context.Context, beacon *chain.Beacon) error {
	err := b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		key := chain.RoundToBytes(beacon.Round)
		buff, err := beacon.Marshal()
		if err != nil {
			return err
		}
		return bucket.Put(key, buff)
	})
	return err
}

// Last returns the last beacon signature saved into the db
func (b *BoltStore) Last(context.Context) (*chain.Beacon, error) {
	beacon := &chain.Beacon{}
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		cursor := bucket.Cursor()
		_, v := cursor.Last()
		if v == nil {
			return errors.ErrNoBeaconStored
		}
		return beacon.Unmarshal(v)
	})
	return beacon, err
}

// Get returns the beacon saved at this round
func (b *BoltStore) Get(_ context.Context, round uint64) (*chain.Beacon, error) {
	beacon := &chain.Beacon{}
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		v := bucket.Get(chain.RoundToBytes(round))
		if v == nil {
			return errors.ErrNoBeaconStored
		}
		return beacon.Unmarshal(v)
	})
	return beacon, err
}

func (b *BoltStore) Del(_ context.Context, round uint64) error {
	return b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		return bucket.Delete(chain.RoundToBytes(round))
	})
}

func (b *BoltStore) Cursor(ctx context.Context, fn func(context.Context, chain.Cursor) error) error {
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		c := bucket.Cursor()
		return fn(ctx, &boltCursor{Cursor: c})
	})
	if err != nil {
		log.DefaultLogger().Warnw("", "boltdb", "error getting cursor", "err", err)
	}
	return err
}

// SaveTo saves the bolt database to an alternate file.
func (b *BoltStore) SaveTo(_ context.Context, w io.Writer) error {
	return b.db.View(func(tx *bolt.Tx) error {
		_, err := tx.WriteTo(w)
		return err
	})
}

type boltCursor struct {
	*bolt.Cursor
}

func (c *boltCursor) First(context.Context) (*chain.Beacon, error) {
	k, v := c.Cursor.First()
	if k == nil {
		return nil, errors.ErrNoBeaconStored
	}
	b := &chain.Beacon{}
	err := b.Unmarshal(v)
	return b, err
}

func (c *boltCursor) Next(context.Context) (*chain.Beacon, error) {
	k, v := c.Cursor.Next()
	if k == nil {
		return nil, errors.ErrNoBeaconStored
	}
	b := &chain.Beacon{}
	err := b.Unmarshal(v)
	return b, err
}

func (c *boltCursor) Seek(_ context.Context, round uint64) (*chain.Beacon, error) {
	k, v := c.Cursor.Seek(chain.RoundToBytes(round))
	if k == nil {
		return nil, errors.ErrNoBeaconStored
	}
	b := &chain.Beacon{}
	err := b.Unmarshal(v)
	return b, err
}

func (c *boltCursor) Last(context.Context) (*chain.Beacon, error) {
	k, v := c.Cursor.Last()
	if k == nil {
		return nil, errors.ErrNoBeaconStored
	}
	b := &chain.Beacon{}
	err := b.Unmarshal(v)
	return b, err
}
