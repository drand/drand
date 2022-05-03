package key

import (
	"crypto/cipher"

	"github.com/drand/kyber"
	bls "github.com/drand/kyber-bls12381"

	// FIXME package github.com/drand/kyber/sign/bls is deprecated: This version is vulnerable to
	// rogue public-key attack and the new version of the protocol should be used to make sure a
	// signature aggregate cannot be verified by a forged key. You can find the protocol in kyber/sign/bdn.
	// Note that only the aggregation is broken against the attack and a later version will merge bls and asmbls.
	// nolint:staticcheck
	sign "github.com/drand/kyber/sign/bls"
	"github.com/drand/kyber/sign/schnorr"
	"github.com/drand/kyber/sign/tbls"
	"github.com/drand/kyber/util/random"
)

// TODO: global variables are evil, make that a config

// Pairing is the main pairing suite used by drand. New interesting curves
// should be allowed by drand, such as BLS12-381.
var Pairing = bls.NewBLS12381Suite()

// KeyGroup is the group used to create the keys
var KeyGroup = Pairing.G1()

// SigGroup is the group used to create the signatures; it must always be
// different than KeyGroup: G1 key group and G2 sig group or G1 sig group and G2
// keygroup.
var SigGroup = Pairing.G2()

// Scheme is the signature scheme used, defining over which curve the signature
// and keys respectively are.
var Scheme = tbls.NewThresholdSchemeOnG2(Pairing)

// AuthScheme is the signature scheme used to identify public identities
var AuthScheme = sign.NewSchemeOnG2(Pairing)

// DKGAuthScheme is the signature scheme used to authentify packets during
// a broadcast during a DKG
var DKGAuthScheme = schnorr.NewScheme(&schnorrSuite{KeyGroup})

type schnorrSuite struct {
	kyber.Group
}

func (s *schnorrSuite) RandomStream() cipher.Stream {
	return random.New()
}
