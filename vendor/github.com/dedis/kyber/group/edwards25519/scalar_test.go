package edwards25519

import (
	"testing"

	"github.com/dedis/kyber/util/random"

	kyber "github.com/dedis/kyber"
)

// SimpleCTScalar implements the scalar operations only using `ScMulAdd` by
// plaiying with the parameters.
type SimpleCTScalar struct {
	*scalar
}

func newSimpleCTScalar() kyber.Scalar {
	return &SimpleCTScalar{&scalar{}}
}

var one = new(scalar).SetInt64(1).(*scalar)
var zero = new(scalar).Zero().(*scalar)

var minusOne = new(scalar).SetBytes([]byte{0xec, 0xd3, 0xf5, 0x5c, 0x1a, 0x63, 0x12, 0x58, 0xd6, 0x9c, 0xf7, 0xa2, 0xde, 0xf9, 0xde, 0x14, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10}).(*scalar)

func (s *SimpleCTScalar) Add(s1, s2 kyber.Scalar) kyber.Scalar {
	sc1 := s1.(*SimpleCTScalar)
	sc2 := s2.(*SimpleCTScalar)

	// a * b + c = a * 1 + c
	scMulAdd(&s.v, &sc1.v, &one.v, &sc2.v)
	return s
}

func (s *SimpleCTScalar) Mul(s1, s2 kyber.Scalar) kyber.Scalar {
	sc1 := s1.(*SimpleCTScalar)
	sc2 := s2.(*SimpleCTScalar)

	// a * b + c = a * b + 0
	scMulAdd(&s.v, &sc1.v, &sc2.v, &zero.v)
	return s
}

func (s *SimpleCTScalar) Sub(s1, s2 kyber.Scalar) kyber.Scalar {
	sc1 := s1.(*SimpleCTScalar)
	sc2 := s2.(*SimpleCTScalar)

	// a * b + c = -1 * a + c
	scMulAdd(&s.v, &minusOne.v, &sc1.v, &sc2.v)
	return s

}

func (s *SimpleCTScalar) Equal(s2 kyber.Scalar) bool {
	return s.scalar.Equal(s2.(*SimpleCTScalar).scalar)
}

// factoredScalar implements the scalar operations using a factored version or
// `ScReduce` at the end of each operations.
type factoredScalar struct {
	*scalar
}

func newFactoredScalar() kyber.Scalar {
	return &factoredScalar{&scalar{}}
}

func (s *factoredScalar) Add(s1, s2 kyber.Scalar) kyber.Scalar {
	sf1 := s1.(*factoredScalar)
	sf2 := s2.(*factoredScalar)
	scAddFact(&s.v, &sf1.v, &sf2.v)
	return s
}

func (s *factoredScalar) Mul(s1, s2 kyber.Scalar) kyber.Scalar {
	sf1 := s1.(*factoredScalar)
	sf2 := s2.(*factoredScalar)
	scMulFact(&s.v, &sf1.v, &sf2.v)
	return s
}

func (s *factoredScalar) Sub(s1, s2 kyber.Scalar) kyber.Scalar {
	sf1 := s1.(*factoredScalar)
	sf2 := s2.(*factoredScalar)
	scSubFact(&s.v, &sf1.v, &sf2.v)
	return s
}

func (s *factoredScalar) Equal(s2 kyber.Scalar) bool {
	return s.scalar.Equal(s2.(*factoredScalar).scalar)
}

func TestFactoredScalar(t *testing.T) {
	testSimple(t, newFactoredScalar)
}

func TestSimpleCTScalar(t *testing.T) {
	testSimple(t, newSimpleCTScalar)
}

func testSimple(t *testing.T, new func() kyber.Scalar) {
	s1 := new()
	s2 := new()
	s3 := new()
	s1.SetInt64(2)
	s2.Pick(random.Stream)

	s22 := new().Add(s2, s2)

	if !s3.Mul(s1, s2).Equal(s22) {
		t.Fail()
	}

}

func benchScalarAdd(b *testing.B, new func() kyber.Scalar) {
	var seed = testSuite.Cipher([]byte("hello world"))
	s1 := new()
	s2 := new()
	s3 := new()
	s1.Pick(seed)
	s2.Pick(seed)

	for i := 0; i < b.N; i++ {
		s3.Add(s1, s2)
	}
}

func benchScalarMul(b *testing.B, new func() kyber.Scalar) {
	var seed = testSuite.Cipher([]byte("hello world"))
	s1 := new()
	s2 := new()
	s3 := new()
	s1.Pick(seed)
	s2.Pick(seed)

	for i := 0; i < b.N; i++ {
		s3.Mul(s1, s2)
	}
}

