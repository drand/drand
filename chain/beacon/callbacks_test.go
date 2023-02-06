package beacon

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/boltdb"
	"github.com/drand/drand/test"
	context2 "github.com/drand/drand/test/context"
)

func TestStoreCallback(t *testing.T) {
	dir := t.TempDir()
	ctx, _, _ := context2.PrevSignatureMattersOnContext(t, context.Background())
	l := test.Logger(t)
	bbstore, err := boltdb.NewBoltStore(ctx, l, dir, nil)
	require.NoError(t, err)
	cb := NewCallbackStore(bbstore)
	id1 := "superid"
	doneCh := make(chan bool, 1)
	cb.AddCallback(id1, func(b *chain.Beacon, closed bool) {
		if closed {
			return
		}
		doneCh <- true
	})

	cb.Put(ctx, &chain.Beacon{
		Round: 1,
	})
	require.True(t, checkOne(doneCh))

	cb.AddCallback(id1, func(*chain.Beacon, bool) {})
	cb.Put(ctx, &chain.Beacon{
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
