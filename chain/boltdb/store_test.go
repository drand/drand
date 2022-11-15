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

	// the len calls no longer return the real DB length, but one assumed from the given round
	require.NoError(t, store.Put(ctx, b1))
	sLen, err = store.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 146, sLen)

	require.NoError(t, store.Put(ctx, b1))
	sLen, err = store.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 146, sLen)

	require.NoError(t, store.Put(ctx, b2))
	sLen, err = store.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 147, sLen)

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
	store.Close(ctx)

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

func TestEmptyStoreReturnsLenZero(t *testing.T) {
	store, err := NewBoltStore(test.Logger(t), t.TempDir(), nil)
	require.NoError(t, err)

	result, err := store.Len(context.Background())

	require.NoError(t, err)
	require.Equal(t, 0, result)
}

func TestNonEmptyStoreLenReturnsRoundPlusOne(t *testing.T) {
	ctx := context.Background()
	store, err := NewBoltStore(test.Logger(t), t.TempDir(), nil)
	require.NoError(t, err)

	var round uint64 = 1
	beacon := &chain.Beacon{
		Round: round,
	}
	err = store.Put(ctx, beacon)
	require.NoError(t, err)

	result, err := store.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, int(round+1), result)
}