func benchScalarSub(b *testing.B, new func() kyber.Scalar) {
	var seed = testSuite.Cipher([]byte("hello world"))
	s1 := new()
	s2 := new()
	s3 := new()
	s1.Pick(seed)
	s2.Pick(seed)

	for i := 0; i < b.N; i++ {
		s3.Sub(s1, s2)
	}
}

// addition

func BenchmarkCTScalarAdd(b *testing.B) { benchScalarAdd(b, testSuite.Scalar) }

func BenchmarkCTScalarSimpleAdd(b *testing.B) { benchScalarAdd(b, newSimpleCTScalar) }

func BenchmarkCTScalarFactoredAdd(b *testing.B) { benchScalarAdd(b, newFactoredScalar) }

// multiplication

func BenchmarkCTScalarMul(b *testing.B) { benchScalarMul(b, testSuite.Scalar) }

func BenchmarkCTScalarSimpleMul(b *testing.B) { benchScalarMul(b, newSimpleCTScalar) }

func BenchmarkCTScalarFactoredMul(b *testing.B) { benchScalarMul(b, newFactoredScalar) }

// substraction

func BenchmarkCTScalarSub(b *testing.B) { benchScalarSub(b, testSuite.Scalar) }

func BenchmarkCTScalarSimpleSub(b *testing.B) { benchScalarSub(b, newSimpleCTScalar) }

func BenchmarkCTScalarFactoredSub(b *testing.B) { benchScalarSub(b, newFactoredScalar) }

func doCarryUncentered(limbs [24]int64, i int) {
	carry := limbs[i] >> 21
	limbs[i+1] += carry
	limbs[i] -= carry << 21
}

// Carry excess from the `i`-th limb into the `(i+1)`-th limb.
// Postcondition: `-2^20 <= limbs[i] < 2^20`.
func doCarryCentered(limbs [24]int64, i int) {
	carry := (limbs[i] + (1 << 20)) >> 21
	limbs[i+1] += carry
	limbs[i] -= carry << 21
}

func doReduction(limbs [24]int64, i int) {
	limbs[i-12] += limbs[i] * 666643
	limbs[i-11] += limbs[i] * 470296
	limbs[i-10] += limbs[i] * 654183
	limbs[i-9] -= limbs[i] * 997805
	limbs[i-8] += limbs[i] * 136657
	limbs[i-7] -= limbs[i] * 683901
	limbs[i] = 0
}

func scReduceLimbs(limbs [24]int64) {
	//for i in 0..23 {
	for i := 0; i < 23; i++ {
		doCarryCentered(limbs, i)
	}
	//for i in (0..23).filter(|x| x % 2 == 1) {
	for i := 1; i < 23; i += 2 {
		doCarryCentered(limbs, i)
	}

	doReduction(limbs, 23)
	doReduction(limbs, 22)
	doReduction(limbs, 21)
	doReduction(limbs, 20)
	doReduction(limbs, 19)
	doReduction(limbs, 18)

	//for i in (6..18).filter(|x| x % 2 == 0) {
	for i := 6; i < 18; i += 2 {
		doCarryCentered(limbs, i)
	}

	//  for i in (6..16).filter(|x| x % 2 == 1) {
	for i := 7; i < 16; i += 2 {
		doCarryCentered(limbs, i)
	}
	doReduction(limbs, 17)
	doReduction(limbs, 16)
	doReduction(limbs, 15)
	doReduction(limbs, 14)
	doReduction(limbs, 13)
	doReduction(limbs, 12)

	//for i in (0..12).filter(|x| x % 2 == 0) {
	for i := 0; i < 12; i += 2 {
		doCarryCentered(limbs, i)
	}
	//for i in (0..12).filter(|x| x % 2 == 1) {
	for i := 1; i < 12; i += 2 {
		doCarryCentered(limbs, i)
	}

	doReduction(limbs, 12)

	//for i in 0..12 {
	for i := 0; i < 12; i++ {
		doCarryUncentered(limbs, i)
	}

	doReduction(limbs, 12)

	//for i in 0..11 {
	for i := 0; i < 11; i++ {
		doCarryUncentered(limbs, i)
	}
}

