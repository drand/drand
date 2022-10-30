//go:build integration && !postgres && !memdb

package main_test

import (
	"testing"

	"github.com/drand/drand/internal/chain"
)

func withTestDB() chain.StorageType {
	return chain.BoltDB
}

func withPgDSN(_ *testing.T) func() string {
	return func() string {
		return ""
	}
}
