//go:build postgres

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
	"github.com/drand/drand/crypto"
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
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	prevMatters := sch.Name == crypto.DefaultSchemeID
	if prevMatters {
		// This test stores b2 then b1. However, when the beacon order matters, the correct
		// and expected order to store beacons in is b1 then b2.
		// There's a special beacon.appendStore which serves as an interceptor for these kind
		// of cases and should error when trying to store b1 if b2 was already stored.
		// In the previous implementation when the previous signature was also stored with the
		// current round, this wasn't a problem as the beacon could miss the previous round
		// yet could be fully retrieved.
		// However, now that we rely on the previous value actually existing in the database,
		// this test will fail.
		// TODO (dlsniper): Agree that this test needs to be updated to reflect the new
		//  implementation of the Store interface.
		t.Skipf("This test does not make sense from a chained beacon perspective.")
	}
	if prevMatters {
		ctx = chain.SetPreviousRequiredOnContext(ctx)
	}
	l, db := test.NewUnit(t, c, t.Name())

	beaconName := "beacon"
	store, err := pgdb.NewStore(ctx, l, db, beaconName)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, store.Close(ctx))
	}()

	b0 := &chain.Beacon{
		PreviousSig: nil,
		Round:       0,
		Signature:   []byte("round 0 signature"),
	}
	b1 := &chain.Beacon{
		PreviousSig: b0.Signature,
		Round:       1,
		Signature:   []byte("round 1 signature"),
	}

	b2 := &chain.Beacon{
		PreviousSig: b1.Signature,
		Round:       2,
		Signature:   []byte("round 2 signature"),
	}

	if !prevMatters {
		b1.PreviousSig = nil
		b2.PreviousSig = nil
	}

	// we store b2 and check if it is last
	require.NoError(t, store.Put(ctx, b2))
	eb2, err := store.Last(ctx)
	require.NoError(t, err)
	require.True(t, b2.Equal(eb2))
	eb2, err = store.Last(ctx)
	require.NoError(t, err)
	require.True(t, b2.Equal(eb2))

	// then we store b1
	require.NoError(t, store.Put(ctx, b1))

	// and request last again
	eb2, err = store.Last(ctx)
	require.NoError(t, err)
	require.True(t, b2.Equal(eb2))
}

func TestStore_Cursor(t *testing.T) {
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	prevMatters := sch.Name == crypto.DefaultSchemeID
	if prevMatters {
		ctx = chain.SetPreviousRequiredOnContext(ctx)
	}
	l, db := test.NewUnit(t, c, t.Name())

	beaconName := t.Name()
	dbStore, err := pgdb.NewStore(ctx, l, db, beaconName)
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

	for i := 0; i < len(sigs); i++ {
		var prevSig []byte
		if i > 0 {
			prevSig = sigs[i-1]
		}
		b := &chain.Beacon{
			Round:     uint64(i),
			Signature: sigs[i],
		}
		if prevMatters {
			b.PreviousSig = prevSig
		}

		beacons[i] = b

		require.NoError(t, dbStore.Put(ctx, b))
	}

	t.Log("generated beacons:")
	for i := 0; i < len(beacons); i++ {
		t.Logf("beacons[%d]: %#v\n", i, *beacons[i])
	}

	err = dbStore.Cursor(ctx, func(ctx context.Context, c chain.Cursor) error {
		b, err := c.Last(ctx)
		require.NoError(t, err)
		require.NotNil(t, b)
		require.True(t, beacons[len(beacons)-1].Equal(b))

		for key, orig := range beacons {
			t.Logf("seeking beacon %d\n", key)

			b, err := c.Seek(ctx, uint64(key))
			require.NoError(t, err)
			require.NotNil(t, b)
			require.True(t, orig.Equal(b))

			n, err := c.Next(ctx)
			if key == len(beacons)-1 {
				require.ErrorIs(t, err, chainerrors.ErrNoBeaconStored)
			} else {
				require.NoError(t, err)
				require.True(t, beacons[key+1].Equal(n))
			}
		}

		return nil
	})
	require.NoError(t, err)
}

