//go:build postgres

package beacon

import (
	"context"
	"fmt"
	"testing"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/postgresdb/pgdb"
	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/log"
	"github.com/drand/drand/test"
)

var c *test.Container

func TestMain(m *testing.M) {
	var err error
	c, err = test.StartPGDB()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer test.StopPGDB(c)

	m.Run()
}

func createStore(t *testing.T, l log.Logger, _ *BeaconTest, _ int) (chain.Store, error) {
	dbName := test.ComputeDBName()
	_, dbConn := test.NewUnit(t, c, dbName)

	ctx := context.Background()
	sch := scheme.GetSchemeFromEnv()
	if sch.ID == scheme.DefaultSchemeID {
		ctx = chain.SetPreviousRequiredOnContext(ctx)
	}

	return pgdb.NewStore(ctx, l, dbConn, dbName)
}
