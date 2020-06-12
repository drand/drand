package mock

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/drand/drand/chain"
)

// NewMockResult creates a mock result for testing.
func NewMockResult(round uint64) Result {
	sig := make([]byte, 8)
	binary.LittleEndian.PutUint64(sig, round)
	return Result{
		Rnd:  round,
		Sig:  sig,
		Rand: chain.RandomnessFromSignature(sig),
	}
}

// Result is a mock result that can be used for testing.
type Result struct {
	Rnd  uint64
	Rand []byte
	Sig  []byte
}

// Randomness is a hash of the signature.
func (r *Result) Randomness() []byte {
	return r.Rand
}

// Signature is the signature of the randomness for this round.
func (r *Result) Signature() []byte {
	return r.Sig
}

// Round is the round number for this random data.
func (r *Result) Round() uint64 {
	return r.Rnd
}

// AssertValid checks that this result is valid.
func (r *Result) AssertValid(t *testing.T) {
	t.Helper()
	sigTarget := make([]byte, 8)
	binary.LittleEndian.PutUint64(sigTarget, r.Rnd)
	if !bytes.Equal(r.Sig, sigTarget) {
		t.Fatalf("expected sig: %x, got %x", sigTarget, r.Sig)
	}
	randTarget := chain.RandomnessFromSignature(sigTarget)
	if !bytes.Equal(r.Rand, randTarget) {
		t.Fatalf("expected rand: %x, got %x", randTarget, r.Rand)
	}
}
