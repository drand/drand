// +build vartime

package curve25519

import (
	"math/big"
	//"encoding/hex"
	"crypto/cipher"

	"github.com/dedis/kyber/group/mod"
)

func chi(r, v *mod.Int) {
	r.Init64(int64(big.Jacobi(&v.V, v.M)), v.M)
}

// Elligator 1 parameters
type el1param struct {
	ec      *curve  // back-pointer to curve
	c, r, s mod.Int // c,r,s parameters
	r2m2    mod.Int // r^2-2
	invc2   mod.Int // 1/c^2
	pp1d4   big.Int // (p+1)/4
	cm1s    mod.Int // (c-1)s
	m2      mod.Int // -2
	c3x     mod.Int // 2s(c-1)Chi(c)/r
}

// Initialize Elligator 1 parameters given magic point s
func (el *el1param) init(ec *curve, s *big.Int) *el1param {
	var two, invc, cm1, d mod.Int

	el.ec = ec
	el.s.Init(s, &ec.P)

	// c = 2/s^2
	two.Init64(2, &ec.P)
	el.c.Mul(&el.s, &el.s).Div(&two, &el.c)

	// r = c+1/c
	invc.Inv(&el.c)
	el.r.Add(&el.c, &invc)

	// Precomputed values
	el.r2m2.Mul(&el.r, &el.r).Sub(&el.r2m2, &two)          // r^2-2
	el.invc2.Mul(&invc, &invc)                             // 1/c^2
	el.pp1d4.Add(&ec.P, one).Div(&el.pp1d4, big.NewInt(4)) // (p+1)/4
	cm1.Sub(&el.c, &ec.one)
	el.cm1s.Mul(&cm1, &el.s) // (c-1)s
	el.m2.Init64(-2, &ec.P)  // -2

	// 2s(c-1)Chi(c)/r
	chi(&el.c3x, &el.c)
	el.c3x.Mul(&el.c3x, &two).Mul(&el.c3x, &el.s).Mul(&el.c3x, &cm1)
	el.c3x.Div(&el.c3x, &el.r)

	// Sanity check: d = -(c+1)^2/(c-1)^2
	d.Add(&el.c, &ec.one).Div(&d, &cm1).Mul(&d, &d).Neg(&d)
	if d.Cmp(&ec.d) != 0 {
		panic("el1 init: d came out wrong")
	}

	return el
}

func (el *el1param) HideLen() int {
	return el.ec.PointLen()
}

// Produce a mask representing the padding bits we'll need
// in the most-significant byte of the point representations we produce.
// For Elligator 1 the representation uses the full range from 0 to p-1.
func (el *el1param) padmask() byte {
	highbits := 1 + ((el.ec.P.BitLen() - 1) & 7)
	return byte(0xff << uint(highbits))
}

// Elligator 1 forward-map from representative to Edwards curve point.
// Currently a straightforward, unoptimized implementation.
// See section 3.2 of the Elligator paper.
func (el *el1param) HideDecode(P point, rep []byte) {
	ec := el.ec
	var t, u, u2, v, Chiv, X, Y, x, y, t1, t2 mod.Int

	l := ec.PointLen()
	if len(rep) != l {
		panic("el1Map: wrong representative length")
	}

	// Take the appropriate number of bits from the representative.
	b := make([]byte, l)
	copy(b, rep)
	b[0] &^= el.padmask() // mask off the padding bits
	t.InitBytes(b, &ec.P, mod.BigEndian)

	// u = (1-t)/(1+t)
	u.Div(t1.Sub(&ec.one, &t), t2.Add(&ec.one, &t))

	// v = u^5 + (r^2-2)u^3 + u
	u2.Mul(&u, &u)                   // u2 = u^2
	v.Mul(&u2, &u2)                  // v = u^4
	v.Add(&v, t1.Mul(&el.r2m2, &u2)) // v = u^4 + (r^2-2)u^2
	v.Add(&v, &ec.one).Mul(&v, &u)   // v = u^5 + (r^2-2)u^3 + u

	// X = Chi(v)u
	chi(&Chiv, &v)
	X.Mul(&Chiv, &u)

	// Y = (Chi(v)v)^((q+1)/4) Chi(v) Chi(u^2+1/c^2)
	t1.Add(&u2, &el.invc2)
	chi(&t1, &t1) // t1 = Chi(u^2+1/c^2)
	Y.Mul(&Chiv, &v)
	Y.Exp(&Y, &el.pp1d4).Mul(&Y, &Chiv).Mul(&Y, &t1)

	// x = (c-1)sX(1+X)/Y
	x.Add(&ec.one, &X).Mul(&X, &x).Mul(&el.cm1s, &x).Div(&x, &Y)

	// y = (rX-(1+X)^2)/(rX+(1+X)^2)
	t1.Mul(&el.r, &X)                 // t1 = rX
	t2.Add(&ec.one, &X).Mul(&t2, &t2) // t2 = (1+X)^2
	y.Div(u.Sub(&t1, &t2), v.Add(&t1, &t2))

	// Sanity-check
	if !ec.onCurve(&x, &y) {
		panic("elligator1 produced invalid point")
	}

	P.initXY(&x.V, &y.V, ec.self)
}

// Elligator 1 reverse-map from point to uniform representative.
// Returns nil if point has no uniform representative.
// See section 3.3 of the Elligator paper.
func (el *el1param) HideEncode(P point, rand cipher.Stream) []byte {
	ec := el.ec
	x, y := P.getXY()
	var a, b, etar, etarp1, X, z, u, t, t1 mod.Int

	// condition 1: a = y+1 is nonzero
	a.Add(y, &ec.one)
	if a.V.Sign() == 0 {
		return nil // y+1 = 0, no representative
	}

	// etar = r(y-1)/2(y+1)
	t1.Add(y, &ec.one).Add(&t1, &t1) // 2(y+1)
	etar.Sub(y, &ec.one).Mul(&etar, &el.r).Div(&etar, &t1)

	// condition 2: b = (1 + eta r)^2 - 1 is a square
	etarp1.Add(&ec.one, &etar) // etarp1 = (1 + eta r)
	b.Mul(&etarp1, &etarp1).Sub(&b, &ec.one)
	if big.Jacobi(&b.V, b.M) < 0 {
		return nil // b not a square, no representative
	}

	// condition 3: if etar = -2 then x=2s(c-1)Chi(c)/r
	if etar.Equal(&el.m2) && !x.Equal(&el.c3x) {
		return nil
	}

	// X = -(1+eta r)+((1+eta r)^2-1)^((q+1)/4)
	X.Exp(&b, &el.pp1d4).Sub(&X, &etarp1)

	// z = Chi((c-1)sX(1+X)x(X^2+1/c^2))
	z.Mul(&el.cm1s, &X).Mul(&z, t.Add(&ec.one, &X)).Mul(&z, x)
	z.Mul(&z, t.Mul(&X, &X).Add(&t, &el.invc2))
	chi(&z, &z)

	// u = zX
	u.Mul(&z, &X)

	// t = (1-u)/(1+u)
	t.Div(a.Sub(&ec.one, &u), b.Add(&ec.one, &u))

	// Map representative to a byte-string by padding the upper byte.
	// This assumes that the prime c.P is close enough to a power of 2
	// that the adversary will never notice the "missing" values;
	// this is true for the class of curves Elligator1 was designed for.
	rep, _ := t.MarshalBinary()
	padmask := el.padmask()
	if padmask != 0 {
		var pad [1]byte
		rand.XORKeyStream(pad[:], pad[:])
		rep[0] |= pad[0] & padmask
	}
	return rep
}