func Test_StorePG(t *testing.T) {
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	prevMatters := sch.Name == crypto.DefaultSchemeID
	if prevMatters {
		ctx = chain.SetPreviousRequiredOnContext(ctx)
	}
	l, db := test.NewUnit(t, c, t.Name())

	beaconName := t.Name()
	store, err := pgdb.NewStore(ctx, l, db, beaconName)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, store.Close(ctx))
	}()

	doStorePgTest(ctx, t, store, l, db, beaconName, prevMatters)
}

func Test_WithReservedIdentifier(t *testing.T) {
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	prevMatters := sch.Name == crypto.DefaultSchemeID
	if prevMatters {
		ctx = chain.SetPreviousRequiredOnContext(ctx)
	}
	l, db := test.NewUnit(t, c, t.Name())

	// We want to have a reserved Postgres identifier here.
	// It helps making sure that the underlying engine doesn't have a problem with the default beacon.
	beaconName := "default"
	store, err := pgdb.NewStore(ctx, l, db, beaconName)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, store.Close(ctx))
	}()

	doStorePgTest(ctx, t, store, l, db, beaconName, prevMatters)
}

//nolint:funlen // We want this to be lengthy function
func doStorePgTest(ctx context.Context, t *testing.T, dbStore *pgdb.Store, l log.Logger, db *sqlx.DB, beaconName string, prevMatters bool) {
	var sig0 = []byte{0x00, 0x01, 0x02}
	var sig1 = []byte{0x01, 0x02, 0x03}
	var sig2 = []byte{0x02, 0x03, 0x04}

	ln, err := dbStore.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, ln)

	b0 := &chain.Beacon{
		PreviousSig: nil,
		Round:       0,
		Signature:   sig0,
	}

	b1 := &chain.Beacon{
		PreviousSig: sig0,
		Round:       1,
		Signature:   sig1,
	}

	b2 := &chain.Beacon{
		PreviousSig: sig1,
		Round:       2,
		Signature:   sig2,
	}

	if !prevMatters {
		b1.PreviousSig = nil
		b2.PreviousSig = nil
	}

	require.NoError(t, dbStore.Put(ctx, b0))

	require.NoError(t, dbStore.Put(ctx, b1))
	ln, err = dbStore.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, ln)

	require.NoError(t, dbStore.Put(ctx, b1))
	ln, err = dbStore.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, ln)

	require.NoError(t, dbStore.Put(ctx, b2))
	ln, err = dbStore.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, ln)

	received, err := dbStore.Last(ctx)
	require.NoError(t, err)
	require.True(t, b2.Equal(received))

	// =========================================================================

	dbStore, err = pgdb.NewStore(ctx, l, db, beaconName)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, dbStore.Close(ctx))
	}()

	require.NoError(t, dbStore.Put(ctx, b1))

	require.NoError(t, dbStore.Put(ctx, b1))
	bb1, err := dbStore.Get(ctx, b1.Round)
	require.NoError(t, err)
	require.True(t, b1.Equal(bb1))

	// =========================================================================

	dbStore, err = pgdb.NewStore(ctx, l, db, beaconName)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, dbStore.Close(ctx))
	}()

	err = dbStore.Put(ctx, b1)
	require.NoError(t, err)
	err = dbStore.Put(ctx, b2)
	require.NoError(t, err)

	err = dbStore.Cursor(ctx, func(ctx context.Context, c chain.Cursor) error {
		expecteds := []*chain.Beacon{b0, b1, b2}
		b, err := c.First(ctx)
		for i := 0; b != nil; b, err = c.Next(ctx) {
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
		require.True(t, b2.Equal(lb2))
		return nil
	})
	require.NoError(t, err)

	_, err = dbStore.Get(ctx, 10000)
	require.Equal(t, chainerrors.ErrNoBeaconStored, err)
}
