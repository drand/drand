package mock

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/sign/tbls"
	"github.com/drand/kyber/util/random"
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
	PSig []byte
}

// Randomness is a hash of the signature.
func (r *Result) Randomness() []byte {
	return r.Rand
}

// Signature is the signature of the randomness for this round.
func (r *Result) Signature() []byte {
	return r.Sig
}

// PreviousSignature is the signature of the previous round.
func (r *Result) PreviousSignature() []byte {
	return r.PSig
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

func sha256Hash(in []byte) []byte {
	h := sha256.New()
	h.Write(in)
	return h.Sum(nil)
}

func roundToBytes(r int) []byte {
	var buff bytes.Buffer
	binary.Write(&buff, binary.BigEndian, uint64(r))
	return buff.Bytes()
}

// VerifiableResults creates a set of results that will pass a `chain.Verify` check.
func VerifiableResults(count int) (*chain.Info, []Result) {
	secret := key.KeyGroup.Scalar().Pick(random.New())
	public := key.KeyGroup.Point().Mul(secret, nil)
	previous := make([]byte, 32)
	if _, err := rand.Reader.Read(previous); err != nil {
		panic(err)
	}

	out := make([]Result, count)
	for i := range out {
		msg := sha256Hash(append(previous[:], roundToBytes(i+1)...))
		sshare := share.PriShare{I: 0, V: secret}
		tsig, err := key.Scheme.Sign(&sshare, msg)
		if err != nil {
			panic(err)
		}
		tshare := tbls.SigShare(tsig)
		sig := tshare.Value()

		out[i] = Result{
			Sig:  sig,
			PSig: previous,
			Rnd:  uint64(i + 1),
			Rand: chain.RandomnessFromSignature(sig),
		}
		previous = make([]byte, len(sig))
		copy(previous[:], sig)
	}
	info := chain.Info{
		PublicKey:   public,
		Period:      time.Second,
		GenesisTime: time.Now().Unix() - int64(count),
		GroupHash:   out[0].PSig,
	}

	return &info, out
}
