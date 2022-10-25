package pg

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
)

// TODO: write more tests

const cleanupQuery = `DROP TABLE %s`

func (p PGStore) cleanup() error {
	_, err := p.db.QueryxContext(context.Background(), fmt.Sprintf(cleanupQuery, p.tableName))
	return err
}

func TestStorePGOrder(t *testing.T) {
	beaconName := t.Name()
	l := log.NewLogger(nil, log.LogDebug)

	store, err := NewPGStore(l, beaconName, nil)
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

func TestStorePG(t *testing.T) {
	beaconName := t.Name()

	var sig1 = []byte{0x01, 0x02, 0x03}
	var sig2 = []byte{0x02, 0x03, 0x04}

	l := log.NewLogger(nil, log.LogDebug)

	store, err := NewPGStore(l, beaconName, nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		err := store.cleanup()
		require.NoError(t, err)
	})

	require.Equal(t, 0, store.Len())

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

	require.NoError(t, store.Put(b1))
	require.Equal(t, 1, store.Len())
	require.NoError(t, store.Put(b1))
	require.Equal(t, 1, store.Len())
	require.NoError(t, store.Put(b2))
	require.Equal(t, 2, store.Len())

	received, err := store.Last()
	require.NoError(t, err)
	require.Equal(t, b2, received)

	store.Close()
	store, err = NewPGStore(l, beaconName, nil)
	require.NoError(t, err)
	require.NoError(t, store.Put(b1))

	require.NoError(t, store.Put(b1))
	bb1, err := store.Get(b1.Round)
	require.NoError(t, err)
	require.Equal(t, b1, bb1)
	store.Close()

	store, err = NewPGStore(l, beaconName, nil)
	require.NoError(t, err)
	err = store.Put(b1)
	require.NoError(t, err)
	err = store.Put(b2)
	require.NoError(t, err)

	store.Cursor(func(c chain.Cursor) {
		expecteds := []*chain.Beacon{b1, b2}
		i := 0
		for b := c.First(); b != nil; b = c.Next() {
			require.True(t, expecteds[i].Equal(b))
			i++
		}

		unknown := c.Seek(10000)
		require.Nil(t, unknown)
	})

	store.Cursor(func(c chain.Cursor) {
		lb2 := c.Last()
		require.NotNil(t, lb2)
		require.Equal(t, b2, lb2)
	})

	unknown, err := store.Get(10000)
	require.Nil(t, unknown)
	require.ErrorIs(t, err, ErrNoBeaconSaved)
}
