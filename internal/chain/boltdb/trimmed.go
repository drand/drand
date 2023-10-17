package boltdb

import (
	"context"
	"errors"
	"io"
	"path"
	"sync"

	"github.com/drand/drand/common/tracer"

	bolt "go.etcd.io/bbolt"

	"github.com/drand/drand/common"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/internal/chain"
	chainerrors "github.com/drand/drand/internal/chain/errors"
)

// trimmedStore implements the Store interface using the kv storage boltdb (native
// golang implementation). Internally, Beacons are stored as JSON-encoded in the
// db file.
type trimmedStore struct {
	sync.Mutex
	db *bolt.DB

	log log.Logger

	requiresPrevious bool
}

// newTrimmedStore returns a Store implementation using the boltdb storage engine.
func newTrimmedStore(ctx context.Context, l log.Logger, folder string, opts *bolt.Options) (*trimmedStore, error) {
	ctx, span := tracer.NewSpan(ctx, "boltTrimmedStore.NewTrimmedStore")
	defer span.End()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

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

	return &trimmedStore{
		log: l,
		db:  db,

		requiresPrevious: chain.PreviousRequiredFromContext(ctx),
	}, err
}

// Len performs a big scan over the bucket and is _very_ slow - use sparingly!
//
//nolint:dupl // This is a function on a separate type.
func (b *trimmedStore) Len(ctx context.Context) (int, error) {
	ctx, span := tracer.NewSpan(ctx, "boltTrimmedStore.Len")
	defer span.End()

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

func (b *trimmedStore) Close() error {
	err := b.db.Close()
	if err != nil {
		b.log.Errorw("", "boltdb", "close", "err", err)
	}
	return err
}

// Put implements the Store interface. WARNING: It does NOT verify that this
// beacon is not already saved in the database or not and will overwrite it.
func (b *trimmedStore) Put(ctx context.Context, beacon *common.Beacon) error {
	ctx, span := tracer.NewSpan(ctx, "boltTrimmedStore.Put")
	defer span.End()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)

		// We know this will be an append-only workload, so let's use a compact db.
		bucket.FillPercent = 1.0

		key := chain.RoundToBytes(beacon.Round)
		err := bucket.Put(key, beacon.Signature)
		if err != nil {
			b.log.Errorw("storing beacon", "round", beacon.Round, "err", err)
		}
		return err
	})
}

// Last returns the last beacon signature saved into the db
func (b *trimmedStore) Last(ctx context.Context) (*common.Beacon, error) {
	ctx, span := tracer.NewSpan(ctx, "boltTrimmedStore.Last")
	defer span.End()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	beacon := common.Beacon{}
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		cursor := bucket.Cursor()
		b, err := b.getCursorBeacon(ctx, bucket, cursor.Last)
		if err != nil {
			return err
		}

		beacon.Round = b.Round
		beacon.Signature = b.Signature
		beacon.PreviousSig = b.PreviousSig
		return nil
	})
	return &beacon, err
}

// Get returns the beacon saved at this round
func (b *trimmedStore) Get(ctx context.Context, round uint64) (*common.Beacon, error) {
	ctx, span := tracer.NewSpan(ctx, "boltTrimmedStore.Get")
	defer span.End()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var beacon *common.Beacon
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		b, err := b.getBeacon(ctx, bucket, round, true)
		if err != nil {
			return err
		}

		beacon = b
		return nil
	})
	return beacon, err
}

func (b *trimmedStore) getBeacon(ctx context.Context, bucket *bolt.Bucket, round uint64, canFetchPrevious bool) (*common.Beacon, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	sig := bucket.Get(chain.RoundToBytes(round))
	if sig == nil {
		return nil, chainerrors.ErrNoBeaconStored
	}

	beacon := common.Beacon{
		Round:     round,
		Signature: make([]byte, len(sig)),
	}
	copy(beacon.Signature, sig)

	if canFetchPrevious &&
		b.requiresPrevious &&
		beacon.Round > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		prevSig := bucket.Get(chain.RoundToBytes(round - 1))
		if prevSig == nil {
			b.log.Errorw("missing previous beacon from database", "round", beacon.Round-1)
			return nil, chainerrors.ErrNoBeaconStored
		}
		beacon.PreviousSig = make([]byte, len(prevSig))
		copy(beacon.PreviousSig, prevSig)
	}

	return &beacon, nil
}