func scAddFact(s, a, c *[32]byte) {
	a0 := 2097151 & load3(a[:])
	a1 := 2097151 & (load4(a[2:]) >> 5)
	a2 := 2097151 & (load3(a[5:]) >> 2)
	a3 := 2097151 & (load4(a[7:]) >> 7)
	a4 := 2097151 & (load4(a[10:]) >> 4)
	a5 := 2097151 & (load3(a[13:]) >> 1)
	a6 := 2097151 & (load4(a[15:]) >> 6)
	a7 := 2097151 & (load3(a[18:]) >> 3)
	a8 := 2097151 & load3(a[21:])
	a9 := 2097151 & (load4(a[23:]) >> 5)
	a10 := 2097151 & (load3(a[26:]) >> 2)
	a11 := (load4(a[28:]) >> 7)
	c0 := 2097151 & load3(c[:])
	c1 := 2097151 & (load4(c[2:]) >> 5)
	c2 := 2097151 & (load3(c[5:]) >> 2)
	c3 := 2097151 & (load4(c[7:]) >> 7)
	c4 := 2097151 & (load4(c[10:]) >> 4)
	c5 := 2097151 & (load3(c[13:]) >> 1)
	c6 := 2097151 & (load4(c[15:]) >> 6)
	c7 := 2097151 & (load3(c[18:]) >> 3)
	c8 := 2097151 & load3(c[21:])
	c9 := 2097151 & (load4(c[23:]) >> 5)
	c10 := 2097151 & (load3(c[26:]) >> 2)
	c11 := (load4(c[28:]) >> 7)

	var limbs [24]int64
	limbs[0] = c0 + a0
	limbs[1] = c1 + a1
	limbs[2] = c2 + a2
	limbs[3] = c3 + a3
	limbs[4] = c4 + a4
	limbs[5] = c5 + a5
	limbs[6] = c6 + a6
	limbs[7] = c7 + a7
	limbs[8] = c8 + a8
	limbs[9] = c9 + a9
	limbs[10] = c10 + a10
	limbs[11] = c11 + a11
	limbs[12] = int64(0)
	limbs[13] = int64(0)
	limbs[14] = int64(0)
	limbs[15] = int64(0)
	limbs[16] = int64(0)
	limbs[17] = int64(0)
	limbs[18] = int64(0)
	limbs[19] = int64(0)
	limbs[20] = int64(0)
	limbs[21] = int64(0)
	limbs[22] = int64(0)
	limbs[23] = int64(0)

	scReduceLimbs(limbs)
}

func scMulFact(s, a, b *[32]byte) {
	a0 := 2097151 & load3(a[:])
	a1 := 2097151 & (load4(a[2:]) >> 5)
	a2 := 2097151 & (load3(a[5:]) >> 2)
	a3 := 2097151 & (load4(a[7:]) >> 7)
	a4 := 2097151 & (load4(a[10:]) >> 4)
	a5 := 2097151 & (load3(a[13:]) >> 1)
	a6 := 2097151 & (load4(a[15:]) >> 6)
	a7 := 2097151 & (load3(a[18:]) >> 3)
	a8 := 2097151 & load3(a[21:])
	a9 := 2097151 & (load4(a[23:]) >> 5)
	a10 := 2097151 & (load3(a[26:]) >> 2)
	a11 := (load4(a[28:]) >> 7)
	b0 := 2097151 & load3(b[:])
	b1 := 2097151 & (load4(b[2:]) >> 5)
	b2 := 2097151 & (load3(b[5:]) >> 2)
	b3 := 2097151 & (load4(b[7:]) >> 7)
	b4 := 2097151 & (load4(b[10:]) >> 4)
	b5 := 2097151 & (load3(b[13:]) >> 1)
	b6 := 2097151 & (load4(b[15:]) >> 6)
	b7 := 2097151 & (load3(b[18:]) >> 3)
	b8 := 2097151 & load3(b[21:])
	b9 := 2097151 & (load4(b[23:]) >> 5)
	b10 := 2097151 & (load3(b[26:]) >> 2)
	b11 := (load4(b[28:]) >> 7)
	c0 := int64(0)
	c1 := int64(0)
	c2 := int64(0)
	c3 := int64(0)
	c4 := int64(0)
	c5 := int64(0)
	c6 := int64(0)
	c7 := int64(0)
	c8 := int64(0)
	c9 := int64(0)
	c10 := int64(0)
	c11 := int64(0)

	var limbs [24]int64
	limbs[0] = c0 + a0*b0
	limbs[1] = c1 + a0*b1 + a1*b0
	limbs[2] = c2 + a0*b2 + a1*b1 + a2*b0
	limbs[3] = c3 + a0*b3 + a1*b2 + a2*b1 + a3*b0
	limbs[4] = c4 + a0*b4 + a1*b3 + a2*b2 + a3*b1 + a4*b0
	limbs[5] = c5 + a0*b5 + a1*b4 + a2*b3 + a3*b2 + a4*b1 + a5*b0
	limbs[6] = c6 + a0*b6 + a1*b5 + a2*b4 + a3*b3 + a4*b2 + a5*b1 + a6*b0
	limbs[7] = c7 + a0*b7 + a1*b6 + a2*b5 + a3*b4 + a4*b3 + a5*b2 + a6*b1 + a7*b0
	limbs[8] = c8 + a0*b8 + a1*b7 + a2*b6 + a3*b5 + a4*b4 + a5*b3 + a6*b2 + a7*b1 + a8*b0
	limbs[9] = c9 + a0*b9 + a1*b8 + a2*b7 + a3*b6 + a4*b5 + a5*b4 + a6*b3 + a7*b2 + a8*b1 + a9*b0
	limbs[10] = c10 + a0*b10 + a1*b9 + a2*b8 + a3*b7 + a4*b6 + a5*b5 + a6*b4 + a7*b3 + a8*b2 + a9*b1 + a10*b0
	limbs[11] = c11 + a0*b11 + a1*b10 + a2*b9 + a3*b8 + a4*b7 + a5*b6 + a6*b5 + a7*b4 + a8*b3 + a9*b2 + a10*b1 + a11*b0
	limbs[12] = a1*b11 + a2*b10 + a3*b9 + a4*b8 + a5*b7 + a6*b6 + a7*b5 + a8*b4 + a9*b3 + a10*b2 + a11*b1
	limbs[13] = a2*b11 + a3*b10 + a4*b9 + a5*b8 + a6*b7 + a7*b6 + a8*b5 + a9*b4 + a10*b3 + a11*b2
	limbs[14] = a3*b11 + a4*b10 + a5*b9 + a6*b8 + a7*b7 + a8*b6 + a9*b5 + a10*b4 + a11*b3
	limbs[15] = a4*b11 + a5*b10 + a6*b9 + a7*b8 + a8*b7 + a9*b6 + a10*b5 + a11*b4
	limbs[16] = a5*b11 + a6*b10 + a7*b9 + a8*b8 + a9*b7 + a10*b6 + a11*b5
	limbs[17] = a6*b11 + a7*b10 + a8*b9 + a9*b8 + a10*b7 + a11*b6
	limbs[18] = a7*b11 + a8*b10 + a9*b9 + a10*b8 + a11*b7
	limbs[19] = a8*b11 + a9*b10 + a10*b9 + a11*b8
	limbs[20] = a9*b11 + a10*b10 + a11*b9
	limbs[21] = a10*b11 + a11*b10
	limbs[22] = a11 * b11
	limbs[23] = int64(0)

	scReduceLimbs(limbs)
}

