//go:build memdb

package memdb_test

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/chain"
	chainerrors "github.com/drand/drand/chain/errors"
	"github.com/drand/drand/chain/memdb"
)

func TestStoreBoltOrder(t *testing.T) {
	ctx := context.Background()

	store := memdb.NewStore(10)
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
	ctx := context.Background()

	var sig1 = []byte{0x01, 0x02, 0x03}
	var sig2 = []byte{0x02, 0x03, 0x04}

	s := memdb.NewStore(10)

	sLen, err := s.Len(ctx)
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

	require.NoError(t, s.Put(ctx, b1))
	sLen, err = s.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, sLen)

	require.NoError(t, s.Put(ctx, b1))
	sLen, err = s.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, sLen)

	require.NoError(t, s.Put(ctx, b2))
	sLen, err = s.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, sLen)

	received, err := s.Last(ctx)
	require.NoError(t, err)
	require.Equal(t, b2, received)

	err = s.Close(ctx)
	require.NoError(t, err)

	s = memdb.NewStore(10)
	require.NoError(t, s.Put(ctx, b1))

	require.NoError(t, s.Put(ctx, b1))
	bb1, err := s.Get(ctx, b1.Round)
	require.NoError(t, err)
	require.Equal(t, b1, bb1)
	err = s.Close(ctx)
	require.NoError(t, err)

	s = memdb.NewStore(10)
	err = s.Put(ctx, b1)
	require.NoError(t, err)
	err = s.Put(ctx, b2)
	require.NoError(t, err)

	err = s.Cursor(ctx, func(ctx context.Context, c chain.Cursor) error {
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

	err = s.Cursor(ctx, func(ctx context.Context, c chain.Cursor) error {
		lb2, err := c.Last(ctx)
		require.NoError(t, err)
		require.NotNil(t, lb2)
		require.Equal(t, b2, lb2)
		return nil
	})
	require.NoError(t, err)

	_, err = s.Get(ctx, 10000)
	require.Equal(t, chainerrors.ErrNoBeaconStored, err)
}

func TestStore_Cursor(t *testing.T) {
	ctx := context.Background()

	dbStore := memdb.NewStore(10)
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

	err := dbStore.Cursor(ctx, func(ctx context.Context, c chain.Cursor) error {
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

func TestStore_Put(t *testing.T) {
	ctx := context.Background()

	genBeacons := func(size int) []chain.Beacon {
		result := make([]chain.Beacon, size)
		for i := 0; i < size; i++ {
			result[i] = chain.Beacon{
				PreviousSig: []byte{byte(i - 1)},
				Round:       uint64(i),
				Signature:   []byte{byte(i)},
			}
		}

		return result
	}

	shuffle := func(size int) []chain.Beacon {
		rand.Seed(time.Now().UnixNano())
		result := genBeacons(size)

		rand.Shuffle(size, func(i, j int) {
			result[i], result[j] = result[j], result[i]
		})

		return result
	}

	tests := map[string]struct {
		bufferSize int
		beacons    []chain.Beacon
	}{
		"under-buffer":                 {5, genBeacons(3)},
		"equal-to-buffer":              {5, genBeacons(5)},
		"over-buffer":                  {5, genBeacons(18)},
		"out-of-order-under-buffer":    {5, shuffle(3)},
		"out-of-order-equal-to-buffer": {5, genBeacons(5)},
		"out-of-order-over-buffer":     {5, genBeacons(18)},
	}
	for tName, tt := range tests {
		tName := tName
		tt := tt
		t.Run(tName, func(t *testing.T) {
			s := memdb.NewStore(tt.bufferSize)

			for i := 0; i < len(tt.beacons); i++ {
				err := s.Put(ctx, &tt.beacons[i])
				require.NoError(t, err)
			}
		})
	}
}
