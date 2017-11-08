package proof

// A clique protocol is a kyber.on for a cryptographic protocol
// in which every participant knows about and interacts directly
// in lock-step with every other participant in the clique.
// Clique protocols are suitable for small-scale groups,
// such as "boards of trustees" chosen from larger groups.
//
// The basic clique protocol
// assumes that nodes are always "live" and never go offline,
// but we can achieve availability via threshold kyber.

import "github.com/dedis/kyber"

// Protocol represents the role of a participant in a clique protocol.
// A participant is represented as a higher-order function taking a StarContext,
// which invokes the StarContext's methods to send and receive messages,
// and finally returns once the protocol has concluded for all participants.
// Returns a slice of success/error indicators, one for each participant.
//
type Protocol func(ctx Context) []error

// Context represents a kyber.context for running a clique protocol.
// A clique protocol is initiated by a leader
// but incorporates a variable number of followers,
// all of whom operate in lock-step under the leader's direction.
// At each step, each follower produces one message;
// the leader aggregates all the followers' messages for that step
// and returns the vector of collected messages to each follower.
// Followers can drop out or be absent at any step, in which case
// they are seen as contributing an empty message in that step.
type Context interface {

	// A follower calls Step to provide its message for the next step,
	// and wait for the leader to collect and distribute all messages.
	// Returns the list of collected messages, one per participant.
	// The returned message slice is positionally consistent across steps:
	// each index consistently represents the same participant every step.
	// One returned message will be the same slice as the one passed in,
	// representing the calling participant's own slot.
	Step(msg []byte) ([][]byte, error)

	// Get a source of private cryptographic randomness.
	Random() kyber.Cipher
}
