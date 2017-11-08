package mod

import (
	"math/big"
)

// Jacobi computes the Jacobi symbol of (x/y) using Euclid's algorithm.
// This is usually much faster modular multiplication via Euler's criterion.
func Jacobi(x, y *big.Int) int {

	// We use the formulation described in chapter 2, section 2.4,
	// "The Yacas Book of Algorithms":
	// http://yacas.sourceforge.net/Algo.book.pdf

	var a, b, c big.Int
	a.Set(x)
	b.Set(y)
	j := 1
	for {
		if a.Cmp(zero) == 0 {
			return 0
		}
		if b.Cmp(one) == 0 {
			return j
		}
		a.Mod(&a, &b)

		// Handle factors of 2 in a
		s := 0
		for a.Bit(s) == 0 {
			s++
		}
		if s&1 != 0 {
			bmod8 := b.Bits()[0] & 7
			if bmod8 == 3 || bmod8 == 5 {
				j = -j
			}
		}
		c.Rsh(&a, uint(s)) // a = 2^s*c

		// Swap numerator and denominator
		if b.Bits()[0]&3 == 3 && c.Bits()[0]&3 == 3 {
			j = -j
		}
		a.Set(&b)
		b.Set(&c)
	}
}
