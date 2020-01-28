package key

import (
	bls "github.com/drand/bls12-381"
	"github.com/drand/kyber/sign/tbls"
)

// TODO: global variables are evil, make that a config

// Pairing is the main pairing suite used by drand. New interesting curves
// should be allowed by drand, such as BLS12-381.
var Pairing = bls.NewBLS12381Suite()

// G1 is the G1 group implementation.
//var G1 = Pairing.G1()

// G2 is the G2 group implementation.
//var G2 = Pairing.G2()

// KeyGroup is the group used to create the keys
var KeyGroup = Pairing.G1()

// SigGroup is the group used to create the signatures; it must always be
// different than KeyGroup: G1 key group and G2 sig group or G1 sig group and G2
// keygroup.
var SigGroup = Pairing.G2()

// Scheme is the signature scheme used, defining over which curve the signature
// and keys respectively are.
var Scheme = tbls.NewThresholdSchemeOnG2(Pairing)
