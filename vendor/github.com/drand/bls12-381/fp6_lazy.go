// +build !nolazy

package bls

type fp6 struct {
	f  *fp2
	t  [5]*fe2
	lt [6]*lfe2
	t6 *lfe6
}

func newFp6(f *fp2) *fp6 {
	t := [5]*fe2{}
	lt := [6]*lfe2{}
	for i := 0; i < len(t); i++ {
		t[i] = &fe2{}
	}
	for i := 0; i < len(lt); i++ {
		lt[i] = &lfe2{}
	}
	if f == nil {
		return &fp6{newFp2(nil), t, lt, &lfe6{}}
	}
	return &fp6{f, t, lt, &lfe6{}}
}

func (fp *fp6) lcopy(c, a *lfe6) {
	fp.f.lcopy(&c[0], &a[0])
	fp.f.lcopy(&c[1], &a[1])
	fp.f.lcopy(&c[2], &a[2])
}

func (fp *fp6) mulByNonResidue12(c, a *lfe6) {
	lt := fp.lt
	fp.f.lcopy(lt[0], &a[0])
	fp.f.mulByNonResidue12unsafe(&c[0], &a[2])
	fp.f.lcopy(&c[2], &a[1])
	fp.f.lcopy(&c[1], lt[0])
}

func (fp *fp6) add12(c, a, b *lfe6) {
	fp.f.add12(&c[0], &a[0], &b[0])
	fp.f.add12(&c[1], &a[1], &b[1])
	fp.f.add12(&c[2], &a[2], &b[2])
}

func (fp *fp6) addAssign12(a, b *lfe6) {
	fp.f.addAssign12(&a[0], &b[0])
	fp.f.addAssign12(&a[1], &b[1])
	fp.f.addAssign12(&a[2], &b[2])
}

func (fp *fp6) double12(c, a, b *lfe6) {
	fp.f.double12(&c[0], &a[0])
	fp.f.double12(&c[1], &a[1])
	fp.f.double12(&c[2], &a[2])
}

func (fp *fp6) doubleAssign12(a *lfe6) {
	fp.f.doubleAssign12(&a[0])
	fp.f.doubleAssign12(&a[1])
	fp.f.doubleAssign12(&a[2])
}

func (fp *fp6) sub12(c, a, b *lfe6) {
	fp.f.sub12(&c[0], &a[0], &b[0])
	fp.f.sub12(&c[1], &a[1], &b[1])
	fp.f.sub12(&c[2], &a[2], &b[2])
}

func (fp *fp6) subAssign12(a, b *lfe6) {
	fp.f.subAssign12(&a[0], &b[0])
	fp.f.subAssign12(&a[1], &b[1])
	fp.f.subAssign12(&a[2], &b[2])
}

func (fp *fp6) square(c, a *fe6) {
	fp.lsquare(fp.t6, a)
	fp.reduce(c, fp.t6)
}

func (fp *fp6) mul(c, a, b *fe6) {
	fp.lmul(fp.t6, a, b)
	fp.reduce(c, fp.t6)
}

func (fp *fp6) mulAssign(a, b *fe6) {
	fp.lmul(fp.t6, a, b)
	fp.reduce(a, fp.t6)
}

func (fp *fp6) reduce(c *fe6, a *lfe6) {
	fp.f.reduce(&c[0], &a[0])
	fp.f.reduce(&c[1], &a[1])
	fp.f.reduce(&c[2], &a[2])
}

