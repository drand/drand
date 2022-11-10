//go:build integration

package main_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

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
	return chain.PostgresSQL
}

func withPgDSN(t *testing.T) func() string {
	return func() string {
		dbName := computeDBName(t)

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

// computeDBName helps generate new, unique database names during the runtime of a test.
// By adding the time, with milliseconds, we can avoid this, e.g. testbroadcast_09223736225
func computeDBName(t *testing.T) string {
	t.Helper()

	suffix := strings.Replace(time.Now().Format("02150405.000"), ".", "", -1)
	dbName := fmt.Sprintf("%s_%s", t.Name(), suffix)
	return dbName
	// return strings.ToLower(dbName)
}
