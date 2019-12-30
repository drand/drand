package bls

import (
	"fmt"
	"io"
	"math/big"
)

var fp6One = fe6{fp2One, fp2Zero, fp2Zero}
var fp6Zero = fe6{fp2Zero, fp2Zero, fp2Zero}

func (fp *fp6) newElement() *fe6 {
	return &fe6{}
}

func (fp *fp6) newElementFromBytes(c *fe6, b []byte) error {
	if len(b) < 288 {
		return fmt.Errorf("input string should be larger than 288 bytes")
	}
	if err := fp.f.newElementFromBytes(&c[2], b[:96]); err != nil {
		return err
	}
	if err := fp.f.newElementFromBytes(&c[1], b[96:192]); err != nil {
		return err
	}
	if err := fp.f.newElementFromBytes(&c[0], b[192:]); err != nil {
		return err
	}
	return nil
}

func (fp *fp6) randElement(a *fe6, r io.Reader) (*fe6, error) {
	if _, err := fp.f.randElement(&a[0], r); err != nil {
		return nil, err
	}
	if _, err := fp.f.randElement(&a[1], r); err != nil {
		return nil, err
	}
	if _, err := fp.f.randElement(&a[2], r); err != nil {
		return nil, err
	}
	return a, nil
}

func (fp *fp6) zero() *fe6 {
	return &fe6{}
}

func (fp *fp6) one() *fe6 {
	return &fe6{*fp.f.one()}
}

func (fp *fp6) toBytes(a *fe6) []byte {
	out := make([]byte, 288)
	copy(out[:96], fp.f.toBytes(&a[2]))
	copy(out[96:192], fp.f.toBytes(&a[1]))
	copy(out[192:], fp.f.toBytes(&a[0]))
	return out
}

func (fp *fp6) isZero(a *fe6) bool {
	return fp.f.isZero(&a[0]) && fp.f.isZero(&a[1]) && fp.f.isZero(&a[2])
}

func (fp *fp6) equal(a, b *fe6) bool {
	return fp.f.equal(&a[0], &b[0]) && fp.f.equal(&a[1], &b[1]) && fp.f.equal(&a[2], &b[2])
}

func (fp *fp6) copy(c, a *fe6) {
	fp.f.copy(&c[0], &a[0])
	fp.f.copy(&c[1], &a[1])
	fp.f.copy(&c[2], &a[2])
}

func (fp *fp6) mulByNonResidue(c, a *fe6) {
	t := fp.t
	fp.f.copy(t[0], &a[0])
	fp.f.mulByNonResidue(&c[0], &a[2])
	fp.f.copy(&c[2], &a[1])
	fp.f.copy(&c[1], t[0])
}

func (fp *fp6) add(c, a, b *fe6) {
	fp.f.add(&c[0], &a[0], &b[0])
	fp.f.add(&c[1], &a[1], &b[1])
	fp.f.add(&c[2], &a[2], &b[2])
}

func (fp *fp6) addAssign(a, b *fe6) {
	fp.f.addAssign(&a[0], &b[0])
	fp.f.addAssign(&a[1], &b[1])
	fp.f.addAssign(&a[2], &b[2])
}

func (fp *fp6) double(c, a *fe6) {
	fp.f.double(&c[0], &a[0])
	fp.f.double(&c[1], &a[1])
	fp.f.double(&c[2], &a[2])
}

func (fp *fp6) doubleAssign(a *fe6) {
	fp.f.doubleAssign(&a[0])
	fp.f.doubleAssign(&a[1])
	fp.f.doubleAssign(&a[2])
}

func (fp *fp6) sub(c, a, b *fe6) {
	fp.f.sub(&c[0], &a[0], &b[0])
	fp.f.sub(&c[1], &a[1], &b[1])
	fp.f.sub(&c[2], &a[2], &b[2])
}

func (fp *fp6) subAssign(a, b *fe6) {
	fp.f.subAssign(&a[0], &b[0])
	fp.f.subAssign(&a[1], &b[1])
	fp.f.subAssign(&a[2], &b[2])
}

