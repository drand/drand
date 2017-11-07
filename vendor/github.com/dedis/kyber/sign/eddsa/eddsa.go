// Package eddsa implements the EdDSA signature algorithm according to
// the RFC https://tools.ietf.org/html/draft-josefsson-eddsa-ed25519-02
package eddsa

import (
	"crypto/cipher"
	"crypto/sha512"
	"errors"
	"fmt"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/group/edwards25519"
	"github.com/dedis/kyber/util/random"
)

var group = new(edwards25519.Curve)

// EdDSA implements the EdDSA signature algorithm according to
// the RFC https://tools.ietf.org/html/draft-josefsson-eddsa-ed25519-02
type EdDSA struct {
	seed   []byte
	prefix []byte
	// Secret being already hashed + bit tweaked
	Secret kyber.Scalar
	// Public is the corresponding public key
	Public kyber.Point
}

// NewEdDSA will return a freshly generated key pair to use for generating
// EdDSA signatures.
// If stream == nil, it will take the random.Stream.
func NewEdDSA(stream cipher.Stream) *EdDSA {
	if stream == nil {
		stream = random.Stream
	}
	buffer := random.NonZeroBytes(32, stream)

	scalar := hashSeed(buffer)

	secret := group.Scalar().SetBytes(scalar[:32])
	public := group.Point().Mul(secret, nil)

	return &EdDSA{
		seed:   buffer,
		prefix: scalar[32:],
		Secret: secret,
		Public: public,
	}
}

// Prefix returns the Prefix as being the right part of
// the hashed seed
func (e *EdDSA) Prefix() []byte {
	c := make([]byte, len(e.prefix))
	copy(c, e.prefix)
	return c
}

// MarshalBinary will return the representation used by
// the reference implementation of SUPERCOP ref10
// Namely seed || Public
func (e *EdDSA) MarshalBinary() ([]byte, error) {
	pBuff, err := e.Public.MarshalBinary()
	if err != nil {
		return nil, err
	}

	eddsa := make([]byte, 64)
	copy(eddsa, e.seed)
	copy(eddsa[32:], pBuff)
	return eddsa, nil
}

//UnmarshalBinary transforms a slice of bytes into a EdDSA signature
func (e *EdDSA) UnmarshalBinary(buff []byte) error {
	if len(buff) != 64 {
		return errors.New("wrong length for decoding EdDSA private")
	}

	e.seed = buff[:32]
	scalar := hashSeed(e.seed)
	e.prefix = scalar[32:]
	e.Secret = group.Scalar().SetBytes(scalar[:32])
	e.Public = group.Point().Mul(e.Secret, nil)
	return nil
}

// Sign will return a EdDSA signature of the message msg using Ed25519.
// NOTE: Code taken from the Python implementation from the RFC
// https://tools.ietf.org/html/draft-josefsson-eddsa-ed25519-02
func (e *EdDSA) Sign(msg []byte) ([]byte, error) {
	hash := sha512.New()
	_, _ = hash.Write(e.prefix)
	_, _ = hash.Write(msg)

	// deterministic random secret and its commit
	r := group.Scalar().SetBytes(hash.Sum(nil))
	R := group.Point().Mul(r, nil)

	// challenge
	// H( R || Public || Msg)
	hash.Reset()
	Rbuff, err := R.MarshalBinary()
	if err != nil {
		return nil, err
	}
	Abuff, err := e.Public.MarshalBinary()
	if err != nil {
		return nil, err
	}

	_, _ = hash.Write(Rbuff)
	_, _ = hash.Write(Abuff)
	_, _ = hash.Write(msg)

	h := group.Scalar().SetBytes(hash.Sum(nil))

	// response
	// s = r + h * s
	s := group.Scalar().Mul(e.Secret, h)
	s.Add(r, s)

	sBuff, err := s.MarshalBinary()
	if err != nil {
		return nil, err
	}

	// return R || s
	var sig [64]byte
	copy(sig[:], Rbuff)
	copy(sig[32:], sBuff)

	return sig[:], nil
}

// Verify takes a signature issued by EdDSA.Sign and
// return nil if it is a valid signature, or an error otherwise
// Takes:
//  - public key used in signing
//  - msg is the message to sign
//  - sig is the signature return by EdDSA.Sign
func Verify(public kyber.Point, msg, sig []byte) error {
	if len(sig) != 64 {
		return errors.New("signature length invalid")
	}

	R := group.Point()
	if err := R.UnmarshalBinary(sig[:32]); err != nil {
		return fmt.Errorf("got R invalid point: %s", err)
	}

	s := group.Scalar()
	if err := s.UnmarshalBinary(sig[32:]); err != nil {
		return fmt.Errorf("schnorr: s invalid scalar %s", err)
	}

	// reconstruct h = H(R || Public || Msg)
	Pbuff, err := public.MarshalBinary()
	if err != nil {
		return err
	}
	hash := sha512.New()
	_, _ = hash.Write(sig[:32])
	_, _ = hash.Write(Pbuff)
	_, _ = hash.Write(msg)

	h := group.Scalar().SetBytes(hash.Sum(nil))
	// reconstruct S == k*A + R
	S := group.Point().Mul(s, nil)
	hA := group.Point().Mul(h, public)
	RhA := group.Point().Add(R, hA)

	if !RhA.Equal(S) {
		return errors.New("reconstructed S is not equal to signature")
	}
	return nil
}

func hashSeed(seed []byte) (hash [64]byte) {
	hash = sha512.Sum512(seed)
	hash[0] &= 0xf8
	hash[31] &= 0x3f
	hash[31] |= 0x40
	return
}
