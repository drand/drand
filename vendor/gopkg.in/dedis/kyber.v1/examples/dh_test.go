package examples

import (
	"gopkg.in/dedis/kyber.v1/group/edwards25519"
	"gopkg.in/dedis/kyber.v1/util/random"
)

/*
This example illustrates how to use the crypto toolkit's kyber.group API
to perform basic Diffie-Hellman key exchange calculations,
using the NIST-standard P256 elliptic curve in this case.
Any other suitable elliptic curve or other cryptographic group may be used
simply by changing the first line that picks the suite.
*/
func Example_diffieHellman() {

	// Crypto setup: NIST-standardized P256 curve with AES-128 and SHA-256
	suite := edwards25519.NewAES128SHA256Ed25519()

	// Alice's public/private keypair
	a := suite.Scalar().Pick(random.Stream) // Alice's private key
	A := suite.Point().Mul(a, nil)          // Alice's public key

	// Bob's public/private keypair
	b := suite.Scalar().Pick(random.Stream) // Alice's private key
	B := suite.Point().Mul(b, nil)          // Alice's public key

	// Assume Alice and Bob have securely obtained each other's public keys.

	// Alice computes their shared secret using Bob's public key.
	SA := suite.Point().Mul(a, B)

	// Bob computes their shared secret using Alice's public key.
	SB := suite.Point().Mul(b, A)

	// They had better be the same!
	if !SA.Equal(SB) {
		panic("Diffie-Hellman key exchange didn't work")
	}
	println("Shared secret: " + SA.String())

	// Output:
}
