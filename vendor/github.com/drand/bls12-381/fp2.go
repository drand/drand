// +build nolazy

package bls

type fp2 struct {
	f *fp
	t [4]*fe
}

func newFp2(f *fp) *fp2 {
	t := [4]*fe{}
	for i := 0; i < len(t); i++ {
		t[i] = &fe{}
	}
	if f == nil {
		return &fp2{newFp(), t}
	}
	return &fp2{f, t}
}

func (fp *fp2) mul(c, a, b *fe2) {
	t := fp.t
	fp.f.mul(t[1], &a[0], &b[0])
	fp.f.mul(t[2], &a[1], &b[1])
	fp.f.add(t[0], &a[0], &a[1])
	fp.f.add(t[3], &b[0], &b[1])
	fp.f.sub(&c[0], t[1], t[2])
	fp.f.addAssign(t[1], t[2])
	fp.f.mulAssign(t[0], t[3])
	fp.f.sub(&c[1], t[0], t[1])
}

func (fp *fp2) mulAssign(a, b *fe2) {
	t := fp.t
	fp.f.mul(t[1], &a[0], &b[0])
	fp.f.mul(t[2], &a[1], &b[1])
	fp.f.add(t[0], &a[0], &a[1])
	fp.f.add(t[3], &b[0], &b[1])
	fp.f.sub(&a[0], t[1], t[2])
	fp.f.addAssign(t[1], t[2])
	fp.f.mulAssign(t[0], t[3])
	fp.f.sub(&a[1], t[0], t[1])
}

func (fp *fp2) mont(c *fe2, a *fe2) {
	fp.f.mont(&c[0], &a[0])
	fp.f.mont(&c[1], &a[1])
}
