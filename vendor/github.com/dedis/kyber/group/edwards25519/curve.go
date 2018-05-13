package edwards25519

import (
	"crypto/cipher"
	"crypto/sha512"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/util/random"
)

// Curve represents the Ed25519 group.
// There are no parameters and no initialization is required
// because it supports only this one specific curve.
type Curve struct {
}

// Return the name of the curve, "Ed25519".
func (c *Curve) String() string {
	return "Ed25519"
}

// ScalarLen returns 32, the size in bytes of an encoded Scalar
// for the Ed25519 curve.
func (c *Curve) ScalarLen() int {
	return 32
}

// Scalar creates a new Scalar for the prime-order subgroup of the Ed25519 curve.
// The scalars in this package implement kyber.Scalar's SetBytes
// method, interpreting the bytes as a little-endian integer, in order to remain
// compatible with other Ed25519 implementations, and with the standard implementation
// of the EdDSA signature.
func (c *Curve) Scalar() kyber.Scalar {
	return &scalar{curve: c}
}

// PointLen returns 32, the size in bytes of an encoded Point on the Ed25519 curve.
func (c *Curve) PointLen() int {
	return 32
}

// Point creates a new Point on the Ed25519 curve.
func (c *Curve) Point() kyber.Point {
	P := new(point)
	P.curve = c
	return P
}

// NewKey returns a formatted Ed25519 key (avoiding subgroup attack by requiring
// it to be a multiple of 8). NewKey implements the kyber/util/key.Generator interface.
func (c *Curve) NewKey(stream cipher.Stream) kyber.Scalar {
	var buffer [32]byte
	random.Bytes(buffer[:], stream)
	scalar := sha512.Sum512(buffer[:])
	scalar[0] &= 0xf8
	scalar[31] &= 0x3f
	scalar[31] |= 0x40

	secret := c.Scalar().SetBytes(scalar[:32])
	return secret
}
