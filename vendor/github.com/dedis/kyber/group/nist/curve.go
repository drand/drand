// +build vartime

package nist

import (
	"crypto/cipher"
	"crypto/elliptic"
	"errors"
	"io"
	"math/big"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/group/internal/marshalling"
	"github.com/dedis/kyber/group/mod"
	"github.com/dedis/kyber/util/random"
)

type curvePoint struct {
	x, y *big.Int
	c    *curve
}

func (p *curvePoint) String() string {
	return "(" + p.x.String() + "," + p.y.String() + ")"
}

func (p *curvePoint) Equal(p2 kyber.Point) bool {
	cp2 := p2.(*curvePoint)

	// Make sure both coordinates are normalized.
	// Apparently Go's elliptic curve code doesn't always ensure this.
	M := p.c.p.P
	p.x.Mod(p.x, M)
	p.y.Mod(p.y, M)
	cp2.x.Mod(cp2.x, M)
	cp2.y.Mod(cp2.y, M)

	return p.x.Cmp(cp2.x) == 0 && p.y.Cmp(cp2.y) == 0
}

func (p *curvePoint) Null() kyber.Point {
	p.x = new(big.Int).SetInt64(0)
	p.y = new(big.Int).SetInt64(0)
	return p
}

func (p *curvePoint) Base() kyber.Point {
	p.x = p.c.p.Gx
	p.y = p.c.p.Gy
	return p
}

func (p *curvePoint) Valid() bool {
	// The IsOnCurve function in Go's elliptic curve package
	// doesn't consider the point-at-infinity to be "on the curve"
	return p.c.IsOnCurve(p.x, p.y) ||
		(p.x.Sign() == 0 && p.y.Sign() == 0)
}

// Try to generate a point on this curve from a chosen x-coordinate,
// with a random sign.
func (p *curvePoint) genPoint(x *big.Int, rand cipher.Stream) bool {

	// Compute the corresponding Y coordinate, if any
	y2 := new(big.Int).Mul(x, x)
	y2.Mul(y2, x)
	threeX := new(big.Int).Lsh(x, 1)
	threeX.Add(threeX, x)
	y2.Sub(y2, threeX)
	y2.Add(y2, p.c.p.B)
	y2.Mod(y2, p.c.p.P)
	y := p.c.sqrt(y2)

	// Pick a random sign for the y coordinate
	b := make([]byte, 1)
	rand.XORKeyStream(b, b)
	if (b[0] & 0x80) != 0 {
		y.Sub(p.c.p.P, y)
	}

	// Check that it's a valid point
	y2t := new(big.Int).Mul(y, y)
	y2t.Mod(y2t, p.c.p.P)
	if y2t.Cmp(y2) != 0 {
		return false // Doesn't yield a valid point!
	}

	p.x = x
	p.y = y
	return true
}

func (p *curvePoint) EmbedLen() int {
	// Reserve at least 8 most-significant bits for randomness,
	// and the least-significant 8 bits for embedded data length.
	// (Hopefully it's unlikely we'll need >=2048-bit curves soon.)
	return (p.c.p.P.BitLen() - 8 - 8) / 8
}

func (p *curvePoint) Pick(rand cipher.Stream) kyber.Point {
	return p.Embed(nil, rand)
}

// Pick a curve point containing a variable amount of embedded data.
// Remaining bits comprising the point are chosen randomly.
func (p *curvePoint) Embed(data []byte, rand cipher.Stream) kyber.Point {

	l := p.c.coordLen()
	dl := p.EmbedLen()
	if dl > len(data) {
		dl = len(data)
	}

	for {
		b := random.Bits(uint(p.c.p.P.BitLen()), false, rand)
		if data != nil {
			b[l-1] = byte(dl)         // Encode length in low 8 bits
			copy(b[l-dl-1:l-1], data) // Copy in data to embed
		}
		if p.genPoint(new(big.Int).SetBytes(b), rand) {
			return p
		}
	}
}

// Extract embedded data from a curve point
func (p *curvePoint) Data() ([]byte, error) {
	b := p.x.Bytes()
	l := p.c.coordLen()
	if len(b) < l { // pad leading zero bytes if necessary
		b = append(make([]byte, l-len(b)), b...)
	}
	dl := int(b[l-1])
	if dl > p.EmbedLen() {
		return nil, errors.New("invalid embedded data length")
	}
	return b[l-dl-1 : l-1], nil
}

