//go:build memdb

package beacon

import (
	"testing"

	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/internal/chain"
	"github.com/drand/drand/v2/internal/chain/memdb"
)

func createStore(_ *testing.T, _ log.Logger, _ *BeaconTest, _ int) (chain.Store, error) {
	return memdb.NewStore(10), nil
}
