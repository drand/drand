//go:build integration && memdb

package main_test

import (
	"testing"

	"github.com/drand/drand/chain"
)

func withTestDB() chain.StorageType {
	return chain.MemDB
}

func withPgDSN(_ *testing.T) func() string {
	return func() string {
		return ""
	}
}