func (fp *fp6) lmul(c *lfe6, a, b *fe6) {
	t := fp.t
	lt := fp.lt
	fp1 := fp.f.f
	fp2 := fp.f
	v0 := lt[2]
	v1 := lt[3]
	v2 := lt[4]
	fp2.lmul(v0, &a[0], &b[0])
	fp2.lmul(v1, &a[1], &b[1])
	fp2.lmul(v2, &a[2], &b[2])
	fp2.ladd(t[0], &a[1], &a[2])
	fp2.ladd(t[1], &b[1], &b[2])
	fp2.lmul(lt[0], t[0], t[1])
	fp1.subAssign12(&lt[0][0], &v1[0])
	fp1.lsubAssign12(&lt[0][1], &v1[1])
	fp1.subAssign12(&lt[0][0], &v2[0])
	fp1.lsubAssign12(&lt[0][1], &v2[1])
	fp2.mulByNonResidue12unsafe(lt[1], lt[0])
	fp2.add12(&c[0], lt[1], v0)
	fp2.ladd(t[0], &a[0], &a[1])
	fp2.ladd(t[1], &b[0], &b[1])
	fp2.lmul(lt[0], t[0], t[1])
	fp1.subAssign12(&lt[0][0], &v0[0])
	fp1.lsubAssign12(&lt[0][1], &v0[1])
	fp1.subAssign12(&lt[0][0], &v1[0])
	fp1.lsubAssign12(&lt[0][1], &v1[1])
	fp2.mulByNonResidue12unsafe(lt[1], v2)
	fp2.add12(&c[1], lt[1], lt[0])
	fp2.ladd(t[0], &a[0], &a[2])
	fp2.ladd(t[1], &b[0], &b[2])
	fp2.lmul(lt[0], t[0], t[1])
	fp1.subAssign12(&lt[0][0], &v0[0])
	fp1.lsubAssign12(&lt[0][1], &v0[1])
	fp1.subAssign12(&lt[0][0], &v2[0])
	fp1.lsubAssign12(&lt[0][1], &v2[1])
	fp2.add12(&c[2], lt[0], v1)
	fp1.add12(&c[2][0], &lt[0][0], &v1[0])
	fp1.ladd12(&c[2][1], &lt[0][1], &v1[1])
}

func (fp *fp6) lMulBy01(c *lfe6, a *fe6, b0, b1 *fe2) {
	t := fp.t
	lt := fp.lt
	fp2 := fp.f
	fp1 := fp.f.f
	v0 := lt[2]
	v1 := lt[3]
	fp2.lmul(v0, &a[0], b0)
	fp2.lmul(v1, &a[1], b1)
	fp2.ladd(t[0], &a[1], &a[2])
	fp2.lmul(lt[0], t[0], b1)
	fp1.subAssign12(&lt[0][0], &v1[0])
	fp1.lsubAssign12(&lt[0][1], &v1[1])
	fp2.mulByNonResidue12unsafe(lt[1], lt[0])
	fp2.add12(&c[0], lt[1], v0)
	fp2.ladd(t[0], &a[0], &a[1])
	fp2.ladd(t[1], b0, b1)
	fp2.lmul(lt[0], t[0], t[1])
	fp1.subAssign12(&lt[0][0], &v0[0])
	fp1.lsubAssign12(&lt[0][1], &v0[1])
	fp1.sub12(&c[1][0], &lt[0][0], &v1[0])
	fp1.lsub12(&c[1][1], &lt[0][1], &v1[1])
	fp2.ladd(t[0], &a[0], &a[2])
	fp2.lmul(lt[0], t[0], b0)
	fp1.subAssign12(&lt[0][0], &v0[0])
	fp1.lsubAssign12(&lt[0][1], &v0[1])
	fp2.add12(&c[2], lt[0], v1)
}

func (fp *fp6) lsquare(c *lfe6, a *fe6) {
	t := fp.t
	lt := fp.lt
	fp1 := fp.f.f
	fp2 := fp.f
	fp2.lmul(lt[0], &a[0], &a[1])
	fp1.doubleAssign12(&lt[0][0])
	fp1.ldouble12(&lt[0][1], &lt[0][1])
	fp2.lsquare(lt[1], &a[2])
	fp2.mulByNonResidue12unsafe(lt[2], lt[1])
	fp2.add12(&c[1], lt[2], lt[0])
	fp2.subAssign12(lt[0], lt[1])
	fp2.lsquare(lt[1], &a[0])
	fp2.sub(t[0], &a[0], &a[1])
	fp2.addAssign(t[0], &a[2])
	fp2.lsquare(lt[3], t[0])
	fp2.lmul(lt[4], &a[1], &a[2])
	fp1.doubleAssign12(&lt[4][0])
	fp1.ldouble12(&lt[4][1], &lt[4][1])
	fp2.mulByNonResidue12unsafe(lt[2], lt[4])
	fp2.add12(&c[0], lt[2], lt[1])
	fp2.subAssign12(lt[3], lt[1])
	fp2.addAssign12(lt[3], lt[0])
	fp2.add12(&c[2], lt[3], lt[4])
}

func (fp *fp6) mulBy01(c, a *fe6, b0, b1 *fe2) {
	fp.lMulBy01(fp.t6, a, b0, b1)
	fp.reduce(c, fp.t6)
}

func (fp *fp6) mulBy01Assign(a *fe6, b0, b1 *fe2) {
	fp.lMulBy01(fp.t6, a, b0, b1)
	fp.reduce(a, fp.t6)
}
