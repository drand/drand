package boltdb

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/testlogger"
	"github.com/drand/drand/v2/internal/chain"
	chainerrors "github.com/drand/drand/v2/internal/chain/errors"
	context2 "github.com/drand/drand/v2/internal/test/context"
)

func TestTrimmedStoreBoltOrder(t *testing.T) {
	tmp := t.TempDir()

	ctx, _, prevMatters := context2.PrevSignatureMattersOnContext(t, context.Background())

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
		t.Skip("This test does not make sense in chained mode.")
	}

	l := testlogger.New(t)
	store, err := newTrimmedStore(ctx, l, tmp, nil)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, store.Close())
	}()

	b0 := &common.Beacon{
		PreviousSig: nil,
		Round:       0,
		Signature:   []byte("round 0 signature"),
	}
	b1 := &common.Beacon{
		PreviousSig: b0.Signature,
		Round:       1,
		Signature:   []byte("round 1 signature"),
	}

	b2 := &common.Beacon{
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

//nolint:funlen // This function has the right length
func TestTrimmedStoreBolt(t *testing.T) {
	tmp := t.TempDir()

	ctx, _, prevMatters := context2.PrevSignatureMattersOnContext(t, context.Background())

	l := testlogger.New(t)

	var sig0 = []byte{0x00, 0x01, 0x02}
	var sig1 = []byte{0x01, 0x02, 0x03}
	var sig2 = []byte{0x02, 0x03, 0x04}

	store, err := newTrimmedStore(ctx, l, tmp, nil)
	require.NoError(t, err)

	sLen, err := store.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, sLen)

	b0 := &common.Beacon{
		PreviousSig: nil,
		Round:       0,
		Signature:   sig0,
	}

	b1 := &common.Beacon{
		PreviousSig: sig0,
		Round:       1,
		Signature:   sig1,
	}

	b2 := &common.Beacon{
		PreviousSig: sig1,
		Round:       2,
		Signature:   sig2,
	}

	if !prevMatters {
		b1.PreviousSig = nil
		b2.PreviousSig = nil
	}

	require.NoError(t, store.Put(ctx, b0))

	require.NoError(t, store.Put(ctx, b1))
	sLen, err = store.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, sLen)

	require.NoError(t, store.Put(ctx, b1))
	sLen, err = store.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, sLen)

	require.NoError(t, store.Put(ctx, b2))
	sLen, err = store.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, sLen)

	received, err := store.Last(ctx)
	require.NoError(t, err)
	require.Equal(t, b2, received)

	err = store.Close()
	require.NoError(t, err)

	store, err = newTrimmedStore(ctx, l, tmp, nil)
	require.NoError(t, err)
	require.NoError(t, store.Put(ctx, b1))

	require.NoError(t, store.Put(ctx, b1))
	bb1, err := store.Get(ctx, b1.Round)
	require.NoError(t, err)
	require.Equal(t, b1, bb1)
	err = store.Close()
	require.NoError(t, err)

	store, err = newTrimmedStore(ctx, l, tmp, nil)
	require.NoError(t, err)
	err = store.Put(ctx, b1)
	require.NoError(t, err)
	err = store.Put(ctx, b2)
	require.NoError(t, err)

	err = store.Cursor(ctx, func(ctx context.Context, c chain.Cursor) error {
		expecteds := []*common.Beacon{b0, b1, b2}
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

	err = store.Cursor(ctx, func(ctx context.Context, c chain.Cursor) error {
		lb2, err := c.Last(ctx)
		require.NoError(t, err)
		require.NotNil(t, lb2)
		require.Equal(t, b2, lb2)
		return nil
	})
	require.NoError(t, err)

	_, err = store.Get(ctx, 10000)
	require.Equal(t, chainerrors.ErrNoBeaconStored, err)
}

func TestTrimmedStore_Cursor(t *testing.T) {
	tmp := t.TempDir()

	ctx, _, prevMatters := context2.PrevSignatureMattersOnContext(t, context.Background())

	l := testlogger.New(t)
	dbStore, err := newTrimmedStore(ctx, l, tmp, nil)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, dbStore.Close())
	}()

	sigs := map[int][]byte{
		0: {0x01, 0x02, 0x03},
		1: {0x02, 0x03, 0x04},
		2: {0x03, 0x04, 0x05},
		3: {0x04, 0x05, 0x06},
		4: {0x05, 0x06, 0x07},
		5: {0x06, 0x07, 0x08},
	}

	beacons := make(map[int]*common.Beacon, len(sigs)-1)

	for i := 0; i < len(sigs); i++ {
		var prevSig []byte
		if i > 0 {
			prevSig = sigs[i-1]
		}
		b := &common.Beacon{
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
