// +build experimental

package openssl

// #include <openssl/bn.h>
//
// // Macros don't work so well with cgo, so de-macroize them in C
// int bn_zero(BIGNUM *bn) { return BN_zero(bn); }
// int bn_one(BIGNUM *bn) { return BN_one(bn); }
//
import "C"

import (
	"crypto/cipher"
	"io"

	"github.com/dedis/kyber/abstract"
	"github.com/dedis/kyber/group"
)

type scalar struct {
	bignum
	c *curve
}

func newScalar(c *curve) *scalar {
	s := new(scalar)
	s.bignum.Init()
	s.c = c
	return s
}

func (s *scalar) String() string { return s.BigInt().String() }

func (s *scalar) Equal(s2 abstract.Scalar) bool {
	return s.Cmp(&s2.(*scalar).bignum) == 0
}

func (s *scalar) Set(x abstract.Scalar) abstract.Scalar {
	xs := x.(*scalar)
	if C.BN_copy(s.bignum.bn, xs.bignum.bn) == nil {
		panic("BN_copy: " + getErrString())
	}
	return s
}

func (s *scalar) Zero() abstract.Scalar {
	if C.bn_zero(s.bignum.bn) == 0 {
		panic("BN_zero: " + getErrString())
	}
	return s
}

func (s *scalar) One() abstract.Scalar {
	if C.bn_one(s.bignum.bn) == 0 {
		panic("BN_one: " + getErrString())
	}
	return s
}

func (s *scalar) SetInt64(v int64) abstract.Scalar {
	neg := false
	if v < 0 {
		neg = true
		v = -v
	}

	// Initialize the absolute value
	vl := C.BN_ULONG(v)
	if int64(v) != v {
		panic("openssl.SetInt64: value doesn't fit into C.ulong")
	}
	if C.BN_set_word(s.bignum.bn, vl) == 0 {
		panic("BN_set_word: " + getErrString())
	}

	// Negate if needed
	if neg {
		if C.BN_sub(s.bignum.bn, s.c.n.bn, s.bignum.bn) == 0 {
			panic("BN_sub: " + getErrString())
		}
	}

	return s
}

func (s *scalar) Add(x, y abstract.Scalar) abstract.Scalar {
	xs := x.(*scalar)
	ys := y.(*scalar)
	if C.BN_mod_add(s.bignum.bn, xs.bignum.bn, ys.bignum.bn, s.c.n.bn,
		s.c.ctx) == 0 {
		panic("BN_mod_add: " + getErrString())
	}
	return s
}

func (s *scalar) Sub(x, y abstract.Scalar) abstract.Scalar {
	xs := x.(*scalar)
	ys := y.(*scalar)
	if C.BN_mod_sub(s.bignum.bn, xs.bignum.bn, ys.bignum.bn, s.c.n.bn,
		s.c.ctx) == 0 {
		panic("BN_mod_sub: " + getErrString())
	}
	return s
}

func (s *scalar) Neg(x abstract.Scalar) abstract.Scalar {
	xs := x.(*scalar)
	if C.BN_mod_sub(s.bignum.bn, s.c.n.bn, xs.bignum.bn, s.c.n.bn,
		s.c.ctx) == 0 {
		panic("BN_mod_sub: " + getErrString())
	}
	return s
}

func (s *scalar) Mul(x, y abstract.Scalar) abstract.Scalar {
	xs := x.(*scalar)
	ys := y.(*scalar)
	if C.BN_mod_mul(s.bignum.bn, xs.bignum.bn, ys.bignum.bn, s.c.n.bn,
		s.c.ctx) == 0 {
		panic("BN_mod_mul: " + getErrString())
	}
	return s
}

func (s *scalar) Div(x, y abstract.Scalar) abstract.Scalar {
	xs := x.(*scalar)
	ys := y.(*scalar)

	// First compute inverse of y, then multiply by x.
	// Must use a temporary in the case x == s.
	t := &s.bignum
	if x == s {
		t = newBigNum()
	}
	if C.BN_mod_inverse(t.bn, ys.bignum.bn, s.c.n.bn,
		s.c.ctx) == nil {
		panic("BN_mod_inverse: " + getErrString())
	}
	if C.BN_mod_mul(s.bignum.bn, xs.bignum.bn, t.bn, s.c.n.bn,
		s.c.ctx) == 0 {
		panic("BN_mod_mul: " + getErrString())
	}
	return s
}

func (s *scalar) Inv(x abstract.Scalar) abstract.Scalar {
	xs := x.(*scalar)
	if C.BN_mod_inverse(s.bignum.bn, xs.bignum.bn, s.c.n.bn,
		s.c.ctx) == nil {
		panic("BN_mod_inverse: " + getErrString())
	}
	return s
}

func (s *scalar) Pick(rand cipher.Stream) abstract.Scalar {
	s.bignum.RandMod(s.c.n, rand)
	return s
}

func (s *scalar) MarshalSize() int {
	return s.c.nlen
}

func (s *scalar) MarshalBinary() ([]byte, error) {
	return s.Bytes(s.c.nlen), nil
}

func (s *scalar) UnmarshalBinary(buf []byte) error {
	s.SetBytes(buf)
	return nil
}

func (s *scalar) MarshalTo(w io.Writer) (int, error) {
	return group.ScalarMarshalTo(s, w)
}

func (s *scalar) UnmarshalFrom(r io.Reader) (int, error) {
	return group.ScalarUnmarshalFrom(s, r)
}
