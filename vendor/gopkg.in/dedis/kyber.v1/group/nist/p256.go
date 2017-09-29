// +build vartime

package nist

import (
	"crypto/elliptic"
	"math/big"
)

// P256 implements the kyber.Group interface
// for the NIST P-256 elliptic curve,
// based on Go's native elliptic curve library.
type p256 struct {
	curve
}

func (curve *p256) String() string {
	return "P256"
}

// Optimized modular square root for P-256 curve, from
// "Mathematical routines for the NIST prime elliptic curves" (April 2010)
func (curve *p256) sqrt(c *big.Int) *big.Int {
	m := curve.p.P

	t1 := new(big.Int)
	t1.Mul(c, c)
	t1.Mul(t1, c) // t1 = c^(2^2-1)

	p2 := new(big.Int)
	p2.SetBit(p2, 2, 1)
	t2 := new(big.Int)
	t2.Exp(t1, p2, m)
	t2.Mul(t2, t1) // t2 = c^(2^4-1)

	p3 := new(big.Int)
	p3.SetBit(p3, 4, 1)
	t3 := new(big.Int)
	t3.Exp(t2, p3, m)
	t3.Mul(t3, t2) // t3 = c^(2^8-1)

	p4 := new(big.Int)
	p4.SetBit(p4, 8, 1)
	t4 := new(big.Int)
	t4.Exp(t3, p4, m)
	t4.Mul(t4, t3) // t4 = c^(2^16-1)

	p5 := new(big.Int)
	p5.SetBit(p5, 16, 1)
	r := new(big.Int)
	r.Exp(t4, p5, m)
	r.Mul(r, t4) // r = c^(2^32-1)

	p6 := new(big.Int)
	p6.SetBit(p6, 32, 1)
	r.Exp(r, p6, m)
	r.Mul(r, c) // r = c^(2^64-2^32+1)

	p7 := new(big.Int)
	p7.SetBit(p7, 96, 1)
	r.Exp(r, p7, m)
	r.Mul(r, c) // r = c^(2^160-2^128+2^96+1)

	p8 := new(big.Int)
	p8.SetBit(p8, 94, 1)
	r.Exp(r, p8, m)

	// r = c^(2^254-2^222+2^190+2^94) = sqrt(c) mod p256
	return r
}

// Initialize standard Curve instances
func (c *p256) Init() curve {
	c.curve.Curve = elliptic.P256()
	c.p = c.Params()
	c.curveOps = c
	return c.curve
}
