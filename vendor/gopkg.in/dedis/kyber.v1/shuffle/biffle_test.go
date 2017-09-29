package shuffle

import (
	"testing"

	kyber "gopkg.in/dedis/kyber.v1"
	"gopkg.in/dedis/kyber.v1/cipher"
	"gopkg.in/dedis/kyber.v1/proof"
)

func TestBiffle(t *testing.T) {
	biffleTest(suite, N)
}

func biffleTest(suite Suite, N int) {

	rand := suite.Cipher(cipher.RandomKey)

	// Create a "server" private/public keypair
	h := suite.Scalar().Pick(rand)
	H := suite.Point().Mul(h, nil)

	// Create a set of ephemeral "client" keypairs to shuffle
	var c [2]kyber.Scalar
	var C [2]kyber.Point
	//	fmt.Println("\nclient keys:")
	for i := 0; i < 2; i++ {
		c[i] = suite.Scalar().Pick(rand)
		C[i] = suite.Point().Mul(c[i], nil)
		//		fmt.Println(" "+C[i].String())
	}

	// ElGamal-encrypt all these keypairs with the "server" key
	var X, Y [2]kyber.Point
	r := suite.Scalar() // temporary
	for i := 0; i < 2; i++ {
		r.Pick(rand)
		X[i] = suite.Point().Mul(r, nil)
		Y[i] = suite.Point().Mul(r, H) // ElGamal blinding factor
		Y[i].Add(Y[i], C[i])           // Encrypted client public key
	}

	// Repeat only the actual shuffle portion for test purposes.
	for i := 0; i < N; i++ {

		// Do a key-shuffle
		Xbar, Ybar, prover := Biffle(suite, nil, H, X, Y, rand)
		prf, err := proof.HashProve(suite, "Biffle", rand, prover)
		if err != nil {
			panic("Biffle proof failed: " + err.Error())
		}
		//fmt.Printf("proof:\n%s\n",hex.Dump(prf))

		// Check it
		verifier := BiffleVerifier(suite, nil, H, X, Y, Xbar, Ybar)
		err = proof.HashVerify(suite, "Biffle", verifier, prf)
		if err != nil {
			panic("Biffle verify failed: " + err.Error())
		}
	}
}
