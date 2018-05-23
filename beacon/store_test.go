package beacon

import (
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBoltStore(t *testing.T) {
	tmp := path.Join(os.TempDir(), "drandtest")
	require.NoError(t, os.MkdirAll(tmp, 0755))
	path := tmp
	defer os.RemoveAll(tmp)

	var sig1 = []byte{0x01, 0x02, 0x03}
	var sig2 = []byte{0x02, 0x03, 0x04}

	store, err := NewBoltStore(path, nil)
	require.NoError(t, err)

	b1 := &Beacon{
		PreviousRand: sig1,
		Round:        145,
		Randomness:   sig2,
	}

	b2 := &Beacon{
		PreviousRand: sig2,
		Round:        146,
		Randomness:   sig1,
	}

	require.NoError(t, store.Put(b1))
	require.Equal(t, 1, store.Len())
	require.NoError(t, store.Put(b2))
	require.Equal(t, 2, store.Len())

	received, err := store.Last()
	require.NoError(t, err)
	require.Equal(t, b2, received)

	store.Close()
	store, err = NewBoltStore(path, nil)
	require.NoError(t, err)
	require.NoError(t, store.Put(b1))

	doneCh := make(chan bool)
	callback := func(b *Beacon) {
		require.Equal(t, b1, b)
		doneCh <- true
	}
	store = NewCallbackStore(store, callback)
	go store.Put(b1)
	select {
	case <-doneCh:
		return
	case <-time.After(50 * time.Millisecond):
		t.Fail()
	}
}
