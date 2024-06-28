//go:build postgres

package core

import (
	"fmt"
	"testing"

	"github.com/drand/drand/internal/chain"
	"github.com/drand/drand/internal/test"
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

func withTestDB(t *testing.T, dbName string) ConfigOption {
	return func(cfg *Config) {
		_, conn := test.NewUnit(t, c, dbName)
		cfg.pgConn = conn
	}
}

func WithTestDB(t *testing.T, dbName string) []ConfigOption {
	return []ConfigOption{
		WithDBStorageEngine(chain.PostgreSQL),
		withTestDB(t, dbName),
	}
}
