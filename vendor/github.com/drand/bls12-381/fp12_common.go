package bls

import (
	"fmt"
	"io"
	"math/big"
)

var fp12One = fe12{fp6One, fp6Zero}
var fp12Zero = fe12{fp6Zero, fp6Zero}

func (fp *fp12) newElement() *fe12 {
	return &fe12{}
}

func (fp *fp12) newElementFromBytes(f *fe12, b []byte) error {
	if len(b) < 576 {
		return fmt.Errorf("input string should be larger than 576 bytes")
	}
	if err := fp.f.newElementFromBytes(&f[1], b[:288]); err != nil {
		return err
	}
	if err := fp.f.newElementFromBytes(&f[0], b[288:]); err != nil {
		return err
	}
	return nil
}

func (fp *fp12) randElement(a *fe12, r io.Reader) (*fe12, error) {
	if _, err := fp.f.randElement(&a[0], r); err != nil {
		return nil, err
	}
	if _, err := fp.f.randElement(&a[1], r); err != nil {
		return nil, err
	}
	return a, nil
}

func (fp *fp12) zero() *fe12 {
	return &fe12{}
}

func (fp *fp12) one() *fe12 {
	return &fe12{*fp.f.one()}
}

func (fp *fp12) toBytes(a *fe12) []byte {
	out := make([]byte, 576)
	copy(out[:288], fp.f.toBytes(&a[1]))
	copy(out[288:], fp.f.toBytes(&a[0]))
	return out
}

func (fp *fp12) isZero(a *fe12) bool {
	return fp.f.isZero(&a[0]) && fp.f.isZero(&a[1])
}

func (fp *fp12) equal(a, b *fe12) bool {
	return fp.f.equal(&a[0], &b[0]) && fp.f.equal(&a[1], &b[1])
}

func (fp *fp12) copy(c, a *fe12) {
	fp.f.copy(&c[0], &a[0])
	fp.f.copy(&c[1], &a[1])
}

func (fp *fp12) add(c, a, b *fe12) {
	fp.f.add(&c[0], &a[0], &b[0])
	fp.f.add(&c[1], &a[1], &b[1])

}

func (fp *fp12) double(c, a *fe12) {
	fp.f.double(&c[0], &a[0])
	fp.f.double(&c[1], &a[1])

}

func (fp *fp12) sub(c, a, b *fe12) {
	fp.f.sub(&c[0], &a[0], &b[0])
	fp.f.sub(&c[1], &a[1], &b[1])

}

func (fp *fp12) neg(c, a *fe12) {
	fp.f.neg(&c[0], &a[0])
	fp.f.neg(&c[1], &a[1])
}

func (fp *fp12) conjugate(c, a *fe12) {
	fp.f.copy(&c[0], &a[0])
	fp.f.neg(&c[1], &a[1])
}

func (fp *fp12) square(c, a *fe12) {
	t := fp.t
	fp.f.add(t[0], &a[0], &a[1])
	fp.f.mul(t[2], &a[0], &a[1])
	fp.f.mulByNonResidue(t[1], &a[1])
	fp.f.addAssign(t[1], &a[0])
	fp.f.mulByNonResidue(t[3], t[2])
	fp.f.mulAssign(t[0], t[1])
	fp.f.subAssign(t[0], t[2])
	fp.f.sub(&c[0], t[0], t[3])
	fp.f.double(&c[1], t[2])
}

func (fp *fp12) cyclotomicSquare(c, a *fe12) {
	t := fp.t2
	fp2 := fp.f.f
	fp.fp4Square(t[3], t[4], &a[0][0], &a[1][1])
	fp2.sub(t[2], t[3], &a[0][0])
	fp2.doubleAssign(t[2])
	fp2.add(&c[0][0], t[2], t[3])
	fp2.add(t[2], t[4], &a[1][1])
	fp2.doubleAssign(t[2])
	fp2.add(&c[1][1], t[2], t[4])
	fp.fp4Square(t[3], t[4], &a[1][0], &a[0][2])
	fp.fp4Square(t[5], t[6], &a[0][1], &a[1][2])
	fp2.sub(t[2], t[3], &a[0][1])
	fp2.doubleAssign(t[2])
	fp2.add(&c[0][1], t[2], t[3])
	fp2.add(t[2], t[4], &a[1][2])
	fp2.doubleAssign(t[2])
	fp2.add(&c[1][2], t[2], t[4])
	fp2.mulByNonResidue(t[3], t[6])
	fp2.add(t[2], t[3], &a[1][0])
	fp2.doubleAssign(t[2])
	fp2.add(&c[1][0], t[2], t[3])
	fp2.sub(t[2], t[5], &a[0][2])
	fp2.doubleAssign(t[2])
	fp2.add(&c[0][2], t[2], t[5])
}

func (fp *fp12) inverse(c, a *fe12) {
	t := fp.t
	fp.f.square(t[0], &a[0])
	fp.f.square(t[1], &a[1])
	fp.f.mulByNonResidue(t[1], t[1])
	fp.f.sub(t[1], t[0], t[1])
	fp.f.inverse(t[0], t[1])
	fp.f.mul(&c[0], &a[0], t[0])
	fp.f.mulAssign(t[0], &a[1])
	fp.f.neg(&c[1], t[0])
}

func (fp *fp12) exp(c, a *fe12, e *big.Int) {
	z := fp.one()
	for i := e.BitLen() - 1; i >= 0; i-- {
		fp.square(z, z)
		if e.Bit(i) == 1 {
			fp.mul(z, z, a)
		}
	}
	fp.copy(c, z)
}

func (fp *fp12) cyclotomicExp(c, a *fe12, e *big.Int) {
	z := fp.t12
	fp.copy(z, &fp12One)
	for i := e.BitLen() - 1; i >= 0; i-- {
		fp.cyclotomicSquare(z, z)
		if e.Bit(i) == 1 {
			fp.mul(z, z, a)
		}
	}
	fp.copy(c, z)
}

func (fp *fp12) mulBy014Assign(a *fe12, c0, c1, c4 *fe2) {
	t := fp.t
	o := fp.t2[0]
	fp.f.mulBy01(t[0], &a[0], c0, c1)
	fp.f.mulBy1(t[1], &a[1], c4)
	fp.f.f.add(o, c1, c4)
	fp.f.add(t[2], &a[1], &a[0])
	fp.f.mulBy01Assign(t[2], c0, o)
	fp.f.subAssign(t[2], t[0])
	fp.f.sub(&a[1], t[2], t[1])
	fp.f.mulByNonResidue(t[1], t[1])
	fp.f.add(&a[0], t[1], t[0])
}

func (fp *fp12) frobeniusMap(c, a *fe12, power uint) {
	fp.f.frobeniusMap(&c[0], &a[0], power)
	fp.f.frobeniusMap(&c[1], &a[1], power)
	switch power {
	case 0:
		return
	case 6:
		fp.f.neg(&c[1], &c[1])
	default:
		fp.f.mulByBaseField(&c[1], &c[1], &frobeniusCoeffs12[power])
	}
}

func (fp *fp12) frobeniusMapAssign(a *fe12, power uint) {

	fp.f.frobeniusMapAssign(&a[0], power)
	fp.f.frobeniusMapAssign(&a[1], power)
	switch power {
	case 0:
		return
	case 6:
		fp.f.neg(&a[1], &a[1])
	default:
		fp.f.mulByBaseField(&a[1], &a[1], &frobeniusCoeffs12[power])
	}
}
