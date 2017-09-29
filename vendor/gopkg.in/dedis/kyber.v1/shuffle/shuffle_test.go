package shuffle

import (
	//"fmt"
	//"encoding/hex"

	"testing"

	kyber "gopkg.in/dedis/kyber.v1"
	"gopkg.in/dedis/kyber.v1/cipher"
	"gopkg.in/dedis/kyber.v1/group/edwards25519"
	"gopkg.in/dedis/kyber.v1/proof"
)

var suite = edwards25519.NewAES128SHA256Ed25519()
var k = 5
var N = 10

func TestShuffle(t *testing.T) {
	shuffleTest(suite, k, N)
}

func shuffleTest(suite Suite, k, N int) {
	rand := suite.Cipher(cipher.RandomKey)

	// Create a "server" private/public keypair
	h := suite.Scalar().Pick(rand)
	H := suite.Point().Mul(h, nil)

	// Create a set of ephemeral "client" keypairs to shuffle
	c := make([]kyber.Scalar, k)
	C := make([]kyber.Point, k)
	//	fmt.Println("\nclient keys:")
	for i := 0; i < k; i++ {
		c[i] = suite.Scalar().Pick(rand)
		C[i] = suite.Point().Mul(c[i], nil)
		//		fmt.Println(" "+C[i].String())
	}

	// ElGamal-encrypt all these keypairs with the "server" key
	X := make([]kyber.Point, k)
	Y := make([]kyber.Point, k)
	r := suite.Scalar() // temporary
	for i := 0; i < k; i++ {
		r.Pick(rand)
		X[i] = suite.Point().Mul(r, nil)
		Y[i] = suite.Point().Mul(r, H) // ElGamal blinding factor
		Y[i].Add(Y[i], C[i])           // Encrypted client public key
	}

	// Repeat only the actual shuffle portion for test purposes.
	for i := 0; i < N; i++ {

		// Do a key-shuffle
		Xbar, Ybar, prover := Shuffle(suite, nil, H, X, Y, rand)
		prf, err := proof.HashProve(suite, "PairShuffle", rand, prover)
		if err != nil {
			panic("Shuffle proof failed: " + err.Error())
		}
		//fmt.Printf("proof:\n%s\n",hex.Dump(prf))

		// Check it
		verifier := Verifier(suite, nil, H, X, Y, Xbar, Ybar)
		err = proof.HashVerify(suite, "PairShuffle", verifier, prf)
		if err != nil {
			panic("Shuffle verify failed: " + err.Error())
		}
	}
}
