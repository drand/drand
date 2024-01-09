//go:build integration && postgres

package main_test

import (
	"fmt"
	"testing"

	"github.com/drand/drand/v2/internal/chain"
	"github.com/drand/drand/v2/internal/test"
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

func withTestDB() chain.StorageType {
	return chain.PostgreSQL
}

func withPgDSN(t *testing.T) func() string {
	return func() string {
		dbName := test.ComputeDBName()

		dsn := fmt.Sprintf(
			"postgres://postgres:postgres@%s/%s?sslmode=disable&connect_timeout=5",
			c.Host,
			dbName,
		)

		withTestDBUnit(t, dbName)

		t.Logf("*** created database: %s\n", dsn)

		return dsn
	}
}

func withTestDBUnit(t *testing.T, dbName string) {
	_, _ = test.NewUnit(t, c, dbName)
}
