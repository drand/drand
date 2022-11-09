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
	c, err = test.StartDB()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer test.StopDB(c)

	m.Run()
}

func Test_OrderStorePG(t *testing.T) {
	l, db, teardown := test.NewUnit(t, c, t.Name())
	defer t.Cleanup(teardown)

	beaconName := "beacon"
	store, err := pgdb.NewStore(context.Background(), l, db, beaconName)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, store.Close())
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
	require.NoError(t, store.Put(b2))
	eb2, err := store.Last()
	require.NoError(t, err)
	require.Equal(t, b2, eb2)
	eb2, err = store.Last()
	require.NoError(t, err)
	require.Equal(t, b2, eb2)

	// then we store b1
	require.NoError(t, store.Put(b1))

	// and request last again
	eb2, err = store.Last()
	require.NoError(t, err)
	require.Equal(t, b2, eb2)
}

func Test_StorePG(t *testing.T) {
	l, db, teardown := test.NewUnit(t, c, t.Name())
	defer func() {
		t.Cleanup(teardown)
	}()

	beaconName := t.Name()
	store, err := pgdb.NewStore(context.Background(), l, db, beaconName)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, store.Close())
	}()

	doStorePgTest(t, store, l, db, beaconName)
}

func Test_WithReservedIdentifier(t *testing.T) {
	l, db, teardown := test.NewUnit(t, c, t.Name())
	defer t.Cleanup(teardown)

	beaconName := t.Name()
	store, err := pgdb.NewStore(context.Background(), l, db, beaconName)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, store.Close())
	}()

	doStorePgTest(t, store, l, db, beaconName)
}

func doStorePgTest(t *testing.T, dbStore *pgdb.Store, l log.Logger, db *sqlx.DB, beaconName string) {
	var sig1 = []byte{0x01, 0x02, 0x03}
	var sig2 = []byte{0x02, 0x03, 0x04}

	ln, err := dbStore.Len()
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

	require.NoError(t, dbStore.Put(b1))
	ln, err = dbStore.Len()
	require.NoError(t, err)
	require.Equal(t, 1, ln)
	require.NoError(t, dbStore.Put(b1))
	ln, _ = dbStore.Len()
	require.Equal(t, 1, ln)
	require.NoError(t, dbStore.Put(b2))
	ln, _ = dbStore.Len()
	require.Equal(t, 2, ln)

	received, err := dbStore.Last()
	require.NoError(t, err)
	require.Equal(t, b2, received)

	// =========================================================================

	dbStore, err = pgdb.NewStore(context.Background(), l, db, beaconName)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, dbStore.Close())
	}()

	require.NoError(t, dbStore.Put(b1))

	require.NoError(t, dbStore.Put(b1))
	bb1, err := dbStore.Get(b1.Round)
	require.NoError(t, err)
	require.Equal(t, b1, bb1)

	// =========================================================================

	dbStore, err = pgdb.NewStore(context.Background(), l, db, beaconName)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, dbStore.Close())
	}()

	err = dbStore.Put(b1)
	require.NoError(t, err)
	err = dbStore.Put(b2)
	require.NoError(t, err)

	err = dbStore.Cursor(func(c chain.Cursor) error {
		expecteds := []*chain.Beacon{b1, b2}
		i := 0
		b, err := c.First()
		for ; b != nil; b, err = c.Next() {
			require.NoError(t, err)
			require.True(t, expecteds[i].Equal(b))
			i++
		}
		// Last iteration will always produce an ErrNoBeaconSaved value
		if !errors.Is(err, chainerrors.ErrNoBeaconStored) {
			require.NoError(t, err)
		}

		unknown, err := c.Seek(10000)
		require.ErrorIs(t, err, chainerrors.ErrNoBeaconStored)
		require.Nil(t, unknown)
		return nil
	})
	require.NoError(t, err)

	err = dbStore.Cursor(func(c chain.Cursor) error {
		lb2, err := c.Last()
		require.NoError(t, err)
		require.NotNil(t, lb2)
		require.Equal(t, b2, lb2)
		return nil
	})
	require.NoError(t, err)

	_, err = dbStore.Get(10000)
	require.Equal(t, chainerrors.ErrNoBeaconStored, err)
}
