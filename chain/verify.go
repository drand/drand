package chain

import (
	"crypto/sha256"

	"github.com/drand/drand/common/scheme"

	"github.com/drand/drand/key"
	"github.com/drand/kyber"
)

// Verifier allows verifying the beacons signature based on a scheme.
type Verifier struct {
	// scheme holds a set of values the verifying process will use to act in specific ways, regarding signature verification, etc
	scheme scheme.Scheme
}

func NewVerifier(sch scheme.Scheme) *Verifier {
	return &Verifier{scheme: sch}
}

// DigestMessage returns a slice of bytes as the message to sign or to verify
// alongside a beacon signature.
func (v Verifier) DigestMessage(currRound uint64, prevSig []byte) []byte {
	h := sha256.New()

	if !v.scheme.DecouplePrevSig {
		_, _ = h.Write(prevSig)
	}
	_, _ = h.Write(RoundToBytes(currRound))
	return h.Sum(nil)
}

// VerifyBeacon returns an error if the given beacon does not verify given the
// public key. The public key "point" can be obtained from the
// `key.DistPublic.Key()` method. The distributed public is the one written in
// the configuration file of the network.
func (v Verifier) VerifyBeacon(b Beacon, pubkey kyber.Point) error {
	prevSig := b.PreviousSig
	round := b.Round

	msg := v.DigestMessage(round, prevSig)

	return key.Scheme.VerifyRecovered(pubkey, msg, b.Signature)
}

func (v Verifier) IsPrevSigMeaningful() bool {
	return !v.scheme.DecouplePrevSig
}
