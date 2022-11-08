package beacon

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/boltdb"
	"github.com/drand/drand/log"
)

func TestStoreCallback(t *testing.T) {
	dir := t.TempDir()
	l := log.NewLogger(nil, log.LogDebug)
	bbstore, err := boltdb.NewBoltStore(l, dir, nil)
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
