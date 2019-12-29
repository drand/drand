// Package bls implements the Boneh-Lynn-Shacham (BLS) signature scheme which
// was introduced in the paper "Short Signatures from the Weil Pairing". BLS
// requires pairing-based cryptography.
//
// Deprecated: This version is vulnerable to rogue public-key attack and the
// new version of the protocol should be used to make sure a signature
// aggregate cannot be verified by a forged key. You can find the protocol
// in kyber/sign/bdn. Note that only the aggregation is broken against the
// attack and a later version will merge bls and asmbls.
//
// See the paper: https://crypto.stanford.edu/~dabo/pubs/papers/BLSmultisig.html
package bls

import (
	"crypto/cipher"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/drand/kyber"
	"github.com/drand/kyber/pairing"
	"github.com/drand/kyber/sign"
)

type hashablePoint interface {
	Hash([]byte) kyber.Point
}

type scheme struct {
	sigGroup kyber.Group
	keyGroup kyber.Group
	pairing  func(signature, public, hashedPoint kyber.Point) bool
}

// NewSchemeOnG1 returns a sign.Scheme that uses G1 for its signature space and G2
// for its public keys
func NewSchemeOnG1(suite pairing.Suite) sign.AggregatableScheme {
	sigGroup := suite.G1()
	keyGroup := suite.G2()
	pairing := func(public, hashedMsg, sigPoint kyber.Point) bool {
		// e ( H(m) , g^x)
		left := suite.Pair(hashedMsg, public)
		// e ( H(m)^x , g)
		right := suite.Pair(sigPoint, keyGroup.Point().Base())
		return left.Equal(right)
	}
	return &scheme{
		sigGroup: sigGroup,
		keyGroup: keyGroup,
		pairing:  pairing,
	}
}

// NewSchemeOnG2 returns a sign.Scheme that uses G2 for its signature space and
// G1 for its public key
func NewSchemeOnG2(suite pairing.Suite) sign.AggregatableScheme {
	sigGroup := suite.G2()
	keyGroup := suite.G1()
	pairing := func(public, hashedMsg, sigPoint kyber.Point) bool {
		// e (g^x, H(m))
		left := suite.Pair(public, hashedMsg)
		// e( g, H(m)^x)
		right := suite.Pair(keyGroup.Point().Base(), sigPoint)
		return left.Equal(right)
	}
	return &scheme{
		sigGroup: sigGroup,
		keyGroup: keyGroup,
		pairing:  pairing,
	}
}

func (s *scheme) NewKeyPair(random cipher.Stream) (kyber.Scalar, kyber.Point) {
	secret := s.keyGroup.Scalar().Pick(random)
	public := s.keyGroup.Point().Mul(secret, nil)
	return secret, public
}

func (s *scheme) Sign(private kyber.Scalar, msg []byte) ([]byte, error) {
	hashable, ok := s.sigGroup.Point().(hashablePoint)
	if !ok {
		return nil, errors.New("point needs to implement hashablePoint")
	}
	HM := hashable.Hash(msg)
	xHM := HM.Mul(private, HM)

	sig, err := xHM.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return sig, nil
}

func (s *scheme) Verify(X kyber.Point, msg, sig []byte) error {
	hashable, ok := s.sigGroup.Point().(hashablePoint)
	if !ok {
		return errors.New("bls: point needs to implement hashablePoint")
	}
	HM := hashable.Hash(msg)
	sigPoint := s.sigGroup.Point()
	if err := sigPoint.UnmarshalBinary(sig); err != nil {
		return err
	}
	if !s.pairing(X, HM, sigPoint) {
		return errors.New("bls: invalid signature")
	}
	return nil
}

func (s *scheme) AggregateSignatures(sigs ...[]byte) ([]byte, error) {
	sig := s.sigGroup.Point()
	for _, sigBytes := range sigs {
		sigToAdd := s.sigGroup.Point()
		if err := sigToAdd.UnmarshalBinary(sigBytes); err != nil {
			return nil, err
		}
		sig.Add(sig, sigToAdd)
	}
	return sig.MarshalBinary()
}

func (s *scheme) AggregatePublicKeys(Xs ...kyber.Point) kyber.Point {
	aggregated := s.keyGroup.Point()
	for _, X := range Xs {
		aggregated.Add(aggregated, X)
	}
	return aggregated
}

// BatchVerify verifies a large number of publicKey/msg pairings with a single aggregated signature.
// Since aggregation is generally much faster than verification, this can be a speed enhancement.
// Benchmarks show a roughly 50% performance increase over individual signature verification
// Every msg must be unique or there is the possibility to accept an invalid signature
// see: https://crypto.stackexchange.com/questions/56288/is-bls-signature-scheme-strongly-unforgeable/56290
// for a description of why each message must be unique.
func BatchVerify(suite pairing.Suite, publics []kyber.Point, msgs [][]byte, sig []byte) error {
	if !distinct(msgs) {
		return fmt.Errorf("bls: error, messages must be distinct")
	}

	s := suite.G1().Point()
	if err := s.UnmarshalBinary(sig); err != nil {
		return err
	}

	var aggregatedLeft kyber.Point
	for i := range msgs {
		hashable, ok := suite.G1().Point().(hashablePoint)
		if !ok {
			return errors.New("bls: point needs to implement hashablePoint")
		}
		hm := hashable.Hash(msgs[i])
		pair := suite.Pair(hm, publics[i])

		if i == 0 {
			aggregatedLeft = pair
		} else {
			aggregatedLeft.Add(aggregatedLeft, pair)
		}
	}

	right := suite.Pair(s, suite.G2().Point().Base())
	if !aggregatedLeft.Equal(right) {
		return errors.New("bls: invalid signature")
	}
	return nil
}

func distinct(msgs [][]byte) bool {
	m := make(map[[32]byte]bool)
	for _, msg := range msgs {
		h := sha256.Sum256(msg)
		if m[h] {
			return false
		}
		m[h] = true
	}
	return true
}
