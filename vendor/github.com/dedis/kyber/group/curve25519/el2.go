// +build vartime

package curve25519

import (
	"math/big"
	//"encoding/hex"
	"crypto/cipher"

	"github.com/dedis/kyber/group/mod"
)

// Elligator 2 parameters
type el2param struct {
	ec     *curve  // back-pointer to curve
	u      mod.Int // u: any non-square element
	A, B   mod.Int // Montgomery curve parameters
	sqrtB  mod.Int // sqrt(B)
	negA   mod.Int // -A
	pp3d8  big.Int // (p+3)/8
	pm1d2  big.Int // (p-1)/2
	sqrtm1 mod.Int // sqrt(-1)
}

// Initialize Elligator 1 parameters given magic point s
func (el *el2param) init(ec *curve, u *big.Int) *el2param {
	el.ec = ec
	el.u.Init(u, &ec.P)

	// Compute the parameters for the Montgomery conversion:
	// A = 2(a+d)/(a-d)
	// B = 4/(a-d)
	// See Bernstein et al, "Twisted Edwards Curves", theorem 3.2
	// http://eprint.iacr.org/2008/013.pdf
	var amd mod.Int
	amd.Sub(&ec.a, &ec.d) // t = a-d
	el.A.Add(&ec.a, &ec.d).Add(&el.A, &el.A).Div(&el.A, &amd)
	el.B.Init64(4, &ec.P).Div(&el.B, &amd)

	// Other precomputed constants
	el.sqrtB.Sqrt(&el.B)
	el.negA.Neg(&el.A)
	el.pp3d8.Add(&ec.P, big.NewInt(3)).Div(&el.pp3d8, big.NewInt(8))
	el.pm1d2.Sub(&ec.P, big.NewInt(1)).Div(&el.pm1d2, big.NewInt(2))
	el.sqrtm1.Init64(-1, &ec.P).Sqrt(&el.sqrtm1)

	return el
}

// Elligator 2 represents points using only the range 0..(p-1)/2.
func (el *el2param) HideLen() int {
	return (el.pm1d2.BitLen() + 7) / 8
}

// Produce a mask representing the padding bits we'll need
// in the most-significant byte of the point representations we produce.
// For Elligator 2 the representation uses only the range 0..(p-1)/2.
func (el *el2param) padmask() byte {
	highbits := 1 + ((el.pm1d2.BitLen() - 1) & 7)
	return byte(0xff << uint(highbits))
}

// Convert point from Twisted Edwards form: ax^2+y^2 = 1+dx^2y^2
// to Montgomery form: v^2 = u^3+Au^2+u
// via the equivalence:
//
//	u = (1+y)/(1-y)
//	v = sqrt(B)u/x
//
// where A=2(a+d)/(a-d) and B=4(a-d)
//
// Beware: the Twisted Edwards Curves paper uses B as a factor for v^2,
// whereas the Elligator 2 paper uses B as a factor for the last u term.
//
func (el *el2param) ed2mont(u, v, x, y *mod.Int) {
	ec := el.ec
	var t1, t2 mod.Int
	u.Div(t1.Add(&ec.one, y), t2.Sub(&ec.one, y))
	v.Mul(u, &el.sqrtB).Div(v, x)
}

// Convert from Montgomery form (u,v) to Edwards (x,y) via:
//
//	x = sqrt(B)u/v
//	y = (u-1)/(u+1)
//
func (el *el2param) mont2ed(x, y, u, v *mod.Int) {
	ec := el.ec
	var t1, t2 mod.Int
	x.Mul(u, &el.sqrtB).Div(x, v)
	y.Div(t1.Sub(u, &ec.one), t2.Add(u, &ec.one))
}

// Compute the square root function,
// specified in section 5.5 of the Elligator paper.
func (el *el2param) sqrt(r, a *mod.Int) {
	var b, b2 mod.Int
	b.Exp(a, &el.pp3d8) // b = a^((p+3)/8); b in {a,-a}

	b2.Mul(&b, &b) // b^2 = a?
	if !b2.Equal(a) {
		b.Mul(&b, &el.sqrtm1) // b*sqrt(-1)
	}

	if b.V.Cmp(&el.pm1d2) > 0 { // |b|
		b.Neg(&b)
	}

	r.Set(&b)
}

