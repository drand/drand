package bls

import (
	"fmt"
	"math/big"
)

type PointG2 [3]fe2

func (p *PointG2) Set(p2 *PointG2) *PointG2 {
	p[0][0].Set(&p2[0][0])
	p[1][1].Set(&p2[1][1])
	p[2][0].Set(&p2[2][0])
	p[0][1].Set(&p2[0][1])
	p[1][0].Set(&p2[1][0])
	p[2][1].Set(&p2[2][1])
	return p
}

type G2 struct {
	f *fp2
	t [9]*fe2
}

func NewG2(f *fp2) *G2 {
	t := [9]*fe2{}
	for i := 0; i < 9; i++ {
		t[i] = f.zero()
	}
	if f == nil {
		return &G2{
			f: newFp2(nil),
			t: t,
		}
	}
	return &G2{
		f: f,
		t: t,
	}
}

func (g *G2) FromUncompressed(uncompressed []byte) (*PointG2, error) {
	if len(uncompressed) < 192 {
		return nil, fmt.Errorf("input string should be equal or larger than 192")
	}
	var in [192]byte
	copy(in[:], uncompressed[:192])
	if in[0]&(1<<7) != 0 {
		return nil, fmt.Errorf("compression flag should be zero")
	}
	if in[0]&(1<<5) != 0 {
		return nil, fmt.Errorf("sort flag should be zero")
	}
	if in[0]&(1<<6) != 0 {
		for i, v := range in {
			if (i == 0 && v != 0x40) || (i != 0 && v != 0x00) {
				return nil, fmt.Errorf("input string should be zero when infinity flag is set")
			}
		}
		return g.Zero(), nil
	}
	in[0] &= 0x1f
	x, y := &fe2{}, &fe2{}
	if err := g.f.newElementFromBytes(x, in[:96]); err != nil {
		return nil, err
	}
	if err := g.f.newElementFromBytes(y, in[96:]); err != nil {
		return nil, err
	}
	p := &PointG2{}
	g.f.copy(&p[0], x)
	g.f.copy(&p[1], y)
	g.f.copy(&p[2], &fp2One)
	if !g.IsOnCurve(p) {
		return nil, fmt.Errorf("point is not on curve")
	}
	if !g.isTorsionFree(p) {
		return nil, fmt.Errorf("point is not on correct subgroup")
	}
	return p, nil
}

func (g *G2) ToUncompressed(p *PointG2) []byte {
	out := make([]byte, 192)
	g.Affine(p)
	if g.IsZero(p) {
		out[0] |= 1 << 6
	}
	copy(out[:96], g.f.toBytes(&p[0]))
	copy(out[96:], g.f.toBytes(&p[1]))
	return out
}

func (g *G2) FromCompressed(compressed []byte) (*PointG2, error) {
	if len(compressed) < 96 {
		return nil, fmt.Errorf("input string should be equal or larger than 96")
	}
	var in [96]byte
	copy(in[:], compressed[:])
	if in[0]&(1<<7) == 0 {
		return nil, fmt.Errorf("bad compression")
	}
	if in[0]&(1<<6) != 0 {
		// in[0] == (1 << 6) + (1 << 7)
		for i, v := range in {
			if (i == 0 && v != 0xc0) || (i != 0 && v != 0x00) {
				return nil, fmt.Errorf("input string should be zero when infinity flag is set")
			}
		}
		return g.Zero(), nil
	}
	a := in[0]&(1<<5) != 0
	in[0] &= 0x1f
	x := &fe2{}
	if err := g.f.newElementFromBytes(x, in[:]); err != nil {
		return nil, err
	}
	// solve curve equation
	y := &fe2{}
	g.f.square(y, x)
	g.f.mul(y, y, x)
	g.f.add(y, y, b2)
	if ok := g.f.sqrt(y, y); !ok {
		return nil, fmt.Errorf("point is not on curve")
	}
	// select lexicographically, should be in normalized form
	negYn, negY, yn := &fe2{}, &fe2{}, &fe2{}
	g.f.f.demont(&yn[0], &y[0])
	g.f.f.demont(&yn[1], &y[1])
	g.f.neg(negY, y)
	g.f.neg(negYn, yn)
	if (yn[1].Cmp(&negYn[1]) > 0 != a) || (yn[1].IsZero() && yn[0].Cmp(&negYn[0]) > 0 != a) {
		g.f.copy(y, negY)
	}
	p := &PointG2{}
	g.f.copy(&p[0], x)
	g.f.copy(&p[1], y)
	g.f.copy(&p[2], &fp2One)
	if !g.isTorsionFree(p) {
		return nil, fmt.Errorf("point is not on correct subgroup")
	}
	return p, nil
}

