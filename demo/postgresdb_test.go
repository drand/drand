//go:build integration

package main_test

import (
	"fmt"
	"testing"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/test"
)

var c *test.Container

func TestMain(m *testing.M) {
	var err error
	c, err = test.StartDB()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer test.StopDB(c)

	m.Run()
}

func withTestDB() chain.StorageType {
	return chain.PostgreSQL
}

func withPgDSN(t *testing.T) func() string {
	return func() string {
		dbName := test.ComputeDBName()

		dsn := fmt.Sprintf(
			"postgres://postgres:postgres@%s/%s?sslmode=disable&timeout=5&connect_timeout=5",
			c.Host,
			dbName,
		)

		withTestDBUnit(t, dbName)

		t.Logf("*** created database: %s\n", dsn)

		return dsn
	}
}

func withTestDBUnit(t *testing.T, dbName string) {
	_, _, cleanup := test.NewUnit(t, c, dbName)
	t.Cleanup(cleanup)
}
