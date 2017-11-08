// Package mod contains big.Int arithmetic functions
// that probably belong in Go's math/big package.
package mod

import (
	"math/big"
)

var zero = big.NewInt(0)

// Sqrt sets z to one of the square roots of a modulo p if a square root exists.
// The modulus p must be an odd prime.
// Returns true on success, false if input a is not a square modulo p.
func Sqrt(z *big.Int, a *big.Int, p *big.Int) bool {

	if a.Sign() == 0 {
		z.SetInt64(0) // sqrt(0) = 0
		return true
	}
	if Jacobi(a, p) != 1 {
		return false // a is not a square mod M
	}

	// Break p-1 into s*2^e such that s is odd.
	var s big.Int
	var e int
	s.Sub(p, one)
	for s.Bit(0) == 0 {
		s.Div(&s, two)
		e++
	}

	// Find some non-square n
	var n big.Int
	n.SetInt64(2)
	for Jacobi(&n, p) != -1 {
		n.Add(&n, one)
	}

	// Heart of the Tonelli-Shanks algorithm.
	// Follows the description in
	// "Square roots from 1; 24, 51, 10 to Dan Shanks" by Ezra Brown.
	var x, b, g, t big.Int
	x.Add(&s, one).Div(&x, two).Exp(a, &x, p)
	b.Exp(a, &s, p)
	g.Exp(&n, &s, p)
	r := e
	for {
		// Find the least m such that ord_p(b) = 2^m
		var m int
		t.Set(&b)
		for t.Cmp(one) != 0 {
			t.Exp(&t, two, p)
			m++
		}

		if m == 0 {
			z.Set(&x)
			return true
		}

		t.SetInt64(0).SetBit(&t, r-m-1, 1).Exp(&g, &t, p)
		// t = g^(2^(r-m-1)) mod p
		g.Mul(&t, &t).Mod(&g, p) // g = g^(2^(r-m)) mod p
		x.Mul(&x, &t).Mod(&x, p)
		b.Mul(&b, &g).Mod(&b, p)
		r = m
	}
}
