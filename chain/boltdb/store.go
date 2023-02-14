package boltdb

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"sync"

	json "github.com/nikkolasg/hexjson"
	bolt "go.etcd.io/bbolt"

	"github.com/drand/drand/chain"
	chainerrors "github.com/drand/drand/chain/errors"
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

type newDBFormat bool

var useNewDBFormat newDBFormat = true

func IsATest(ctx context.Context) context.Context {
	return context.WithValue(ctx, useNewDBFormat, useNewDBFormat)
}

func isThisATest(ctx context.Context) bool {
	_, ok := ctx.Value(useNewDBFormat).(newDBFormat)
	return ok
}

// NewBoltStore returns a Store implementation using the boltdb storage engine.
func NewBoltStore(ctx context.Context, l log.Logger, folder string, opts *bolt.Options) (chain.Store, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	dbPath := path.Join(folder, BoltFileName)

	if shouldUseTrimmedBolt(ctx, l, dbPath, opts) {
		return newTrimmedStore(ctx, l, folder, opts)
	}

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

func shouldUseTrimmedBolt(ctx context.Context, l log.Logger, sourceBeaconPath string, opts *bolt.Options) bool {
	if isThisATest(ctx) {
		return false
	}

	// New beacons stores should use the trimmed version
	if _, err := os.Stat(sourceBeaconPath); errors.Is(err, os.ErrNotExist) {
		return true
	}

	// Existing beacon stores should use the format that's suitable
	existingDB, err := bolt.Open(sourceBeaconPath, BoltStoreOpenPerm, opts)
	if err != nil {
		l.Errorw("while trying to open existing bolt database", "err", err)
		return true
	}
	defer func() {
		_ = existingDB.Close()
	}()

	err = existingDB.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		_, value := bucket.Cursor().First()
		b := chain.Beacon{}
		return json.Unmarshal(value, &b)
	})

	return err != nil
}

// Len performs a big scan over the bucket and is _very_ slow - use sparingly!
func (b *BoltStore) Len(ctx context.Context) (int, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	var length = 0
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		// this `.Stats()` call is the particularly expensive one!
		length = bucket.Stats().KeyN
		return nil
	})
	if err != nil {
		b.log.Warnw("", "boltdb", "error getting length", "err", err)
	}
	return length, err
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
func (b *BoltStore) Put(ctx context.Context, beacon *chain.Beacon) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		key := chain.RoundToBytes(beacon.Round)
		buff, err := beacon.Marshal()
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		err = bucket.Put(key, buff)
		if err != nil {
			b.log.Debugw("storing beacon", "round", beacon.Round, "err", err)
		}
		return err
	})
}

// Last returns the last beacon signature saved into the db
func (b *BoltStore) Last(ctx context.Context) (*chain.Beacon, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	beacon := &chain.Beacon{}
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		cursor := bucket.Cursor()
		_, v := cursor.Last()
		if v == nil {
			return chainerrors.ErrNoBeaconStored
		}
		return beacon.Unmarshal(v)
	})
	return beacon, err
}

// Get returns the beacon saved at this round
func (b *BoltStore) Get(ctx context.Context, round uint64) (*chain.Beacon, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	beacon := &chain.Beacon{}
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		v := bucket.Get(chain.RoundToBytes(round))
		if v == nil {
			return chainerrors.ErrNoBeaconStored
		}
		return beacon.Unmarshal(v)
	})
	return beacon, err
}

func (b *BoltStore) Del(ctx context.Context, round uint64) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		return bucket.Delete(chain.RoundToBytes(round))
	})
}

func (b *BoltStore) Cursor(ctx context.Context, fn func(context.Context, chain.Cursor) error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		c := bucket.Cursor()
		return fn(ctx, &boltCursor{Cursor: c})
	})
	if err != nil {
		// We omit the ErrNoBeaconStored error as it is noisy and cursor.Next() will use it as flag value
		// for reaching the end of the database.
		if !errors.Is(err, chainerrors.ErrNoBeaconStored) {
			b.log.Errorw("", "boltdb", "error getting cursor", "err", err)
		}
	}
	return err
}

// SaveTo saves the bolt database to an alternate file.
func (b *BoltStore) SaveTo(ctx context.Context, w io.Writer) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return b.db.View(func(tx *bolt.Tx) error {
		_, err := tx.WriteTo(w)
		return err
	})
}

type boltCursor struct {
	*bolt.Cursor
}

func (c *boltCursor) First(ctx context.Context) (*chain.Beacon, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	k, v := c.Cursor.First()
	if k == nil {
		return nil, chainerrors.ErrNoBeaconStored
	}
	b := &chain.Beacon{}
	err := b.Unmarshal(v)
	return b, err
}

// Next returns the next value in the database for the given cursor.
// When reaching the end of the database, it emits the ErrNoBeaconStored error to flag that it finished the iteration.
func (c *boltCursor) Next(ctx context.Context) (*chain.Beacon, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	k, v := c.Cursor.Next()
	if k == nil {
		return nil, chainerrors.ErrNoBeaconStored
	}
	b := &chain.Beacon{}
	err := b.Unmarshal(v)
	return b, err
}

func (c *boltCursor) Seek(ctx context.Context, round uint64) (*chain.Beacon, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	k, v := c.Cursor.Seek(chain.RoundToBytes(round))
	if k == nil {
		return nil, chainerrors.ErrNoBeaconStored
	}
	b := &chain.Beacon{}
	err := b.Unmarshal(v)
	return b, err
}

func (c *boltCursor) Last(ctx context.Context) (*chain.Beacon, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	k, v := c.Cursor.Last()
	if k == nil {
		return nil, chainerrors.ErrNoBeaconStored
	}
	b := &chain.Beacon{}
	err := b.Unmarshal(v)
	return b, err
}
