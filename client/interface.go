package client

import (
	"encoding"

	"github.com/libp2p/go-libp2p-core/event"
)

// Client represents the drand Client interface.
type Client interface {
	// TextMarshaler (the 'MarshalText' method) allows serialization of the client
	// state for subsequent restoration. The marshalled value can be passed to `New`
	// to restore Client state.
	encoding.TextMarshaler

	// Get returns a the randomness at `round` or an error over a result channel.
	// One result will be returned, with exactly one of Err or Data populated.
	func Get(ctx context.Context, round uint64) <-chan Result

	// RoundAt will return the most recent round of randomness that will be available
	// at time for the current client.
	func RoundAt(time time.Time) uint64
}

// Result represents the randomness for a single drand round.
type Result struct {
	Err error
	Round uint64
	Data []byte
}

// New creates a new Client, or a non-nill error if the input state cannot be parsed.
func New(state []byte) (Client, error)
