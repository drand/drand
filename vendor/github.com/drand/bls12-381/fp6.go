// +build nolazy

package bls

type fp6 struct {
	f *fp2
	t [6]*fe2
}

func newFp6(f *fp2) *fp6 {
	t := [6]*fe2{}
	for i := 0; i < len(t); i++ {
		t[i] = &fe2{}
	}
	if f == nil {
		return &fp6{newFp2(nil), t}
	}
	return &fp6{f, t}
}

func (fp *fp6) mont(c *fe6, a *fe6) {
	fp.f.mont(&c[0], &a[0])
	fp.f.mont(&c[1], &a[1])
	fp.f.mont(&c[2], &a[2])
}

func (fp *fp6) mul(c, a, b *fe6) {
	t := fp.t
	fp.f.mul(t[0], &a[0], &b[0])
	fp.f.mul(t[1], &a[1], &b[1])
	fp.f.mul(t[2], &a[2], &b[2])
	fp.f.add(t[3], &a[1], &a[2])
	fp.f.add(t[4], &b[1], &b[2])
	fp.f.mulAssign(t[3], t[4])
	fp.f.add(t[4], t[1], t[2])
	fp.f.subAssign(t[3], t[4])
	fp.f.mulByNonResidue(t[3], t[3])
	fp.f.add(t[5], t[0], t[3])
	fp.f.add(t[3], &a[0], &a[1])
	fp.f.add(t[4], &b[0], &b[1])
	fp.f.mulAssign(t[3], t[4])
	fp.f.add(t[4], t[0], t[1])
	fp.f.subAssign(t[3], t[4])
	fp.f.mulByNonResidue(t[4], t[2])
	fp.f.add(&c[1], t[3], t[4])
	fp.f.add(t[3], &a[0], &a[2])
	fp.f.add(t[4], &b[0], &b[2])
	fp.f.mulAssign(t[3], t[4])
	fp.f.add(t[4], t[0], t[2])
	fp.f.subAssign(t[3], t[4])
	fp.f.add(&c[2], t[1], t[3])
	fp.f.copy(&c[0], t[5])
}

func (fp *fp6) mulAssign(a, b *fe6) {
	t := fp.t
	fp.f.mul(t[0], &a[0], &b[0])
	fp.f.mul(t[1], &a[1], &b[1])
	fp.f.mul(t[2], &a[2], &b[2])
	fp.f.add(t[3], &a[1], &a[2])
	fp.f.add(t[4], &b[1], &b[2])
	fp.f.mulAssign(t[3], t[4])
	fp.f.add(t[4], t[1], t[2])
	fp.f.subAssign(t[3], t[4])
	fp.f.mulByNonResidue(t[3], t[3])
	fp.f.add(t[5], t[0], t[3])
	fp.f.add(t[3], &a[0], &a[1])
	fp.f.add(t[4], &b[0], &b[1])
	fp.f.mulAssign(t[3], t[4])
	fp.f.add(t[4], t[0], t[1])
	fp.f.subAssign(t[3], t[4])
	fp.f.mulByNonResidue(t[4], t[2])
	fp.f.add(&a[1], t[3], t[4])
	fp.f.add(t[3], &a[0], &a[2])
	fp.f.add(t[4], &b[0], &b[2])
	fp.f.mulAssign(t[3], t[4])
	fp.f.add(t[4], t[0], t[2])
	fp.f.subAssign(t[3], t[4])
	fp.f.add(&a[2], t[1], t[3])
	fp.f.copy(&a[0], t[5])
}

func (fp *fp6) square(c, a *fe6) {
	t := fp.t
	fp.f.square(t[0], &a[0])
	fp.f.mul(t[1], &a[0], &a[1])
	fp.f.doubleAssign(t[1])
	fp.f.sub(t[2], &a[0], &a[1])
	fp.f.addAssign(t[2], &a[2])
	fp.f.squareAssign(t[2])
	fp.f.mul(t[3], &a[1], &a[2])
	fp.f.doubleAssign(t[3])
	fp.f.square(t[4], &a[2])
	fp.f.mulByNonResidue(t[5], t[3])
	fp.f.add(&c[0], t[0], t[5])
	fp.f.mulByNonResidue(t[5], t[4])
	fp.f.add(&c[1], t[1], t[5])
	fp.f.addAssign(t[1], t[2])
	fp.f.addAssign(t[1], t[3])
	fp.f.addAssign(t[0], t[4])
	fp.f.sub(&c[2], t[1], t[0])
}

func (fp *fp6) mulBy01Assign(a *fe6, b0, b1 *fe2) {
	t := fp.t
	fp.f.mul(t[0], &a[0], b0)
	fp.f.mul(t[1], &a[1], b1)
	fp.f.add(t[5], &a[1], &a[2])
	fp.f.mul(t[2], b1, t[5])
	fp.f.subAssign(t[2], t[1])
	fp.f.mulByNonResidue(t[2], t[2])
	fp.f.add(t[5], &a[0], &a[2])
	fp.f.mul(t[3], b0, t[5])
	fp.f.subAssign(t[3], t[0])
	fp.f.add(&a[2], t[3], t[1])
	fp.f.add(t[4], b0, b1)
	fp.f.add(t[5], &a[0], &a[1])
	fp.f.mulAssign(t[4], t[5])
	fp.f.subAssign(t[4], t[0])
	fp.f.sub(&a[1], t[4], t[1])
	fp.f.add(&a[0], t[2], t[0])
}

func (fp *fp6) mulBy01(c, a *fe6, b0, b1 *fe2) {
	t := fp.t
	fp.f.mul(t[0], &a[0], b0)
	fp.f.mul(t[1], &a[1], b1)
	fp.f.add(t[2], &a[1], &a[2])
	fp.f.mulAssign(t[2], b1)
	fp.f.subAssign(t[2], t[1])
	fp.f.mulByNonResidue(t[2], t[2])
	fp.f.add(t[3], &a[0], &a[2])
	fp.f.mulAssign(t[3], b0)
	fp.f.subAssign(t[3], t[0])
	fp.f.add(&c[2], t[3], t[1])
	fp.f.add(t[4], b0, b1)
	fp.f.add(t[3], &a[0], &a[1])
	fp.f.mulAssign(t[4], t[3])
	fp.f.subAssign(t[4], t[0])
	fp.f.sub(&c[1], t[4], t[1])
	fp.f.add(&c[0], t[2], t[0])
}
