//go:build !integration

package core

import (
	"testing"

	"github.com/drand/drand/chain"
)

func WithTestDB(_ *testing.T, _ string) []ConfigOption {
	return []ConfigOption{
		WithDBStorageEngine(chain.BoltDB),
	}
}
