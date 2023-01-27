//go:build !postgres && !memdb

package beacon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/boltdb"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/log"
)

func createStore(t *testing.T, l log.Logger, b *BeaconTest, idx int) (chain.Store, error) {
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	if sch.Name == crypto.DefaultSchemeID {
		ctx = chain.SetPreviousRequiredOnContext(ctx)
	}
	return boltdb.NewBoltStore(ctx, l, b.paths[idx], nil)
}
