package client

import (
	"context"
	"encoding"
	"time"
)

// Client represents the drand Client interface.
type Client interface {
	// TextMarshaler (the 'MarshalText' method) allows serialization of the client
	// state for subsequent restoration. The marshalled value can be passed to `New`
	// to restore Client state.
	encoding.TextMarshaler

	// Get returns a the randomness at `round` or an error.
	// Requesting round = 0 will return randomness for the most
	// recent known round, bounded at a minimum to the `RoundAt(time.Now())`
	Get(ctx context.Context, round uint64) (Result, error)

	// Watch returns new randomness as it becomes available.
	Watch(ctx context.Context) <-chan Result

	// RoundAt will return the most recent round of randomness that will be available
	// at time for the current client.
	RoundAt(time time.Time) uint64
}

// Result represents the randomness for a single drand round.
type Result struct {
	Round     uint64
	Signature []byte
}
