package beacon

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/boltdb"
	"github.com/stretchr/testify/require"
)

func TestStoreCallback(t *testing.T) {
	dir, err := ioutil.TempDir("", "*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)
	bbstore, err := boltdb.NewBoltStore(dir, nil)
	require.NoError(t, err)
	cb := NewCallbackStore(bbstore)
	id1 := "superid"
	doneCh := make(chan bool, 1)
	cb.AddCallback(id1, func(b *chain.Beacon) {
		doneCh <- true
	})

	cb.Put(&chain.Beacon{
		Round: 1,
	})
	require.True(t, checkOne(doneCh))
	cb.AddCallback(id1, func(*chain.Beacon) {})
	cb.Put(&chain.Beacon{
		Round: 1,
	})
	require.False(t, checkOne(doneCh))

	cb.RemoveCallback(id1)
	require.False(t, checkOne(doneCh))
}

func checkOne(ch chan bool) bool {
	select {
	case <-ch:
		return true
	case <-time.After(100 * time.Millisecond):
		return false
	}
}