func (g *G2) ToCompressed(p *PointG2) []byte {
	out := make([]byte, 96)
	g.Affine(p)
	if g.IsZero(p) {
		out[0] |= 1 << 6
	} else {
		copy(out[:], g.f.toBytes(&p[0]))
		y, negY := &fe2{}, &fe2{}
		g.f.copy(y, &p[1])
		g.f.f.demont(&y[0], &y[0])
		g.f.f.demont(&y[1], &y[1])
		g.f.neg(negY, y)
		if (y[1].Cmp(&negY[1]) > 0) || (y[1].IsZero() && y[1].Cmp(&negY[1]) > 0) {
			out[0] |= 1 << 5
		}
	}
	out[0] |= 1 << 7
	return out
}

func (g *G2) fromRawUnchecked(in []byte) *PointG2 {
	p := &PointG2{}
	if err := g.f.newElementFromBytes(&p[0], in[:96]); err != nil {
		panic(err)
	}
	if err := g.f.newElementFromBytes(&p[1], in[96:]); err != nil {
		panic(err)
	}
	g.f.copy(&p[2], &fp2One)
	return p
}

func (g *G2) isTorsionFree(p *PointG2) bool {
	tmp := &PointG2{}
	g.MulScalar(tmp, p, q)
	return g.IsZero(tmp)
}

func (g *G2) Zero() *PointG2 {
	return &PointG2{
		*g.f.zero(),
		*g.f.one(),
		*g.f.zero(),
	}
}

func (g *G2) One() *PointG2 {
	return g.Copy(&PointG2{}, &g2One)
}

func (g *G2) Copy(dst *PointG2, src *PointG2) *PointG2 {
	return dst.Set(src)
}

func (g *G2) IsZero(p *PointG2) bool {
	return g.f.isZero(&p[2])
}

func (g *G2) Equal(p1, p2 *PointG2) bool {
	if g.IsZero(p1) {
		return g.IsZero(p2)
	}
	if g.IsZero(p2) {
		return g.IsZero(p1)
	}
	t := g.t
	g.f.square(t[0], &p1[2])
	g.f.square(t[1], &p2[2])
	g.f.mul(t[2], t[0], &p2[0])
	g.f.mul(t[3], t[1], &p1[0])
	g.f.mul(t[0], t[0], &p1[2])
	g.f.mul(t[1], t[1], &p2[2])
	g.f.mul(t[1], t[1], &p1[1])
	g.f.mul(t[0], t[0], &p2[1])
	return g.f.equal(t[0], t[1]) && g.f.equal(t[2], t[3])
}

func (g *G2) IsOnCurve(p *PointG2) bool {
	if g.IsZero(p) {
		return true
	}
	t := g.t
	g.f.square(t[0], &p[1])
	g.f.square(t[1], &p[0])
	g.f.mul(t[1], t[1], &p[0])
	g.f.square(t[2], &p[2])
	g.f.square(t[3], t[2])
	g.f.mul(t[2], t[2], t[3])
	g.f.mul(t[2], b2, t[2])
	g.f.add(t[1], t[1], t[2])
	return g.f.equal(t[0], t[1])
}

func (g *G2) IsAffine(p *PointG2) bool {
	return g.f.equal(&p[2], &fp2One)
}

func (g *G2) Affine(p *PointG2) {
	if g.IsZero(p) {
		return
	}
	if !g.IsAffine(p) {
		t := g.t
		g.f.inverse(t[0], &p[2])
		g.f.square(t[1], t[0])
		g.f.mul(&p[0], &p[0], t[1])
		g.f.mul(t[0], t[0], t[1])
		g.f.mul(&p[1], &p[1], t[0])
		g.f.copy(&p[2], g.f.one())
	}
}

