// +build experimental

package openssl

// #include <openssl/ec.h>
// #include <openssl/bn.h>
// #include <openssl/obj_mac.h>
//
// struct bignum_ctx {
// };
//
// struct ec_group_st {		// CGo doesn't like undefined C structs
// };
//
// struct ec_point_st {
// };
//
import "C"

import (
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"runtime"
	"unsafe"

	"github.com/dedis/kyber/abstract"
	"github.com/dedis/kyber/group"
)

type point struct {
	p *_Ctype_struct_ec_point_st
	g *_Ctype_struct_ec_group_st
	c *curve
}

type curve struct {
	ctx          *_Ctype_struct_bignum_ctx
	g            *_Ctype_struct_ec_group_st
	p, n, cofact *bignum
	plen, nlen   int
	name         string
	null         *point
}

func newPoint(c *curve) *point {
	p := new(point)
	p.c = c
	p.g = c.g
	p.p = C.EC_POINT_new(c.g)
	runtime.SetFinalizer(p, freePoint)
	return p
}

func freePoint(p *point) {
	C.EC_POINT_free(p.p)
	p.p = nil
}

func (p *point) String() string {
	buf, _ := p.MarshalBinary()
	return hex.EncodeToString(buf)
}
func (p *point) Valid() bool {
	return C.EC_POINT_is_on_curve(p.g, p.p, p.c.ctx) != 0
}
func (p *point) Equal(p2 abstract.Point) bool {
	return C.EC_POINT_cmp(p.g, p.p, p2.(*point).p, p.c.ctx) == 0
}
func (p *point) GetX() *bignum {
	x := newBigNum()
	if C.EC_POINT_get_affine_coordinates_GFp(p.c.g, p.p, x.bn, nil,
		p.c.ctx) == 0 {
		panic("EC_POINT_get_affine_coordinates_GFp: " + getErrString())
	}
	return x
}
func (p *point) GetY() *bignum {
	y := newBigNum()
	if C.EC_POINT_get_affine_coordinates_GFp(p.c.g, p.p, nil, y.bn,
		p.c.ctx) == 0 {
		panic("EC_POINT_get_affine_coordinates_GFp: " + getErrString())
	}
	return y
}

func (p *point) Null() abstract.Point {
	if C.EC_POINT_set_to_infinity(p.c.g, p.p) == 0 {
		panic("EC_POINT_set_to_infinity: " + getErrString())
	}
	return p
}

func (p *point) Base() abstract.Point {
	genp := C.EC_GROUP_get0_generator(p.c.g)
	if genp == nil {
		panic("EC_GROUP_get0_generator: " + getErrString())
	}
	if C.EC_POINT_copy(p.p, genp) == 0 {
		panic("EC_POINT_copy: " + getErrString())
	}
	return p
}

func (p *point) PickLen() int {
	// Reserve at least 8 most-significant bits for randomness,
	// and the least-significant 8 bits for embedded data length.
	// (Hopefully it's unlikely we'll need >=2048-bit curves soon.)
	return (p.c.p.BitLen() - 8 - 8) / 8
}

func (p *point) Pick(data []byte, rand cipher.Stream) (abstract.Point, []byte) {

	l := p.c.PointLen()
	dl := p.PickLen()
	if dl > len(data) {
		dl = len(data)
	}

	b := make([]byte, l)
	for {
		// Pick a random compressed point, and overlay the data.
		// Decoding will fail if the point is not on the curve.
		rand.XORKeyStream(b, b)
		b[0] = (b[0] & 1) | 2 // COMPRESSED, random y bit

		if data != nil {
			b[l-1] = byte(dl)         // Encode length in low 8 bits
			copy(b[l-dl-1:l-1], data) // Copy in data to embed
		}

		if err := p.UnmarshalBinary(b); err == nil { // See if it decodes!
			return p, data[dl:]
		}

		// otherwise try again...
	}
}

func (p *point) Data() ([]byte, error) {
	l := p.c.plen          // encoded byte length of coordinate
	b := p.GetX().Bytes(l) // we only need the X-coordindate
	if len(b) != l {
		panic("encoded coordinate wrong length")
	}
	dl := int(b[l-1])
	if dl > p.PickLen() {
		return nil, errors.New("invalid embedded data length")
	}
	return b[l-dl-1 : l-1], nil
}

func (p *point) Add(ca, cb abstract.Point) abstract.Point {
	a := ca.(*point)
	b := cb.(*point)
	if C.EC_POINT_add(p.c.g, p.p, a.p, b.p, p.c.ctx) == 0 {
		panic("EC_POINT_add: " + getErrString())
	}
	return p
}

func (p *point) Sub(ca, cb abstract.Point) abstract.Point {
	a := ca.(*point)
	b := cb.(*point)
	// Add the point inverse.  Must use temporary if p == a.
	t := p
	if p == a {
		t = newPoint(p.c)
	}
	if C.EC_POINT_copy(t.p, b.p) == 0 {
		panic("EC_POINT_copy: " + getErrString())
	}
	if C.EC_POINT_invert(p.c.g, t.p, p.c.ctx) == 0 {
		panic("EC_POINT_invert: " + getErrString())
	}
	if C.EC_POINT_add(p.c.g, p.p, a.p, t.p, p.c.ctx) == 0 {
		panic("EC_POINT_add: " + getErrString())
	}
	return p
}