func (p *curvePoint) Add(a, b kyber.Point) kyber.Point {
	ca := a.(*curvePoint)
	cb := b.(*curvePoint)
	p.x, p.y = p.c.Add(ca.x, ca.y, cb.x, cb.y)
	return p
}

func (p *curvePoint) Sub(a, b kyber.Point) kyber.Point {
	ca := a.(*curvePoint)
	cb := b.(*curvePoint)

	cbn := p.c.Point().Neg(cb).(*curvePoint)
	p.x, p.y = p.c.Add(ca.x, ca.y, cbn.x, cbn.y)
	return p
}

func (p *curvePoint) Neg(a kyber.Point) kyber.Point {

	s := p.c.Scalar().One()
	s.Neg(s)
	return p.Mul(s, a).(*curvePoint)
}

func (p *curvePoint) Mul(s kyber.Scalar, b kyber.Point) kyber.Point {
	cs := s.(*mod.Int)
	if b != nil {
		cb := b.(*curvePoint)
		p.x, p.y = p.c.ScalarMult(cb.x, cb.y, cs.V.Bytes())
	} else {
		p.x, p.y = p.c.ScalarBaseMult(cs.V.Bytes())
	}
	return p
}

func (p *curvePoint) MarshalSize() int {
	coordlen := (p.c.Params().BitSize + 7) >> 3
	return 1 + 2*coordlen // uncompressed ANSI X9.62 representation
}

func (p *curvePoint) MarshalBinary() ([]byte, error) {
	return elliptic.Marshal(p.c, p.x, p.y), nil
}

func (p *curvePoint) UnmarshalBinary(buf []byte) error {
	// Check whether all bytes after first one are 0, so we
	// just return the initial point. Read everything to
	// prevent timing-leakage.
	var c byte = 0
	for _, b := range buf[1:] {
		c |= b
	}
	if c != 0 {
		p.x, p.y = elliptic.Unmarshal(p.c, buf)
		if p.x == nil || !p.Valid() {
			return errors.New("invalid elliptic curve point")
		}
	} else {
		// All bytes are 0, so we initialize x and y
		p.x = big.NewInt(0)
		p.y = big.NewInt(0)
	}
	return nil
}

func (p *curvePoint) MarshalTo(w io.Writer) (int, error) {
	return marshalling.PointMarshalTo(p, w)
}

func (p *curvePoint) UnmarshalFrom(r io.Reader) (int, error) {
	return marshalling.PointUnmarshalFrom(p, r)
}

// interface for curve-specifc mathematical functions
type curveOps interface {
	sqrt(y *big.Int) *big.Int
}

// Curve is an implementation of the kyber.Group interface
// for NIST elliptic curves, built on Go's native elliptic curve library.
type curve struct {
	elliptic.Curve
	curveOps
	p *elliptic.CurveParams
}

// Return the number of bytes in the encoding of a Scalar for this curve.
func (c *curve) ScalarLen() int { return (c.p.N.BitLen() + 7) / 8 }

// Create a Scalar associated with this curve. The scalars created by
// this package implement kyber.Scalar's SetBytes method, interpreting
// the bytes as a big-endian integer, so as to be compatible with the
// Go standard library's big.Int type.
func (c *curve) Scalar() kyber.Scalar {
	return mod.NewInt64(0, c.p.N)
}

// Number of bytes required to store one coordinate on this curve
func (c *curve) coordLen() int {
	return (c.p.BitSize + 7) / 8
}

// Return the number of bytes in the encoding of a Point for this curve.
// Currently uses uncompressed ANSI X9.62 format with both X and Y coordinates;
// this could change.
func (c *curve) PointLen() int {
	return 1 + 2*c.coordLen() // ANSI X9.62: 1 header byte plus 2 coords
}

// Create a Point associated with this curve.
func (c *curve) Point() kyber.Point {
	p := new(curvePoint)
	p.c = c
	return p
}

func (p *curvePoint) Set(P kyber.Point) kyber.Point {
	p.x = P.(*curvePoint).x
	p.y = P.(*curvePoint).y
	return p
}

func (p *curvePoint) Clone() kyber.Point {
	return &curvePoint{x: p.x, y: p.y, c: p.c}
}

// Return the order of this curve: the prime N in the curve parameters.
func (c *curve) Order() *big.Int {
	return c.p.N
}
