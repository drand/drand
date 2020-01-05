// +build !nolazy

package bls

import (
	"golang.org/x/sys/cpu"
)

type fp struct {
	mul       func(c, a, b *fe)
	mulAssign func(a, b *fe)
	lmul      func(c *lfe, a, b *fe)
	reduce    func(c *fe, a *lfe)
}

func newFp() *fp {
	if cpu.X86.HasBMI2 && !enforceNonBMI2 {
		return &fp{
			mul:       montmul_bmi2,
			mulAssign: montmul_assign_bmi2,
			lmul:      mul_bmi2,
			reduce:    mont_bmi2,
		}
	}
	return &fp{
		mul:       montmul_nobmi2,
		mulAssign: montmul_assign_nobmi2,
		lmul:      mul_nobmi2,
		reduce:    mont_nobmi2,
	}
}

func (f *fp) zero12() *lfe {
	return &lfe{}
}

func (f *fp) lcopy(dst, src *lfe) {
	dst.Set(src)
}

func (f *fp) copyMixed(dst *lfe, src *fe) {
	dst.SetSingle(src)
}

func (f *fp) add12(c, a, b *lfe) {
	add12(c, a, b)
}

func (f *fp) addAssign12(a, b *lfe) {
	add_assign_12(a, b)
}

func (f *fp) ladd12(c, a, b *lfe) {
	ladd12(c, a, b)
}

func (f *fp) double12(c, a *lfe) {
	double12(c, a)
}

func (f *fp) doubleAssign12(a *lfe) {
	double_assign_12(a)
}

func (f *fp) ldouble12(c, a *lfe) {
	ldouble12(c, a)
}

func (f *fp) sub12(c, a, b *lfe) {
	sub12(c, a, b)
}

func (f *fp) subAssign12(a, b *lfe) {
	sub_assign_12(a, b)
}

func (f *fp) lsub12(c, a, b *lfe) {
	lsub12(c, a, b)
}

func (f *fp) lsubAssign12(a, b *lfe) {
	lsub_assign_12(a, b)
}