func (p *point) Neg(ca abstract.Point) abstract.Point {
	if ca != p {
		a := ca.(*point)
		if C.EC_POINT_copy(p.p, a.p) == 0 {
			panic("EC_POINT_copy: " + getErrString())
		}
	}
	if C.EC_POINT_invert(p.c.g, p.p, p.c.ctx) == 0 {
		panic("EC_POINT_invert: " + getErrString())
	}
	return p
}

func (p *point) Mul(cb abstract.Point, cs abstract.Scalar) abstract.Point {
	s := cs.(*scalar)
	if cb == nil { // multiply standard generator
		if C.EC_POINT_mul(p.c.g, p.p, s.bignum.bn, nil, nil,
			p.c.ctx) == 0 {
			panic("EC_POINT_mul: " + getErrString())
		}
	} else { // multiply arbitrary point b
		b := cb.(*point)
		if C.EC_POINT_mul(p.c.g, p.p, nil, b.p, s.bignum.bn,
			p.c.ctx) == 0 {
			panic("EC_POINT_mul: " + getErrString())
		}
	}
	return p
}

func (p *point) MarshalSize() int {
	return 1 + p.c.plen // compressed encoding
}

func (p *point) MarshalBinary() ([]byte, error) {
	l := 1 + p.c.plen
	b := make([]byte, l)

	// Note: EC_POINT_point2oct encodes the "point at infinity"
	// as a single 0 byte, hence returning a length of 1.
	if C.EC_POINT_point2oct(p.c.g, p.p, C.POINT_CONVERSION_COMPRESSED,
		(*_Ctype_unsignedchar)(unsafe.Pointer(&b[0])),
		C.size_t(l), p.c.ctx) == 0 {
		panic("EC_POINT_point2oct: " + getErrString())
	}

	return b, nil
}

func (p *point) UnmarshalBinary(buf []byte) error {
	l := len(buf)
	if buf[0] == 0 { // Special case: point at infinity
		l = 1 // single 0 byte
	}

	if C.EC_POINT_oct2point(p.g, p.p,
		(*_Ctype_unsignedchar)(unsafe.Pointer(&buf[0])),
		C.size_t(l), p.c.ctx) == 0 {
		return errors.New(getErrString())
	}
	return nil
}

func (p *point) MarshalTo(w io.Writer) (int, error) {
	return group.PointMarshalTo(p, w)
}

func (p *point) UnmarshalFrom(r io.Reader) (int, error) {
	return group.PointUnmarshalFrom(p, r)
}

func (c *curve) String() string {
	return c.name
}

func (c *curve) PrimeOrder() bool {
	return true // we only support the NIST prime-order curves
}

func (c *curve) ScalarLen() int {
	return c.nlen
}

func (c *curve) Scalar() abstract.Scalar {
	s := newScalar(c)
	s.c = c
	return s
}

func (c *curve) PointLen() int {
	return 1 + c.plen // compressed encoding
}

func (c *curve) Point() abstract.Point {
	return newPoint(c)
}

func (c *curve) Order() *big.Int {
	return c.n.BigInt()
}

func (c *curve) initNamedCurve(name string, nid C.int) *curve {
	c.name = name

	c.ctx = C.BN_CTX_new()
	if c.ctx == nil {
		panic("C.BN_CTX_new: " + getErrString())
	}

	c.g = C.EC_GROUP_new_by_curve_name(nid)
	if c.g == nil {
		panic("can't find create P256 curve: " + getErrString())
	}

	// Get this curve's prime field
	c.p = newBigNum()
	if C.EC_GROUP_get_curve_GFp(c.g, c.p.bn, nil, nil, c.ctx) == 0 {
		panic("EC_GROUP_get_curve_GFp: " + getErrString())
	}
	c.plen = (c.p.BitLen() + 7) / 8

	// Get the curve's group order
	c.n = newBigNum()
	if C.EC_GROUP_get_order(c.g, c.n.bn, c.ctx) == 0 {
		panic("EC_GROUP_get_order: " + getErrString())
	}
	c.nlen = (c.n.BitLen() + 7) / 8

	// Get the curve's cofactor
	c.cofact = newBigNum()
	if C.EC_GROUP_get_cofactor(c.g, c.cofact.bn, c.ctx) == 0 {
		panic("EC_GROUP_get_cofactor: " + getErrString())
	}

	// Stash a copy of the point at infinity
	c.null = newPoint(c)
	c.null.Null()

	return c
}

func (c *curve) InitP224() abstract.Group {
	return c.initNamedCurve("P224", C.NID_secp224r1)
}

func (c *curve) InitP256() abstract.Group {
	return c.initNamedCurve("P256", C.NID_X9_62_prime256v1)
}

func (c *curve) InitP384() abstract.Group {
	return c.initNamedCurve("P384", C.NID_secp384r1)
}

func (c *curve) InitP521() abstract.Group {
	return c.initNamedCurve("P521", C.NID_secp521r1)
}
