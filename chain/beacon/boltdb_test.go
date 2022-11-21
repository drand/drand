//go:build !postgres && !memdb

package beacon

import (
	"testing"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/boltdb"
	"github.com/drand/drand/log"
)

func createStore(_ *testing.T, l log.Logger, b *BeaconTest, idx int) (chain.Store, error) {
	return boltdb.NewBoltStore(l, b.paths[idx], nil)
}
