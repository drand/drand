//go:build !postgres && !memdb

package beacon

import (
	"context"
	"testing"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/boltdb"
	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/log"
)

func createStore(_ *testing.T, l log.Logger, b *BeaconTest, idx int) (chain.Store, error) {
	ctx := context.Background()
	sch := scheme.GetSchemeFromEnv()
	if sch.ID == scheme.DefaultSchemeID {
		ctx = chain.SetPreviousRequiredOnContext(ctx)
	}
	return boltdb.NewBoltStore(ctx, l, b.paths[idx], nil)
}
