package key

import "go.dedis.ch/kyber/v3/pairing/bn256"

// Pairing is the main pairing suite used by drand. New interesting curves
// should be allowed by drand, such as BLS12-381.
var Pairing = bn256.NewSuite()

// G1 is the G1 group implementation.
var G1 = Pairing.G1()

// G2 is the G2 group implementation.
var G2 = Pairing.G2()
