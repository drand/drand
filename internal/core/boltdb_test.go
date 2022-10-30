//go:build !postgres && !memdb

package core

import (
	"testing"

	"github.com/drand/drand/internal/chain"
)

func WithTestDB(_ *testing.T, _ string) []ConfigOption {
	return []ConfigOption{
		WithDBStorageEngine(chain.BoltDB),
	}
}