func (g *G2) Add(r, p1, p2 *PointG2) *PointG2 {
	if g.IsZero(p1) {
		g.Copy(r, p2)
		return r
	}
	if g.IsZero(p2) {
		g.Copy(r, p1)
		return r
	}
	t := g.t
	g.f.square(t[7], &p1[2])
	g.f.mul(t[1], &p2[0], t[7])
	g.f.mul(t[2], &p1[2], t[7])
	g.f.mul(t[0], &p2[1], t[2])
	g.f.square(t[8], &p2[2])
	g.f.mul(t[3], &p1[0], t[8])
	g.f.mul(t[4], &p2[2], t[8])
	g.f.mul(t[2], &p1[1], t[4])
	if g.f.equal(t[1], t[3]) {
		if g.f.equal(t[0], t[2]) {
			return g.Double(r, p1)
		} else {
			return g.Copy(r, infinity2)
		}
	}
	g.f.sub(t[1], t[1], t[3])
	g.f.double(t[4], t[1])
	g.f.square(t[4], t[4])
	g.f.mul(t[5], t[1], t[4])
	g.f.sub(t[0], t[0], t[2])
	g.f.double(t[0], t[0])
	g.f.square(t[6], t[0])
	g.f.sub(t[6], t[6], t[5])
	g.f.mul(t[3], t[3], t[4])
	g.f.double(t[4], t[3])
	g.f.sub(&r[0], t[6], t[4])
	g.f.sub(t[4], t[3], &r[0])
	g.f.mul(t[6], t[2], t[5])
	g.f.double(t[6], t[6])
	g.f.mul(t[0], t[0], t[4])
	g.f.sub(&r[1], t[0], t[6])
	g.f.add(t[0], &p1[2], &p2[2])
	g.f.square(t[0], t[0])
	g.f.sub(t[0], t[0], t[7])
	g.f.sub(t[0], t[0], t[8])
	g.f.mul(&r[2], t[0], t[1])
	return r
}

func (g *G2) Double(r, p *PointG2) *PointG2 {
	if g.IsZero(p) {
		g.Copy(r, p)
		return r
	}
	t := g.t
	g.f.square(t[0], &p[0])
	g.f.square(t[1], &p[1])
	g.f.square(t[2], t[1])
	g.f.add(t[1], &p[0], t[1])
	g.f.square(t[1], t[1])
	g.f.sub(t[1], t[1], t[0])
	g.f.sub(t[1], t[1], t[2])
	g.f.double(t[1], t[1])
	g.f.double(t[3], t[0])
	g.f.add(t[0], t[3], t[0])
	g.f.square(t[4], t[0])
	g.f.double(t[3], t[1])
	g.f.sub(&r[0], t[4], t[3])
	g.f.sub(t[1], t[1], &r[0])
	g.f.double(t[2], t[2])
	g.f.double(t[2], t[2])
	g.f.double(t[2], t[2])
	g.f.mul(t[0], t[0], t[1])
	g.f.sub(t[1], t[0], t[2])
	g.f.mul(t[0], &p[1], &p[2])
	g.f.copy(&r[1], t[1])
	g.f.double(&r[2], t[0])
	return r
}

func (g *G2) Neg(r, p *PointG2) *PointG2 {
	g.f.copy(&r[0], &p[0])
	g.f.neg(&r[1], &p[1])
	g.f.copy(&r[2], &p[2])
	return r
}

func (g *G2) Sub(c, a, b *PointG2) *PointG2 {
	d := &PointG2{}
	g.Neg(d, b)
	g.Add(c, a, d)
	return c
}

// negates second operand
func (g *G2) SubUnsafe(c, a, b *PointG2) *PointG2 {
	g.Neg(b, b)
	g.Add(c, a, b)
	return c
}

