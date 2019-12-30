// +build nolazy

package bls

type fp12 struct {
	f   *fp6
	t   [5]*fe6
	t2  [9]*fe2
	t12 *fe12
}

func newFp12(f *fp6) *fp12 {
	t := [5]*fe6{}
	t2 := [9]*fe2{}
	for i := 0; i < len(t); i++ {
		t[i] = &fe6{}
	}
	for i := 0; i < len(t2); i++ {
		t2[i] = &fe2{}
	}
	if f == nil {
		return &fp12{newFp6(nil), t, t2, &fe12{}}
	}
	return &fp12{f, t, t2, &fe12{}}
}

func (fp *fp12) mul(c, a, b *fe12) {
	t := fp.t
	fp.f.mul(t[1], &a[0], &b[0])
	fp.f.mul(t[2], &a[1], &b[1])
	fp.f.add(t[0], t[1], t[2])
	fp.f.mulByNonResidue(t[2], t[2])
	fp.f.add(t[3], t[1], t[2])
	fp.f.add(t[1], &a[0], &a[1])
	fp.f.add(t[2], &b[0], &b[1])
	fp.f.mulAssign(t[1], t[2])
	fp.f.copy(&c[0], t[3])
	fp.f.sub(&c[1], t[1], t[0])
}

func (fp *fp12) mulAssign(a, b *fe12) {
	t := fp.t
	fp.f.mul(t[1], &a[0], &b[0])
	fp.f.mul(t[2], &a[1], &b[1])
	fp.f.add(t[0], t[1], t[2])
	fp.f.mulByNonResidue(t[2], t[2])
	fp.f.add(t[3], t[1], t[2])
	fp.f.add(t[1], &a[0], &a[1])
	fp.f.add(t[2], &b[0], &b[1])
	fp.f.mulAssign(t[1], t[2])
	fp.f.copy(&a[0], t[3])
	fp.f.sub(&a[1], t[1], t[0])
}

func (fp *fp12) fp4Square(c0, c1, a0, a1 *fe2) {
	t := fp.t2
	fp2 := fp.f.f
	fp2.square(t[0], a0)
	fp2.square(t[1], a1)
	fp2.mulByNonResidue(t[2], t[1])
	fp2.add(c0, t[2], t[0])
	fp2.add(t[2], a0, a1)
	fp2.squareAssign(t[2])
	fp2.subAssign(t[2], t[0])
	fp2.sub(c1, t[2], t[1])
}
