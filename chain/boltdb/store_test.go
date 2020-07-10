package boltdb

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/stretchr/testify/require"
)

func TestStoreBoltOrder(t *testing.T) {
	tmp, err := ioutil.TempDir("", "drandtest*")
	require.NoError(t, err)
	defer os.RemoveAll(tmp)
	store, err := NewBoltStore(tmp, nil)
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

	require.NoError(t, store.Put(b1))
	time.Sleep(10 * time.Millisecond)
	require.Equal(t, 1, store.Len())
	eb1, err := store.Last()
	require.NoError(t, err)
	require.Equal(t, b1, eb1)

	require.NoError(t, store.Put(b2))
	eb2, err := store.Last()
	require.NoError(t, err)
	require.Equal(t, b2, eb2)
	eb2, err = store.Last()
	require.NoError(t, err)
	require.Equal(t, b2, eb2)
	eb2, err = store.Last()
	require.NoError(t, err)
	require.Equal(t, eb2, b2)
}

func TestStoreBolt(t *testing.T) {
	tmp, err := ioutil.TempDir("", "bolttest*")
	require.NoError(t, err)
	defer os.RemoveAll(tmp)

	var sig1 = []byte{0x01, 0x02, 0x03}
	var sig2 = []byte{0x02, 0x03, 0x04}

	store, err := NewBoltStore(tmp, nil)
	require.NoError(t, err)

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
	store, err = NewBoltStore(tmp, nil)
	require.NoError(t, err)
	require.NoError(t, store.Put(b1))

	require.NoError(t, store.Put(b1))
	bb1, err := store.Get(b1.Round)
	require.NoError(t, err)
	require.Equal(t, b1, bb1)
	store.Close()

	store, err = NewBoltStore(tmp, nil)
	require.NoError(t, err)
	store.Put(b1)
	store.Put(b2)

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
	require.Equal(t, ErrNoBeaconSaved, err)
}
