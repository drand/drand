package pbc

import (
	"crypto/cipher"
	"io"
	"runtime"

	"github.com/dedis/kyber/util/random"

	"github.com/dfinity/go-dfinity-crypto/bls"

	"github.com/dedis/kyber"
)

type scalar struct {
	fe bls.Fr
}

// NewScalar returns a non initialized kyber.Scalar implementation to use with
// PBC groups.
func NewScalar() kyber.Scalar {
	s := &scalar{fe: bls.Fr{}}
	runtime.SetFinalizer(s, clearScalar)
	return s
}

func (s *scalar) Zero() kyber.Scalar {
	return s.SetInt64(0)
}

func (s *scalar) One() kyber.Scalar {
	return s.SetInt64(1)
}

func (s *scalar) Equal(s2 kyber.Scalar) bool {
	return s.fe.IsEqual(&s2.(*scalar).fe)
}

func (s *scalar) Neg(s2 kyber.Scalar) kyber.Scalar {
	bls.FrNeg(&s.fe, &s2.(*scalar).fe)
	return s
}

func (s *scalar) Add(s1, s2 kyber.Scalar) kyber.Scalar {
	sc1 := s1.(*scalar)
	sc2 := s2.(*scalar)
	bls.FrAdd(&s.fe, &sc1.fe, &sc2.fe)
	return s
}

func (s *scalar) Sub(s1, s2 kyber.Scalar) kyber.Scalar {
	sc1 := s1.(*scalar)
	sc2 := s2.(*scalar)
	bls.FrSub(&s.fe, &sc1.fe, &sc2.fe)
	return s
}

func (s *scalar) Mul(s1, s2 kyber.Scalar) kyber.Scalar {
	sc1 := s1.(*scalar)
	sc2 := s2.(*scalar)
	bls.FrMul(&s.fe, &sc1.fe, &sc2.fe)
	return s
}

func (s *scalar) Div(s1, s2 kyber.Scalar) kyber.Scalar {
	sc1 := s1.(*scalar)
	sc2 := s2.(*scalar)
	bls.FrDiv(&s.fe, &sc1.fe, &sc2.fe)
	return s
}

func (s *scalar) Inv(s2 kyber.Scalar) kyber.Scalar {
	sc2 := s2.(*scalar)
	bls.FrInv(&s.fe, &sc2.fe)
	return s
}

func (s *scalar) SetInt64(i int64) kyber.Scalar {
	s.fe.SetInt64(i)
	return s
}

func (s *scalar) Set(a kyber.Scalar) kyber.Scalar {
	buff, _ := a.MarshalBinary()
	err := s.UnmarshalBinary(buff)
	if err != nil {
		panic(err)
	}
	return s
}

func (s *scalar) Clone() kyber.Scalar {
	s2 := NewScalar()
	s2.Set(s)
	return s2
}

func (s *scalar) MarshalBinary() (buff []byte, err error) {
	defer func() {
		if e := recover(); e != nil {
			buff = nil
			err = e.(error)
		}
	}()

	buff = s.fe.Serialize()
	return
}

func (s *scalar) MarshalTo(w io.Writer) (int, error) {
	return ScalarMarshalTo(s, w)
}

func (s *scalar) UnmarshalBinary(buff []byte) error {
	return s.fe.Deserialize(buff)
}

func (s *scalar) UnmarshalFrom(r io.Reader) (int, error) {
	return ScalarUnmarshalFrom(s, r)
}

func (s *scalar) MarshalSize() int {
	// XXX hackish version. Better to know the usual length.
	buff, _ := s.MarshalBinary()
	return len(buff)
}

func (s *scalar) SetBytes(buff []byte) kyber.Scalar {
	// XXX Maybe later have a "real" setbytes
	s.UnmarshalBinary(buff)
	return s
}

func (s *scalar) Bytes() []byte {
	buff, _ := s.MarshalBinary()
	return buff
}

func (s *scalar) Pick(rand cipher.Stream) kyber.Scalar {
	buff := random.NonZeroBytes(s.MarshalSize(), rand)
	err := s.fe.SetLittleEndian(buff)
	if err != nil {
		panic(err)
	}
	return s
}

func (s *scalar) String() string {
	// return hexadecimal string
	return s.fe.GetString(16)
}

// clearScalar frees the memory allocated by the C library.
func clearScalar(s *scalar) {
	s.fe.Clear()
}

func (s *scalar) SetVarTime(varTime bool) error {
	return ErrVarTime
}
