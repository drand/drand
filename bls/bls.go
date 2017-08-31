package bls

import (
	"crypto/cipher"
	"errors"

	"github.com/dedis/drand/pbc"
	"gopkg.in/dedis/kyber.v1"
)

func NewKeyPair(s pbc.PairingSuite, r cipher.Stream) (kyber.Scalar, kyber.Point) {
	sk := s.G2().Scalar().Pick(r)
	pk := s.G2().Point().Mul(sk, nil)
	return sk, pk
}

// Performs a BLS signature operation. Namely, it computes:
//
//   x * H(m) as a point on G1
//
// where x is the private key, and m the message.
func Sign(s pbc.PairingSuite, private kyber.Scalar, msg []byte) []byte {
	HM := hashed(s, msg)
	xHM := HM.Mul(private, HM)
	sig, _ := xHM.MarshalBinary()
	return sig
}

// Verify checks the signature. Namely, it checks the equivalence between
//
//  e(H(m),X) == e(H(m), G2^x) == e(H(m)^x, G2) == e(s, G2)
//
// where m is the message, X the public key from G2, s the signature and G2 the base
// point from which the public key have been generated.
func Verify(s pbc.PairingSuite, public kyber.Point, msg, sig []byte) error {
	HM := hashed(s, msg)
	left := s.GT().PointGT().Pairing(HM, public)
	sigPoint := s.G1().Point()
	if err := sigPoint.UnmarshalBinary(sig); err != nil {
		return err
	}

	g2 := s.G2().Point().Base()
	right := s.GT().PointGT().Pairing(sigPoint, g2)

	if !left.Equal(right) {
		return errors.New("bls: invalid signature")
	}
	return nil
}

func hashed(s pbc.PairingSuite, msg []byte) kyber.Point {
	hashed := s.Hash().Sum(msg)
	p := s.G1().Point().Pick(s.Cipher(hashed))
	return p
}