// Elligator 2 forward-map from representative to Edwards curve point.
// Currently a straightforward, unoptimized implementation.
// See section 5.2 of the Elligator paper.
func (el *el2param) HideDecode(P point, rep []byte) {
	ec := el.ec
	var r, v, x, y, t1, edx, edy mod.Int

	l := ec.PointLen()
	if len(rep) != l {
		panic("el2Map: wrong representative length")
	}

	// Take the appropriate number of bits from the representative.
	buf := make([]byte, l)
	copy(buf, rep)
	buf[0] &^= el.padmask() // mask off the padding bits
	r.InitBytes(buf, &ec.P, mod.BigEndian)

	// v = -A/(1+ur^2)
	v.Mul(&r, &r).Mul(&el.u, &v).Add(&ec.one, &v).Div(&el.negA, &v)

	// e = Chi(v^3+Av^2+Bv), where B=1 because of ed2mont equivalence
	t1.Add(&v, &el.A).Mul(&t1, &v).Add(&t1, &ec.one).Mul(&t1, &v)
	e := big.Jacobi(&t1.V, t1.M)

	// x = ev-(1-e)A/2
	if e == 1 {
		x.Set(&v)
	} else {
		x.Add(&v, &el.A).Neg(&x)
	}

	// y = -e sqrt(x^3+Ax^2+Bx), where B=1
	y.Add(&x, &el.A).Mul(&y, &x).Add(&y, &ec.one).Mul(&y, &x)
	el.sqrt(&y, &y)
	if e == 1 {
		y.Neg(&y) // -e factor
	}

	// Convert Montgomery to Edwards coordinates
	el.mont2ed(&edx, &edy, &x, &y)

	// Sanity-check
	if !ec.onCurve(&edx, &edy) {
		panic("elligator2 produced invalid point")
	}

	P.initXY(&edx.V, &edy.V, ec.self)
}

// Elligator 2 reverse-map from point to uniform representative.
// Returns nil if point has no uniform representative.
// See section 5.3 of the Elligator paper.
func (el *el2param) HideEncode(P point, rand cipher.Stream) []byte {
	edx, edy := P.getXY()
	var x, y, r, xpA, t1 mod.Int

	// convert Edwards to Montgomery coordinates
	el.ed2mont(&x, &y, edx, edy)

	// condition 1: x != -A
	if x.Equal(&el.negA) {
		return nil // x = -A, no representative
	}

	// condition 2: if y=0, then x=0
	if y.V.Sign() == 0 && x.V.Sign() != 0 {
		return nil // y=0 but x!=0, no representative
	}

	// condition 3: -ux(x+A) is a square
	xpA.Add(&x, &el.A)
	t1.Mul(&el.u, &x).Mul(&t1, &xpA).Neg(&t1)
	if big.Jacobi(&t1.V, t1.M) < 0 {
		return nil // not a square, no representative
	}

	if y.V.Cmp(&el.pm1d2) <= 0 { // y in image of sqrt function
		r.Mul(&xpA, &el.u).Div(&x, &r)
	} else { // y not in image of sqrt function
		r.Mul(&el.u, &x).Div(&xpA, &r)
	}
	r.Neg(&r)
	el.sqrt(&r, &r)

	// Sanity check on result
	if r.V.Cmp(&el.pm1d2) > 0 {
		panic("el2: r too big")
	}

	// Map representative to a byte-string by padding the upper byte.
	// This assumes that the prime c.P is close enough to a power of 2
	// that the adversary will never notice the "missing" values;
	// this is true for the class of curves Elligator1 was designed for.
	rep, _ := r.MarshalBinary()
	padmask := el.padmask()
	if padmask != 0 {
		var pad [1]byte
		rand.XORKeyStream(pad[:], pad[:])
		rep[0] |= pad[0] & padmask
	}
	return rep
}
