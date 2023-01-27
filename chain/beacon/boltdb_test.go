//go:build !postgres && !memdb

package beacon

import (
	"context"
	"testing"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/boltdb"
	"github.com/drand/drand/log"
	"github.com/drand/drand/test"
)

func createStore(t *testing.T, l log.Logger, b *BeaconTest, idx int) (chain.Store, error) {
	ctx, _, _ := test.PrevSignatureMatersOnContext(t, context.Background())
	return boltdb.NewBoltStore(ctx, l, b.paths[idx], nil)
}
