package chain

import (
	"crypto/sha256"

	"github.com/drand/drand/common/scheme"

	"github.com/drand/drand/key"
	"github.com/drand/kyber"
)

type Verifier struct {
	scheme scheme.Scheme
}

func NewVerifier(sch scheme.Scheme) *Verifier {
	return &Verifier{scheme: sch}
}

// MessageChained returns a slice of bytes as the message to sign or to verify
// alongside a beacon signature.
func (v Verifier) DigestMessage(currRound uint64, prevSig []byte) []byte {
	h := sha256.New()

	if !v.scheme.DecouplePrevSig {
		_, _ = h.Write(prevSig)
	}
	_, _ = h.Write(RoundToBytes(currRound))
	return h.Sum(nil)
}

// VerifyChainedBeacon returns an error if the given beacon does not verify given the
// public key. The public key "point" can be obtained from the
// `key.DistPublic.Key()` method. The distributed public is the one written in
// the configuration file of the network.
func (v Verifier) VerifyBeacon(b Beacon, pubkey kyber.Point) error {
	prevSig := b.PreviousSig
	round := b.Round

	msg := v.DigestMessage(round, prevSig)

	return key.Scheme.VerifyRecovered(pubkey, msg, b.Signature)
}