func (b *trimmedStore) Del(ctx context.Context, round uint64) error {
	ctx, span := tracer.NewSpan(ctx, "boltTrimmedStore.Del")
	defer span.End()

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

func (b *trimmedStore) Cursor(ctx context.Context, fn func(context.Context, chain.Cursor) error) error {
	ctx, span := tracer.NewSpan(ctx, "boltTrimmedStore.Cursor")
	defer span.End()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		c := bucket.Cursor()
		return fn(ctx, &trimmedBoltCursor{Cursor: c, store: b})
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
func (b *trimmedStore) SaveTo(ctx context.Context, w io.Writer) error {
	_, span := tracer.NewSpan(ctx, "boltTrimmedStore.SaveTo")
	defer span.End()

	return b.db.View(func(tx *bolt.Tx) error {
		_, err := tx.WriteTo(w)
		return err
	})
}

type trimmedBoltCursor struct {
	*bolt.Cursor
	store *trimmedStore
}

func (c *trimmedBoltCursor) First(ctx context.Context) (*common.Beacon, error) {
	ctx, span := tracer.NewSpan(ctx, "boltTrimmedStore.Cursor.First")
	defer span.End()

	return c.store.getCursorBeacon(ctx, c.Bucket(), c.Cursor.First)
}

// Next returns the next value in the database for the given cursor.
// When reaching the end of the database, it emits the ErrNoBeaconStored error to flag that it finished the iteration.
func (c *trimmedBoltCursor) Next(ctx context.Context) (*common.Beacon, error) {
	ctx, span := tracer.NewSpan(ctx, "boltTrimmedStore.Cursor.Next")
	defer span.End()

	return c.store.getCursorBeacon(ctx, c.Bucket(), c.Cursor.Next)
}

func (c *trimmedBoltCursor) Seek(ctx context.Context, round uint64) (*common.Beacon, error) {
	ctx, span := tracer.NewSpan(ctx, "boltTrimmedStore.Cursor.Seek")
	defer span.End()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	_, v := c.Cursor.Seek(chain.RoundToBytes(round))
	if v == nil {
		return nil, chainerrors.ErrNoBeaconStored
	}

	b := common.Beacon{
		Round:     round,
		Signature: v,
	}

	if c.store.requiresPrevious &&
		b.Round > 0 {
		prevBeacon, err := c.store.getBeacon(ctx, c.Bucket(), b.Round-1, false)
		if err != nil {
			c.store.log.Errorw("missing previous beacon from database", "round", b.Round-1, "err", err)
			return nil, chainerrors.ErrNoBeaconStored
		}
		b.PreviousSig = prevBeacon.Signature
	}

	return &b, nil
}

func (c *trimmedBoltCursor) Last(ctx context.Context) (*common.Beacon, error) {
	ctx, span := tracer.NewSpan(ctx, "boltTrimmedStore.Cursor.Last")
	defer span.End()

	return c.store.getCursorBeacon(ctx, c.Bucket(), c.Cursor.Last)
}

type beaconCursorGetter func() (key []byte, value []byte)

func (b *trimmedStore) getCursorBeacon(ctx context.Context, bucket *bolt.Bucket, get beaconCursorGetter) (*common.Beacon, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	key, sig := get()
	if key == nil {
		return nil, chainerrors.ErrNoBeaconStored
	}

	beacon := common.Beacon{
		Round:     chain.BytesToRound(key),
		Signature: make([]byte, len(sig)),
	}
	copy(beacon.Signature, sig)

	if b.requiresPrevious &&
		beacon.Round > 0 {
		prevBeacon, err := b.getBeacon(ctx, bucket, beacon.Round-1, false)
		if err != nil {
			return nil, err
		}
		if prevBeacon == nil {
			b.log.Errorw("missing previous beacon from database", "round", beacon.Round-1)
			return nil, chainerrors.ErrNoBeaconStored
		}
		beacon.PreviousSig = make([]byte, len(prevBeacon.Signature))
		copy(beacon.PreviousSig, prevBeacon.Signature)
	}

	return &beacon, nil
}