func (g *G2) MulScalar(c, p *PointG2, e *big.Int) *PointG2 {
	q, n := &PointG2{}, &PointG2{}
	g.Copy(n, p)
	l := e.BitLen()
	for i := 0; i < l; i++ {
		if e.Bit(i) == 1 {
			g.Add(q, q, n)
		}
		g.Double(n, n)
	}
	return g.Copy(c, q)
}

func (g *G2) MulByCofactor(c, p *PointG2) {
	g.MulScalar(c, p, cofactorG2)
}

func (g *G2) MapToPoint(in []byte) *PointG2 {
	x, y := &fe2{}, &fe2{}
	fp2 := g.f
	fp := fp2.f
	err := fp2.newElementFromBytes(x, in)
	if err != nil {
		panic(err)
	}
	for {
		fp2.square(y, x)
		fp2.mul(y, y, x)
		fp2.add(y, y, b2)
		if ok := fp2.sqrt(y, y); ok {
			// favour negative y
			negYn, negY, yn := &fe2{}, &fe2{}, &fe2{}
			fp.demont(&yn[0], &y[0])
			fp.demont(&yn[1], &y[1])
			fp2.neg(negY, y)
			fp2.neg(negYn, yn)
			if yn[1].Cmp(&negYn[1]) > 0 || (yn[1].IsZero() && yn[0].Cmp(&negYn[0]) > 0) {
				fp2.copy(y, y)
			} else {
				fp2.copy(y, negY)
			}
			p := &PointG2{*x, *y, fp2One}
			g.MulByCofactor(p, p)
			return p
		}
		fp2.add(x, x, &fp2One)
	}
}

func hashWithDomainG2(g2 *G2, msg [32]byte, domain [8]byte) *PointG2 {
	xReBytes := [41]byte{}
	xImBytes := [41]byte{}
	xBytes := make([]byte, 96)
	copy(xReBytes[:32], msg[:])
	copy(xReBytes[32:40], domain[:])
	xReBytes[40] = 0x01
	copy(xImBytes[:32], msg[:])
	copy(xImBytes[32:40], domain[:])
	xImBytes[40] = 0x02
	copy(xBytes[16:48], sha256Hash(xImBytes[:]))
	copy(xBytes[64:], sha256Hash(xReBytes[:]))
	return g2.MapToPoint(xBytes)
}

// func (g *G2) MultiExp(r *PointG2, points []*PointG2, powers []*big.Int) (*PointG2, error) {
// 	if len(points) != len(powers) {
// 		return nil, fmt.Errorf("point and scalar vectors should be in same length")
// 	}
// 	var c uint = 3
// 	if len(powers) > 32 {
// 		c = uint(math.Ceil(math.Log10(float64(len(powers)))))
// 	}
// 	bucket_size, numBits := (1<<c)-1, q.BitLen()
// 	windows := make([]PointG2, numBits/int(c)+1)
// 	bucket := make([]PointG2, bucket_size)
// 	acc, sum, zero := g.Zero(), g.Zero(), g.Zero()
// 	s := new(big.Int)
// 	for i, m := 0, 0; i <= numBits; i, m = i+int(c), m+1 {
// 		for i := 0; i < bucket_size; i++ {
// 			g.Copy(&bucket[i], zero)
// 		}
// 		for j := 0; j < len(powers); j++ {
// 			s = powers[j]
// 			index := s.Uint64() & uint64(bucket_size)
// 			if index != 0 {
// 				g.Add(&bucket[index-1], &bucket[index-1], points[j])
// 			}
// 			s.Rsh(s, c)
// 		}
// 		g.Copy(acc, zero)
// 		g.Copy(sum, zero)
// 		for k := bucket_size - 1; k >= 0; k-- {
// 			g.Add(sum, sum, &bucket[k])
// 			g.Add(acc, acc, sum)
// 		}
// 		g.Copy(&windows[m], acc)
// 	}
// 	g.Copy(acc, zero)
// 	for i := len(windows) - 1; i >= 0; i-- {
// 		for j := 0; j < int(c); j++ {
// 			g.Double(acc, acc)
// 		}
// 		g.Add(acc, acc, &windows[i])
// 	}
// 	g.Copy(r, acc)
// 	return r, nil
// }
