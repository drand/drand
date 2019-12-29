package sign

import (
	"crypto/cipher"

	"github.com/drand/kyber"
	"github.com/drand/kyber/share"
)

// Scheme is the minimal interface for a signature scheme.
// Implemented by BLS and TBLS
type Scheme interface {
	NewKeyPair(random cipher.Stream) (kyber.Scalar, kyber.Point)
	Sign(private kyber.Scalar, msg []byte) ([]byte, error)
	Verify(public kyber.Point, msg, sig []byte) error
}

// AggregatableScheme is an interface allowing to aggregate signatures and
// public keys to efficient verification.
type AggregatableScheme interface {
	Scheme
	AggregateSignatures(sigs ...[]byte) ([]byte, error)
	AggregatePublicKeys(Xs ...kyber.Point) kyber.Point
}

// ThresholdScheme is a threshold signature scheme that issues partial
// signatures and can recover a "full" signature. It is implemented by the tbls
// package.
// TODO: see any potential conflict or synergy with mask and policy
type ThresholdScheme interface {
	Sign(private *share.PriShare, msg []byte) ([]byte, error)
	Recover(public *share.PubPoly, msg []byte, sigs [][]byte, t, n int) ([]byte, error)
	VerifyPartial(public *share.PubPoly, msg, sig []byte) error
	VerifyRecovered(public kyber.Point, msg, sig []byte) error
}
