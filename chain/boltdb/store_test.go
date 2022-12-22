package boltdb

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/chain"
	chainerrors "github.com/drand/drand/chain/errors"
	"github.com/drand/drand/test"
)

func TestStoreBoltOrder(t *testing.T) {
	tmp := t.TempDir()
	ctx := context.Background()
	l := test.Logger(t)
	store, err := NewBoltStore(l, tmp, nil)
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

func TestStoreBolt(t *testing.T) {
	tmp := t.TempDir()
	ctx := context.Background()
	l := test.Logger(t)

	var sig1 = []byte{0x01, 0x02, 0x03}
	var sig2 = []byte{0x02, 0x03, 0x04}

	store, err := NewBoltStore(l, tmp, nil)
	require.NoError(t, err)

	sLen, err := store.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, sLen)

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

	require.NoError(t, store.Put(ctx, b1))
	sLen, err = store.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, sLen)

	require.NoError(t, store.Put(ctx, b1))
	sLen, err = store.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, sLen)

	require.NoError(t, store.Put(ctx, b2))
	sLen, err = store.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, sLen)

	received, err := store.Last(ctx)
	require.NoError(t, err)
	require.Equal(t, b2, received)

	err = store.Close(ctx)
	require.NoError(t, err)

	store, err = NewBoltStore(l, tmp, nil)
	require.NoError(t, err)
	require.NoError(t, store.Put(ctx, b1))

	require.NoError(t, store.Put(ctx, b1))
	bb1, err := store.Get(ctx, b1.Round)
	require.NoError(t, err)
	require.Equal(t, b1, bb1)
	err = store.Close(ctx)
	require.NoError(t, err)

	store, err = NewBoltStore(l, tmp, nil)
	require.NoError(t, err)
	err = store.Put(ctx, b1)
	require.NoError(t, err)
	err = store.Put(ctx, b2)
	require.NoError(t, err)

	err = store.Cursor(ctx, func(ctx context.Context, c chain.Cursor) error {
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

func TestStore_Cursor(t *testing.T) {
	tmp := t.TempDir()
	ctx := context.Background()
	l := test.Logger(t)
	dbStore, err := NewBoltStore(l, tmp, nil)
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
