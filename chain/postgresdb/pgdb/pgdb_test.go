package pgdb_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/chain"
	chainerrors "github.com/drand/drand/chain/errors"
	"github.com/drand/drand/chain/postgresdb/pgdb"
	"github.com/drand/drand/log"
	"github.com/drand/drand/test"
)

var c *test.Container

func TestMain(m *testing.M) {
	var err error
	c, err = test.StartPGDB()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer test.StopPGDB(c)

	m.Run()
}

func Test_OrderStorePG(t *testing.T) {
	ctx := context.Background()
	l, db, teardown := test.NewUnit(t, c, t.Name())
	defer t.Cleanup(teardown)

	beaconName := "beacon"
	store, err := pgdb.NewStore(context.Background(), l, db, beaconName)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, store.Close(ctx))
	}()

	b1 := &chain.Beacon{
		PreviousSig: []byte("a magnificent signature"),
		Round:       145,
		Signature:   []byte("one signature to"),
	}

	b2 := &chain.Beacon{
		PreviousSig: []byte("is not worth an invalid one"),
		Round:       146,
		Signature:   []byte("govern them all"),
	}

	// we store b2 and check if it is last
	require.NoError(t, store.Put(ctx, b2))
	eb2, err := store.Last(ctx)
	require.NoError(t, err)
	require.Equal(t, b2, eb2)
	eb2, err = store.Last(ctx)
	require.NoError(t, err)
	require.Equal(t, b2, eb2)

	// then we store b1
	require.NoError(t, store.Put(ctx, b1))

	// and request last again
	eb2, err = store.Last(ctx)
	require.NoError(t, err)
	require.Equal(t, b2, eb2)
}

func TestStore_Cursor(t *testing.T) {
	ctx := context.Background()
	l, db, teardown := test.NewUnit(t, c, t.Name())
	defer t.Cleanup(teardown)

	beaconName := t.Name()
	dbStore, err := pgdb.NewStore(context.Background(), l, db, beaconName)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, dbStore.Close(ctx))
	}()

	sigs := map[int][]byte{
		0: {0x01, 0x02, 0x03},
		1: {0x02, 0x03, 0x04},
		2: {0x03, 0x04, 0x05},
		3: {0x04, 0x05, 0x06},
		4: {0x05, 0x06, 0x07},
		5: {0x06, 0x07, 0x08},
	}

	beacons := make(map[int]*chain.Beacon, len(sigs)-1)

	for i := 1; i < len(sigs); i++ {
		b := &chain.Beacon{
			PreviousSig: sigs[i-1],
			Round:       uint64(i),
			Signature:   sigs[i],
		}

		beacons[i] = b

		require.NoError(t, dbStore.Put(ctx, b))
	}

	t.Log("generated beacons:")
	for i, beacon := range beacons {
		t.Logf("beacons[%d]: %#v\n", i, *beacon)
	}

	err = dbStore.Cursor(ctx, func(ctx context.Context, c chain.Cursor) error {
		b, err := c.Last(ctx)
		require.NoError(t, err)
		require.NotNil(t, b)
		require.Equal(t, beacons[len(beacons)], b)

		for key, orig := range beacons {
			t.Logf("seeking beacon %d\n", key)

			b, err := c.Seek(ctx, uint64(key))
			require.NoError(t, err)
			require.NotNil(t, b)
			require.Equal(t, orig, b)

			n, err := c.Next(ctx)
			if key == len(beacons) {
				require.ErrorIs(t, err, chainerrors.ErrNoBeaconStored)
			} else {
				require.NoError(t, err)
				require.Equal(t, beacons[key+1], n)
			}
		}

		return nil
	})
	require.NoError(t, err)
}

func Test_StorePG(t *testing.T) {
	ctx := context.Background()
	l, db, teardown := test.NewUnit(t, c, t.Name())
	defer t.Cleanup(teardown)

	beaconName := t.Name()
	store, err := pgdb.NewStore(context.Background(), l, db, beaconName)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, store.Close(ctx))
	}()

	doStorePgTest(ctx, t, store, l, db, beaconName)
}

func Test_WithReservedIdentifier(t *testing.T) {
	ctx := context.Background()
	l, db, teardown := test.NewUnit(t, c, t.Name())
	defer t.Cleanup(teardown)

	// We want to have a reserved Postgres identifier here.
	// It helps making sure that the underlying engine doesn't have a problem with the default beacon.
	beaconName := "default"
	store, err := pgdb.NewStore(context.Background(), l, db, beaconName)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, store.Close(ctx))
	}()

	doStorePgTest(ctx, t, store, l, db, beaconName)
}

func doStorePgTest(ctx context.Context, t *testing.T, dbStore *pgdb.Store, l log.Logger, db *sqlx.DB, beaconName string) {
	var sig1 = []byte{0x01, 0x02, 0x03}
	var sig2 = []byte{0x02, 0x03, 0x04}

	ln, err := dbStore.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, ln)

	b1 := &chain.Beacon{
		PreviousSig: sig1,
		Round:       145,
		Signature:   sig2,
	}

	b2 := &chain.Beacon{
		PreviousSig: sig2,
		Round:       146,
		Signature:   sig1,
	}

	require.NoError(t, dbStore.Put(ctx, b1))
	ln, err = dbStore.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, ln)
	require.NoError(t, dbStore.Put(ctx, b1))
	ln, _ = dbStore.Len(ctx)
	require.Equal(t, 1, ln)
	require.NoError(t, dbStore.Put(ctx, b2))
	ln, _ = dbStore.Len(ctx)
	require.Equal(t, 2, ln)

	received, err := dbStore.Last(ctx)
	require.NoError(t, err)
	require.Equal(t, b2, received)

	// =========================================================================

	dbStore, err = pgdb.NewStore(context.Background(), l, db, beaconName)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, dbStore.Close(ctx))
	}()

	require.NoError(t, dbStore.Put(ctx, b1))

	require.NoError(t, dbStore.Put(ctx, b1))
	bb1, err := dbStore.Get(ctx, b1.Round)
	require.NoError(t, err)
	require.Equal(t, b1, bb1)

	// =========================================================================

	dbStore, err = pgdb.NewStore(context.Background(), l, db, beaconName)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, dbStore.Close(ctx))
	}()

	err = dbStore.Put(ctx, b1)
	require.NoError(t, err)
	err = dbStore.Put(ctx, b2)
	require.NoError(t, err)

	err = dbStore.Cursor(ctx, func(ctx context.Context, c chain.Cursor) error {
		expecteds := []*chain.Beacon{b1, b2}
		i := 0
		b, err := c.First(ctx)
		for ; b != nil; b, err = c.Next(ctx) {
			require.NoError(t, err)
			require.True(t, expecteds[i].Equal(b))
			i++
		}
		// Last iteration will always produce an ErrNoBeaconSaved value
		if !errors.Is(err, chainerrors.ErrNoBeaconStored) {
			require.NoError(t, err)
		}

		unknown, err := c.Seek(ctx, 10000)
		require.ErrorIs(t, err, chainerrors.ErrNoBeaconStored)
		require.Nil(t, unknown)
		return nil
	})
	require.NoError(t, err)

	err = dbStore.Cursor(ctx, func(ctx context.Context, c chain.Cursor) error {
		lb2, err := c.Last(ctx)
		require.NoError(t, err)
		require.NotNil(t, lb2)
		require.Equal(t, b2, lb2)
		return nil
	})
	require.NoError(t, err)

	_, err = dbStore.Get(ctx, 10000)
	require.Equal(t, chainerrors.ErrNoBeaconStored, err)
}
