//go:build memdb

package beacon

import (
	"testing"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/memdb"
	"github.com/drand/drand/log"
)

func createStore(_ *testing.T, _ log.Logger, _ *BeaconTest, _ int) (chain.Store, error) {
	return memdb.NewStore(10), nil
}
