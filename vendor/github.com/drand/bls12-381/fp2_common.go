package bls

import (
	"fmt"
	"io"
	"math/big"
)

var fp2One = fe2{fpOne, fpZero}
var fp2Zero = fe2{fpZero, fpZero}

func (fp *fp2) newElement() *fe2 {
	return &fe2{}
}

func (fp *fp2) newElementFromBytes(fe *fe2, in []byte) error {
	if len(in) != 96 {
		return fmt.Errorf("input string should be larger than 96 bytes")
	}
	if err := fp.f.newElementFromBytes(&fe[1], in[:48]); err != nil {
		return err
	}
	if err := fp.f.newElementFromBytes(&fe[0], in[48:]); err != nil {
		return err
	}
	return nil
}

func (fp *fp2) randElement(a *fe2, r io.Reader) (*fe2, error) {
	if _, err := fp.f.randElement(&a[0], r); err != nil {
		return nil, err
	}
	if _, err := fp.f.randElement(&a[1], r); err != nil {
		return nil, err
	}
	return a, nil
}

func (fp *fp2) zero() *fe2 {
	return &fe2{}
}

func (fp *fp2) one() *fe2 {
	return &fe2{*fp.f.one(), *fp.f.zero()}
}

func (fp *fp2) toBytes(a *fe2) []byte {
	out := make([]byte, 96)
	copy(out[:48], fp.f.toBytes(&a[1]))
	copy(out[48:], fp.f.toBytes(&a[0]))
	return out
}

func (fp *fp2) isZero(a *fe2) bool {
	return fp.f.isZero(&a[0]) && fp.f.isZero(&a[1])
}

func (fp *fp2) equal(a, b *fe2) bool {
	return fp.f.equal(&a[0], &b[0]) && fp.f.equal(&a[1], &b[1])
}

func (fp *fp2) copy(c, a *fe2) {
	fp.f.copy(&c[0], &a[0])
	fp.f.copy(&c[1], &a[1])
}

func (fp *fp2) add(c, a, b *fe2) {
	fp.f.add(&c[0], &a[0], &b[0])
	fp.f.add(&c[1], &a[1], &b[1])
}

func (fp *fp2) addAssign(a, b *fe2) {
	fp.f.addAssign(&a[0], &b[0])
	fp.f.addAssign(&a[1], &b[1])
}

func (fp *fp2) ladd(c, a, b *fe2) {
	fp.f.ladd(&c[0], &a[0], &b[0])
	fp.f.ladd(&c[1], &a[1], &b[1])
}

func (fp *fp2) double(c, a *fe2) {
	fp.f.double(&c[0], &a[0])
	fp.f.double(&c[1], &a[1])
}

func (fp *fp2) doubleAssign(a *fe2) {
	fp.f.doubleAssign(&a[0])
	fp.f.doubleAssign(&a[1])
}

func (fp *fp2) ldouble(c, a *fe2) {
	fp.f.ldouble(&c[0], &a[0])
	fp.f.ldouble(&c[1], &a[1])
}

func (fp *fp2) sub(c, a, b *fe2) {
	fp.f.sub(&c[0], &a[0], &b[0])
	fp.f.sub(&c[1], &a[1], &b[1])
}

func (fp *fp2) subAssign(c, a *fe2) {
	fp.f.subAssign(&c[0], &a[0])
	fp.f.subAssign(&c[1], &a[1])
}

func (fp *fp2) lsub(c, a, b *fe2) {
	fp.f.lsub(&c[0], &a[0], &b[0])
	fp.f.lsub(&c[1], &a[1], &b[1])
}

func (fp *fp2) neg(c, a *fe2) {
	fp.f.neg(&c[0], &a[0])
	fp.f.neg(&c[1], &a[1])
}

func (fp *fp2) conjugate(c, a *fe2) {
	fp.f.copy(&c[0], &a[0])
	fp.f.neg(&c[1], &a[1])
}

func (fp *fp2) square(c, a *fe2) {
	t := fp.t
	fp.f.ladd(t[0], &a[0], &a[1])
	fp.f.sub(t[1], &a[0], &a[1])
	fp.f.ldouble(t[2], &a[0])
	fp.f.mul(&c[0], t[0], t[1])
	fp.f.mul(&c[1], t[2], &a[1])
}

func (fp *fp2) squareAssign(a *fe2) {
	t := fp.t
	fp.f.ladd(t[0], &a[0], &a[1])
	fp.f.sub(t[1], &a[0], &a[1])
	fp.f.ldouble(t[2], &a[0])
	fp.f.mul(&a[0], t[0], t[1])
	fp.f.mul(&a[1], t[2], &a[1])
}

func (fp *fp2) mulByNonResidue(c, a *fe2) {
	t := fp.t
	fp.f.sub(t[0], &a[0], &a[1])
	fp.f.add(&c[1], &a[0], &a[1])
	fp.f.copy(&c[0], t[0])
}

func (fp *fp2) mulByB(c, a *fe2) {
	t := fp.t
	fp.f.double(t[0], &a[0])
	fp.f.double(t[1], &a[1])
	fp.f.doubleAssign(t[0])
	fp.f.doubleAssign(t[1])
	fp.f.sub(&c[0], t[0], t[1])
	fp.f.add(&c[1], t[0], t[1])
}

func (fp *fp2) inverse(c, a *fe2) {
	t := fp.t
	fp.f.square(t[0], &a[0])
	fp.f.square(t[1], &a[1])
	fp.f.addAssign(t[0], t[1])
	fp.f.inverse(t[0], t[0])
	fp.f.mul(&c[0], &a[0], t[0])
	fp.f.mulAssign(t[0], &a[1])
	fp.f.neg(&c[1], t[0])
}

func (fp *fp2) mulByFq(c, a *fe2, b *fe) {
	fp.f.mul(&c[0], &a[0], b)
	fp.f.mul(&c[1], &a[1], b)
}

func (fp *fp2) exp(c, a *fe2, e *big.Int) {
	z := fp.one()
	for i := e.BitLen() - 1; i >= 0; i-- {
		fp.square(z, z)
		if e.Bit(i) == 1 {
			fp.mul(z, z, a)
		}
	}
	fp.copy(c, z)
}

func (fp *fp2) div(c, a, b *fe2) {
	t0 := fp.newElement()
	fp.inverse(t0, b)
	fp.mul(c, a, t0)
}

func (fp *fp2) frobeniousMap(c, a *fe2, power uint) {
	fp.f.copy(&c[0], &a[0])
	if power%2 == 1 {
		fp.f.neg(&c[1], &a[1])
		return
	}
	fp.f.copy(&c[1], &a[1])
}

func (fp *fp2) frobeniousMapAssign(a *fe2, power uint) {
	if power%2 == 1 {
		fp.f.neg(&a[1], &a[1])
		return
	}
}

func (fp *fp2) sqrt(c, a *fe2) bool {
	u, x0, a1, alpha := &fe2{}, &fe2{}, &fe2{}, &fe2{}
	fp.copy(u, a)
	fp.exp(a1, a, pMinus3Over4)
	fp.square(alpha, a1)
	fp.mul(alpha, alpha, a)
	fp.mul(x0, a1, a)
	if fp.equal(alpha, negativeOne2) {
		fp.f.neg(&c[0], &x0[1])
		fp.f.copy(&c[1], &x0[0])
		return true
	}
	fp.add(alpha, alpha, &fp2One)
	fp.exp(alpha, alpha, pMinus1Over2)
	fp.mul(c, alpha, x0)
	fp.square(alpha, c)
	return fp.equal(alpha, u)
}