func (fp *fp6) neg(c, a *fe6) {
	fp.f.neg(&c[0], &a[0])
	fp.f.neg(&c[1], &a[1])
	fp.f.neg(&c[2], &a[2])
}

func (fq *fp6) conjugate(c, a *fe6) {
	fq.f.copy(&c[0], &a[0])
	fq.f.neg(&c[1], &a[1])
	fq.f.copy(&c[2], &a[2])
}

func (fp *fp6) mulByBaseField(c, a *fe6, b *fe2) {
	fp.f.mul(&c[0], &a[0], b)
	fp.f.mul(&c[1], &a[1], b)
	fp.f.mul(&c[2], &a[2], b)
}

func (fp *fp6) frobeniusMap(c, a *fe6, power uint) {
	fp.f.frobeniousMap(&c[0], &a[0], power)
	fp.f.frobeniousMap(&c[1], &a[1], power)
	fp.f.frobeniousMap(&c[2], &a[2], power)
	switch power % 6 {
	case 0:
		return
	case 3:
		fp.f.f.neg(&c[0][0], &a[1][1])
		fp.f.f.copy(&c[1][1], &a[1][0])
		fp.f.neg(&a[2], &a[2])
	default:
		fp.f.mul(&c[1], &c[1], &frobeniusCoeffs61[power%6])
		fp.f.mul(&c[2], &c[2], &frobeniusCoeffs62[power%6])
	}
}

func (fp *fp6) frobeniusMapAssign(a *fe6, power uint) {
	fp.f.frobeniousMapAssign(&a[0], power)
	fp.f.frobeniousMapAssign(&a[1], power)
	fp.f.frobeniousMapAssign(&a[2], power)
	t := fp.t
	switch power % 6 {
	case 0:
		return
	case 3:
		fp.f.f.neg(&t[0][0], &a[1][1])
		fp.f.f.copy(&a[1][1], &a[1][0])
		fp.f.f.copy(&a[1][0], &t[0][0])
		fp.f.neg(&a[2], &a[2])
	default:
		fp.f.mulAssign(&a[1], &frobeniusCoeffs61[power%6])
		fp.f.mulAssign(&a[2], &frobeniusCoeffs62[power%6])
	}
}

func (fp *fp6) exp(c, a *fe6, e *big.Int) {
	z := fp.one()
	for i := e.BitLen() - 1; i >= 0; i-- {
		fp.square(z, z)
		if e.Bit(i) == 1 {
			fp.mul(z, z, a)
		}
	}
	fp.copy(c, z)
}

func (fp *fp6) mulBy1(c, a *fe6, b1 *fe2) {
	t := fp.t
	fp2 := fp.f
	fp2.mul(t[0], &a[2], b1)
	fp2.mul(&c[2], &a[1], b1)
	fp2.mul(&c[1], &a[0], b1)
	fp2.mulByNonResidue(&c[0], t[0])
}

func (fp *fp6) inverse(c, a *fe6) {
	t := fp.t
	fp.f.square(t[0], &a[0])
	fp.f.mul(t[1], &a[1], &a[2])
	fp.f.mulByNonResidue(t[1], t[1])
	fp.f.subAssign(t[0], t[1])
	fp.f.square(t[1], &a[1])
	fp.f.mul(t[2], &a[0], &a[2])
	fp.f.subAssign(t[1], t[2])
	fp.f.square(t[2], &a[2])
	fp.f.mulByNonResidue(t[2], t[2])
	fp.f.mul(t[3], &a[0], &a[1])
	fp.f.subAssign(t[2], t[3])
	fp.f.mul(t[3], &a[2], t[2])
	fp.f.mul(t[4], &a[1], t[1])
	fp.f.addAssign(t[3], t[4])
	fp.f.mulByNonResidue(t[3], t[3])
	fp.f.mul(t[4], &a[0], t[0])
	fp.f.addAssign(t[3], t[4])
	fp.f.inverse(t[3], t[3])
	fp.f.mul(&c[0], t[0], t[3])
	fp.f.mul(&c[1], t[2], t[3])
	fp.f.mul(&c[2], t[1], t[3])
}
