//go:build postgres

package beacon

import (
	"context"
	"fmt"
	"testing"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/postgresdb/pgdb"
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
	_, dbConn, _ := test.NewUnit(t, c, dbName)
	return pgdb.NewStore(context.Background(), l, dbConn, dbName)
}
