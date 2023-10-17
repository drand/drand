package beacon

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/common"
	"github.com/drand/drand/common/testlogger"
	"github.com/drand/drand/internal/chain/boltdb"
	context2 "github.com/drand/drand/internal/test/context"
)

func TestStoreCallback(t *testing.T) {
	dir := t.TempDir()
	ctx, _, _ := context2.PrevSignatureMattersOnContext(t, context.Background())
	l := testlogger.New(t)
	bbstore, err := boltdb.NewBoltStore(ctx, l, dir, nil)
	require.NoError(t, err)
	cb := NewCallbackStore(l, bbstore)
	id1 := "superid"
	doneCh := make(chan bool, 1)
	cb.AddCallback(id1, func(b *common.Beacon, closed bool) {
		if closed {
			return
		}
		doneCh <- true
	})

	err = cb.Put(ctx, &common.Beacon{
		Round: 1,
	})
	require.NoError(t, err)
	require.True(t, checkOne(doneCh))

	cb.AddCallback(id1, func(*common.Beacon, bool) {})
	err = cb.Put(ctx, &common.Beacon{
		Round: 1,
	})
	require.NoError(t, err)
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
