//go:build !postgres && !memdb

package beacon

import (
	"context"
	"testing"

	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/internal/chain"
	"github.com/drand/drand/v2/internal/chain/boltdb"
	context2 "github.com/drand/drand/v2/internal/test/context"
)

func createStore(t *testing.T, l log.Logger, b *BeaconTest, idx int) (chain.Store, error) {
	ctx, _, _ := context2.PrevSignatureMattersOnContext(t, context.Background())
	return boltdb.NewBoltStore(ctx, l, b.paths[idx], nil)
}
