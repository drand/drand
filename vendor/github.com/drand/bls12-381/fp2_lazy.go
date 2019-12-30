// +build !nolazy

package bls

type fp2 struct {
	f  *fp
	t  [3]*fe
	lt [3]*lfe
}

func newFp2(f *fp) *fp2 {
	t := [3]*fe{}
	lt := [3]*lfe{}
	for i := 0; i < len(t); i++ {
		t[i] = &fe{}
	}
	for i := 0; i < len(lt); i++ {
		lt[i] = &lfe{}
	}
	if f == nil {
		return &fp2{newFp(), t, lt}
	}
	return &fp2{f, t, lt}
}

func (fp *fp2) zero12() *lfe2 {
	return &lfe2{}
}

func (fp *fp2) lcopy(c, a *lfe2) {
	fp.f.lcopy(&c[0], &a[0])
	fp.f.lcopy(&c[1], &a[1])
}

func (fp *fp2) copyMixed(c *lfe2, a *fe2) {
	fp.f.copyMixed(&c[0], &a[0])
	fp.f.copyMixed(&c[1], &a[1])
}

func (fp *fp2) mulByNonResidue12(a *lfe2) {
	lt := fp.lt
	fp.f.sub12(lt[0], &a[0], &a[1])
	fp.f.add12(&a[1], &a[0], &a[1])
	fp.f.lcopy(&a[0], lt[0])
}

func (fp *fp2) mulByNonResidue12unsafe(c, a *lfe2) {
	fp.f.sub12(&c[0], &a[0], &a[1])
	fp.f.add12(&c[1], &a[0], &a[1])
}

func (fp *fp2) add12(c, a, b *lfe2) {
	fp.f.add12(&c[0], &a[0], &b[0])
	fp.f.add12(&c[1], &a[1], &b[1])
}

func (fp *fp2) addAssign12(a, b *lfe2) {
	fp.f.addAssign12(&a[0], &b[0])
	fp.f.addAssign12(&a[1], &b[1])
}

func (fp *fp2) ladd12(c, a, b *lfe2) {
	fp.f.ladd12(&c[0], &a[0], &b[0])
	fp.f.ladd12(&c[1], &a[1], &b[1])
}

func (fp *fp2) double12(c, a *lfe2) {
	fp.f.double12(&c[0], &a[0])
	fp.f.double12(&c[1], &a[1])
}

func (fp *fp2) doubleAssign12(a *lfe2) {
	fp.f.doubleAssign12(&a[0])
	fp.f.doubleAssign12(&a[1])
}

func (fp *fp2) ldouble12(c, a *lfe2) {
	fp.f.ldouble12(&c[0], &a[0])
	fp.f.ldouble12(&c[1], &a[1])
}

func (fp *fp2) sub12(c, a, b *lfe2) {
	fp.f.sub12(&c[0], &a[0], &b[0])
	fp.f.sub12(&c[1], &a[1], &b[1])
}

func (fp *fp2) subAssign12(a, b *lfe2) {
	fp.f.subAssign12(&a[0], &b[0])
	fp.f.subAssign12(&a[1], &b[1])
}

func (fp *fp2) lsub12(c, a, b *lfe2) {
	fp.f.lsub12(&c[0], &a[0], &b[0])
	fp.f.lsub12(&c[1], &a[1], &b[1])
}

func (fp *fp2) submixed12(c, a, b *lfe2) {
	fp.f.sub12(&c[0], &a[0], &b[0])
	fp.f.lsub12(&c[1], &a[1], &b[1])
}

func (fp *fp2) mul(c, a, b *fe2) {
	t := fp.t
	lt := fp.lt
	fp.f.lmul(lt[0], &a[0], &b[0])
	fp.f.lmul(lt[1], &a[1], &b[1])
	fp.f.ladd(t[0], &a[0], &a[1])
	fp.f.ladd(t[1], &b[0], &b[1])
	fp.f.lmul(lt[2], t[0], t[1])
	fp.f.lsubAssign12(lt[2], lt[0])
	fp.f.lsubAssign12(lt[2], lt[1])
	fp.f.reduce(&c[1], lt[2])
	fp.f.subAssign12(lt[0], lt[1])
	fp.f.reduce(&c[0], lt[0])
}

func (fp *fp2) mulAssign(a, b *fe2) {
	t := fp.t
	lt := fp.lt
	fp.f.lmul(lt[0], &a[0], &b[0])
	fp.f.lmul(lt[1], &a[1], &b[1])
	fp.f.ladd(t[0], &a[0], &a[1])
	fp.f.ladd(t[1], &b[0], &b[1])
	fp.f.lmul(lt[2], t[0], t[1])
	fp.f.lsubAssign12(lt[2], lt[0])
	fp.f.lsubAssign12(lt[2], lt[1])
	fp.f.reduce(&a[1], lt[2])
	fp.f.subAssign12(lt[0], lt[1])
	fp.f.reduce(&a[0], lt[0])
}

func (fp *fp2) lmul(c *lfe2, a, b *fe2) {
	t := fp.t
	lt := fp.lt
	fp.f.lmul(lt[0], &a[0], &b[0])
	fp.f.lmul(lt[1], &a[1], &b[1])
	fp.f.ladd(t[0], &a[0], &a[1])
	fp.f.ladd(t[1], &b[0], &b[1])
	fp.f.lmul(lt[2], t[0], t[1])
	fp.f.lsubAssign12(lt[2], lt[0])
	fp.f.lsub12(&c[1], lt[2], lt[1])
	fp.f.sub12(&c[0], lt[0], lt[1])
}

func (fp *fp2) lsquare(c *lfe2, a *fe2) {
	t := fp.t
	fp.f.ladd(t[0], &a[0], &a[1])
	fp.f.lsub(t[1], &a[0], &a[1])
	fp.f.lmul(&c[0], t[0], t[1])
	fp.f.ldouble(t[2], &a[0])
	fp.f.lmul(&c[1], t[2], &a[1])
}

func (fp *fp2) reduce(c *fe2, a *lfe2) {
	fp.f.reduce(&c[0], &a[0])
	fp.f.reduce(&c[1], &a[1])
}

// func (fp *fp2) lmulOpt1H2(c *lfe2, a, b *fe2) {
// 	t := fp.t
// 	fp.f.lmul(lt[0], &a[0], &b[0])
// 	fp.f.lmul(lt[1], &a[1], &b[1])
// 	fp.f.ladd(t[0], &a[0], &a[1])
// 	fp.f.ladd(t[1], &b[0], &b[1])
// 	fp.f.lmul(lt[2], t[0], t[1])
// 	fp.f.lsub12(lt[2], lt[2], lt[0])
// 	fp.f.lsub12(&c[1], lt[2], lt[1])
// 	fp.f.lsub12opt1h2(&c[0], lt[0], lt[1])
// }

// func (fp *fp2) lmulOpt1H1(c *lfe2, a, b *fe2) {
// 	t := fp.t
// 	fp.f.lmul(lt[0], &a[0], &b[0])
// 	fp.f.lmul(lt[1], &a[1], &b[1])
// 	fp.f.ladd(t[0], &a[0], &a[1])
// 	fp.f.ladd(t[1], &b[0], &b[1])
// 	fp.f.lmul(lt[2], t[0], t[1])
// 	fp.f.lsub12(lt[2], lt[2], lt[0])
// 	fp.f.lsub12(&c[1], lt[2], lt[1])
// 	fp.f.lsub12opt1h1(&c[0], lt[0], lt[1])
// }