func scSubFact(s, a, c *[32]byte) {
	a0 := 2097151 & load3(a[:])
	a1 := 2097151 & (load4(a[2:]) >> 5)
	a2 := 2097151 & (load3(a[5:]) >> 2)
	a3 := 2097151 & (load4(a[7:]) >> 7)
	a4 := 2097151 & (load4(a[10:]) >> 4)
	a5 := 2097151 & (load3(a[13:]) >> 1)
	a6 := 2097151 & (load4(a[15:]) >> 6)
	a7 := 2097151 & (load3(a[18:]) >> 3)
	a8 := 2097151 & load3(a[21:])
	a9 := 2097151 & (load4(a[23:]) >> 5)
	a10 := 2097151 & (load3(a[26:]) >> 2)
	a11 := (load4(a[28:]) >> 7)
	c0 := 2097151 & load3(c[:])
	c1 := 2097151 & (load4(c[2:]) >> 5)
	c2 := 2097151 & (load3(c[5:]) >> 2)
	c3 := 2097151 & (load4(c[7:]) >> 7)
	c4 := 2097151 & (load4(c[10:]) >> 4)
	c5 := 2097151 & (load3(c[13:]) >> 1)
	c6 := 2097151 & (load4(c[15:]) >> 6)
	c7 := 2097151 & (load3(c[18:]) >> 3)
	c8 := 2097151 & load3(c[21:])
	c9 := 2097151 & (load4(c[23:]) >> 5)
	c10 := 2097151 & (load3(c[26:]) >> 2)
	c11 := (load4(c[28:]) >> 7)

	var limbs [24]int64
	limbs[0] = 1916624 - c0 + a0
	limbs[1] = 863866 - c1 + a1
	limbs[2] = 18828 - c2 + a2
	limbs[3] = 1284811 - c3 + a3
	limbs[4] = 2007799 - c4 + a4
	limbs[5] = 456654 - c5 + a5
	limbs[6] = 5 - c6 + a6
	limbs[7] = 0 - c7 + a7
	limbs[8] = 0 - c8 + a8
	limbs[9] = 0 - c9 + a9
	limbs[10] = 0 - c10 + a10
	limbs[11] = 0 - c11 + a11
	limbs[12] = int64(16)
	limbs[13] = int64(0)
	limbs[14] = int64(0)
	limbs[15] = int64(0)
	limbs[16] = int64(0)
	limbs[17] = int64(0)
	limbs[18] = int64(0)
	limbs[19] = int64(0)
	limbs[20] = int64(0)
	limbs[21] = int64(0)
	limbs[22] = int64(0)
	limbs[23] = int64(0)

	scReduceLimbs(limbs)
}
