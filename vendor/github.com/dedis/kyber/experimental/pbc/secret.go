// +build experimental
// +build pbc

package pbc

// #include <stdlib.h>
// #include <pbc/pbc.h>
import "C"

import (
	"crypto/cipher"
	"errors"
	"io"
	"runtime"
	"unsafe"

	"github.com/dedis/kyber/abstract"
	"github.com/dedis/kyber/group"
)

type scalar struct {
	e C.element_t
}

func clearScalar(s *scalar) {
	println("clearScalar", s)
	C.element_clear(&s.e[0])
}

func newScalar() *scalar {
	s := new(scalar)
	runtime.SetFinalizer(s, clearScalar)
	return s
}

func (s *scalar) String() string {
	var b [256]byte
	l := C.element_snprint((*C.char)(unsafe.Pointer(&b[0])),
		C.size_t(len(b)), &s.e[0])
	if l <= 0 {
		panic("Can't convert pairing element to string")
	}
	return string(b[:l])
}

func (s *scalar) Equal(s2 abstract.Scalar) bool {
	return C.element_cmp(&s.e[0], &s2.(*scalar).e[0]) == 0
}

func (s *scalar) Set(x abstract.Scalar) abstract.Scalar {
	C.element_set(&s.e[0], &x.(*scalar).e[0])
	return s
}

func (s *scalar) Zero() abstract.Scalar {
	C.element_set0(&s.e[0])
	return s
}

func (s *scalar) One() abstract.Scalar {
	C.element_set0(&s.e[0])
	return s
}

func (s *scalar) SetInt64(v int64) abstract.Scalar {
	vl := C.long(v)
	if int64(vl) != v {
		panic("Oops, int64 initializer doesn't fit into C.ulong")
	}
	var z C.mpz_t
	C.mpz_init(&z[0])
	C.mpz_set_si(&z[0], vl)
	C.element_set_mpz(&s.e[0], &z[0])
	C.mpz_clear(&z[0])
	return s
}

func (s *scalar) Pick(rand cipher.Stream) abstract.Scalar {
	panic("XXX")
}

func (s *scalar) Add(a, b abstract.Scalar) abstract.Scalar {
	C.element_add(&s.e[0], &a.(*scalar).e[0], &b.(*scalar).e[0])
	return s
}

func (s *scalar) Sub(a, b abstract.Scalar) abstract.Scalar {
	C.element_sub(&s.e[0], &a.(*scalar).e[0], &b.(*scalar).e[0])
	return s
}

func (s *scalar) Neg(a abstract.Scalar) abstract.Scalar {
	C.element_neg(&s.e[0], &a.(*scalar).e[0])
	return s
}

func (s *scalar) Mul(a, b abstract.Scalar) abstract.Scalar {
	C.element_mul(&s.e[0], &a.(*scalar).e[0], &b.(*scalar).e[0])
	return s
}

func (s *scalar) Div(a, b abstract.Scalar) abstract.Scalar {
	C.element_div(&s.e[0], &a.(*scalar).e[0], &b.(*scalar).e[0])
	return s
}

func (s *scalar) Inv(a abstract.Scalar) abstract.Scalar {
	C.element_invert(&s.e[0], &a.(*scalar).e[0])
	return s
}

func (s *scalar) MarshalSize() int {
	return int(C.element_length_in_bytes(&s.e[0]))
}

func (s *scalar) MarshalBinary() ([]byte, error) {
	l := s.Len()
	b := make([]byte, l)
	a := C.element_to_bytes((*C.uchar)(unsafe.Pointer(&b[0])),
		&s.e[0])
	if int(a) != l {
		panic("Element encoding yielded wrong length")
	}
	return b, nil
}

func (s *scalar) UnmarshalBinary(buf []byte) error {
	l := s.Len()
	if len(buf) != l {
		return errors.New("Encoded element wrong length")
	}
	a := C.element_from_bytes(&s.e[0], (*C.uchar)(unsafe.Pointer(&buf[0])))
	if int(a) != l { // apparently doesn't return decoding errors
		panic("element_from_bytes consumed wrong number of bytes")
	}
	return nil
}

func (s *scalar) MarshalTo(w io.Writer) (int, error) {
	return group.ScalarMarshalTo(s, w)
}

func (s *scalar) UnmarshalFrom(r io.Reader) (int, error) {
	return group.ScalarUnmarshalFrom(s, r)
}
