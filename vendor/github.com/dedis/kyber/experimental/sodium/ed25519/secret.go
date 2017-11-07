// +build experimental

// +build sodium

package ed25519

// #include "sc.h"
//
import "C"

import (
	"bytes"
	"io"
	"unsafe"
	//"runtime"
	"crypto/cipher"
	"encoding/hex"

	"github.com/dedis/kyber/abstract"
	"github.com/dedis/kyber/group"
)

type secret struct {
	b [32]byte
}

var s0 = secret{}
var s1 = secret{[32]byte{1}}
var s2 = secret{[32]byte{2}}
var s3 = secret{[32]byte{3}}
var s4 = secret{[32]byte{4}}

func (s *secret) Set(s2 abstract.Scalar) abstract.Scalar {
	s.b = s2.(*secret).b
	return s
}

func (s *secret) String() string {
	return hex.EncodeToString(s.b[:])
}

func (s *secret) MarshalSize() int {
	return 32
}

func (s *secret) MarshalBinary() ([]byte, error) {
	return s.b[:], nil
}

func (s *secret) UnmarshalBinary(buf []byte) error {
	copy(s.b[:], buf)
	return nil
}

func (s *secret) MarshalTo(w io.Writer) (int, error) {
	return group.ScalarMarshalTo(s, w)
}

func (s *secret) UnmarshalFrom(r io.Reader) (int, error) {
	return group.ScalarUnmarshalFrom(s, r)
}

func (s *secret) Zero() abstract.Scalar {
	panic("XXX")
}

func (s *secret) One() abstract.Scalar {
	panic("XXX")
}

func (s *secret) SetInt64(v int64) abstract.Scalar {
	panic("XXX")
}

func (s *secret) Equal(s2 abstract.Scalar) bool {
	return bytes.Equal(s.b[:], s2.(*secret).b[:])
}

func (s *secret) Add(cx, cy abstract.Scalar) abstract.Scalar {
	x := cx.(*secret)
	y := cy.(*secret)

	// XXX using muladd is probably way overkill
	C.sc_muladd((*C.uchar)(unsafe.Pointer(&s.b[0])),
		(*C.uchar)(unsafe.Pointer(&x.b[0])),
		(*C.uchar)(unsafe.Pointer(&s1.b[0])),
		(*C.uchar)(unsafe.Pointer(&y.b[0])))

	return s
}

func (s *secret) Sub(cx, cy abstract.Scalar) abstract.Scalar {
	panic("XXX")
}

func (s *secret) Neg(x abstract.Scalar) abstract.Scalar {
	panic("XXX")
}

func (s *secret) Mul(cx, cy abstract.Scalar) abstract.Scalar {
	panic("XXX")
}

func (s *secret) Div(cx, cy abstract.Scalar) abstract.Scalar {
	panic("XXX")
}

func (s *secret) Inv(x abstract.Scalar) abstract.Scalar {
	panic("XXX")
}

func (s *secret) Pick(rand cipher.Stream) abstract.Scalar {
	rand.XORKeyStream(s.b[:], s.b[:])
	s.b[0] &= 248
	s.b[31] &= 63
	s.b[31] |= 64
	return s
}
