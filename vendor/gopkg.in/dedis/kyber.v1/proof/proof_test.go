package proof

import (
	"encoding/hex"
	"fmt"
	"testing"

	"gopkg.in/dedis/kyber.v1"
	"gopkg.in/dedis/kyber.v1/cipher"
	"gopkg.in/dedis/kyber.v1/group/edwards25519"
)

func TestRep(t *testing.T) {
	suite := edwards25519.NewAES128SHA256Ed25519()
	rand := suite.Cipher(cipher.RandomKey)

	x := suite.Scalar().Pick(rand)
	y := suite.Scalar().Pick(rand)
	B := suite.Point().Base()
	X := suite.Point().Mul(x, nil)
	Y := suite.Point().Mul(y, X)
	R := suite.Point().Add(X, Y)

	choice := make(map[Predicate]int)

	// Simple single-secret predicate: prove X=x*B
	log := Rep("X", "x", "B")

	// Two-secret representation: prove R=x*B+y*X
	rep := Rep("R", "x", "B", "y", "X")

	// Make an and-predicate
	and := And(log, rep)
	andx := And(and)

	// Make up a couple incorrect facts
	falseLog := Rep("Y", "x", "B")
	falseRep := Rep("R", "x", "B", "y", "B")

	falseAnd := And(falseLog, falseRep)

	or1 := Or(falseAnd, andx)
	choice[or1] = 1
	or1x := Or(or1) // test trivial case
	choice[or1x] = 0

	or2a := Rep("B", "y", "X")
	or2b := Rep("R", "x", "R")
	or2 := Or(or2a, or2b)
	or2x := Or(or2) // test trivial case

	pred := Or(or1x, or2x)
	choice[pred] = 0

	sval := map[string]kyber.Scalar{"x": x, "y": y}
	pval := map[string]kyber.Point{"B": B, "X": X, "Y": Y, "R": R}
	prover := pred.Prover(suite, sval, pval, choice)
	proof, err := HashProve(suite, "TEST", rand, prover)
	if err != nil {
		panic("prover: " + err.Error())
	}

	verifier := pred.Verifier(suite, pval)
	if err := HashVerify(suite, "TEST", verifier, proof); err != nil {
		panic("verify: " + err.Error())
	}
}

// This code creates a simple discrete logarithm knowledge proof.
// In particular, that the prover knows a secret x
// that is the elliptic curve discrete logarithm of a point X
// with respect to some base B: i.e., X=x*B.
// If we take X as a public key and x as its corresponding private key,
// then this constitutes a "proof of ownership" of the public key X.
func Example_rep1() {
	pred := Rep("X", "x", "B")
	fmt.Println(pred.String())
	// Output: X=x*B
}

// This example shows how to generate and verify noninteractive proofs
// of the statement in the example above, i.e.,
// a proof of ownership of public key X.
func Example_rep2() {
	pred := Rep("X", "x", "B")
	fmt.Println(pred.String())

	// Crypto setup
	suite := edwards25519.NewAES128SHA256Ed25519()
	rand := suite.Cipher([]byte("example"))
	B := suite.Point().Base() // standard base point

	// Create a public/private keypair (X,x)
	x := suite.Scalar().Pick(rand) // create a private key x
	X := suite.Point().Mul(x, nil) // corresponding public key X

	// Generate a proof that we know the discrete logarithm of X.
	sval := map[string]kyber.Scalar{"x": x}
	pval := map[string]kyber.Point{"B": B, "X": X}
	prover := pred.Prover(suite, sval, pval, nil)
	proof, _ := HashProve(suite, "TEST", rand, prover)
	fmt.Print("Proof:\n" + hex.Dump(proof))

	// Verify this knowledge proof.
	verifier := pred.Verifier(suite, pval)
	err := HashVerify(suite, "TEST", verifier, proof)
	if err != nil {
		panic("proof failed to verify!")
	}
	fmt.Println("Proof verified.")

	// Output:
	// X=x*B
	// Proof:
	// 00000000  27 fd 13 c3 6e e6 df a5  00 aa 0c 93 a7 b8 21 4b  |'...n.........!K|
	// 00000010  a5 cf 26 c2 a0 99 68 b0  a0 36 9d 7a de 92 95 7a  |..&...h..6.z...z|
	// 00000020  c2 f0 69 05 69 f1 14 15  b1 38 3d 9c 49 bd c5 89  |..i.i....8=.I...|
	// 00000030  55 05 56 9a 44 31 52 12  c2 37 77 5d 37 13 fa 05  |U.V.D1R..7w]7...|
	// Proof verified.
}

// This code creates a predicate stating that the prover knows a representation
// of point X with respect to two different bases B1 and B2.
// This means the prover knows two secrets x1 and x2
// such that X=x1*B1+x2*B2.
//
// Point X might constitute a Pedersen commitment, for example,
// where x1 is the value being committed to and x2 is a random blinding factor.
// Assuming the discrete logarithm problem is hard in the relevant group
// and the logarithmic relationship between bases B1 and B2 is unknown -
// which we would be true if B1 and B2 are chosen at random, for example -
// then a prover who has committed to point P
// will later be unable to "open" the commitment
// using anything other than secrets x1 and x2.
// The prover can also prove that one of the secrets (say x1)
// is equal to a secret used in the representation of some other point,
// while leaving the other secret (x2) unconstrained.
//
// If the prover does know the relationship between B1 and B2, however,
// then X does not serve as a useful commitment:
// the prover can trivially compute the x1 corresponding to an arbitrary x2.
//
func Example_rep3() {
	pred := Rep("X", "x1", "B1", "x2", "B2")
	fmt.Println(pred.String())
	// Output: X=x1*B1+x2*B2
}

// This code creates an And predicate indicating that
// the prover knows two different secrets x and y,
// such that point X is equal to x*B
// and point Y is equal to y*B.
// This predicate might be used to prove knowledge of
// the private keys corresponding to two public keys X and Y, for example.
func Example_and1() {
	pred := And(Rep("X", "x", "B"), Rep("Y", "y", "B"))
	fmt.Println(pred.String())
	// Output: X=x*B && Y=y*B
}

// This code creates an And predicate indicating that
// the prover knows a single secret value x,
// such that point X1 is equal to x*B1
// and point X2 is equal to x*B2.
// Thus, the prover not only proves knowledge of the discrete logarithm
// of X1 with respect to B1 and of X2 with respect to B2,
// but also proves that those two discrete logarithms are equal.
func Example_and2() {
	pred := And(Rep("X1", "x", "B1"), Rep("X2", "x", "B2"))
	fmt.Println(pred.String())
	// Output: X1=x*B1 && X2=x*B2
}

// This code creates an Or predicate indicating that
// the prover either knows a secret x such that X=x*B,
// or the prover knows a secret y such that Y=y*B.
// This predicate in essence proves knowledge of the private key
// for one of two public keys X or Y,
// without revealing which key the prover owns.
func Example_or1() {
	pred := Or(Rep("X", "x", "B"), Rep("Y", "y", "B"))
	fmt.Println(pred.String())
	// Output: X=x*B || Y=y*B
}

// This code shows how to create and verify Or-predicate proofs,
// such as the one above.
// In this case, we know a secret x such that X=x*B,
// but we don't know a secret y such that Y=y*B,
// because we simply pick Y as a random point
// instead of generating it by scalar multiplication.
// (And if the group is cryptographically secure
// we won't find be able to find such a y.)
func Example_or2() {
	// Create an Or predicate.
	pred := Or(Rep("X", "x", "B"), Rep("Y", "y", "B"))
	fmt.Println("Predicate: " + pred.String())

	// Crypto setup
	suite := edwards25519.NewAES128SHA256Ed25519()
	rand := suite.Cipher([]byte("example"))
	B := suite.Point().Base() // standard base point

	// Create a public/private keypair (X,x) and a random point Y
	x := suite.Scalar().Pick(rand) // create a private key x
	X := suite.Point().Mul(x, nil) // corresponding public key X
	Y := suite.Point().Pick(rand)  // pick a random point Y

	// We'll need to tell the prover which Or clause is actually true.
	// In this case clause 0, the first sub-predicate, is true:
	// i.e., we know a secret x such that X=x*B.
	choice := make(map[Predicate]int)
	choice[pred] = 0

	// Generate a proof that we know the discrete logarithm of X or Y.
	sval := map[string]kyber.Scalar{"x": x}
	pval := map[string]kyber.Point{"B": B, "X": X, "Y": Y}
	prover := pred.Prover(suite, sval, pval, choice)
	proof, _ := HashProve(suite, "TEST", rand, prover)
	fmt.Print("Proof:\n" + hex.Dump(proof))

	// Verify this knowledge proof.
	// The verifier doesn't need the secret values or choice map, of course.
	verifier := pred.Verifier(suite, pval)
	err := HashVerify(suite, "TEST", verifier, proof)
	if err != nil {
		panic("proof failed to verify!")
	}
	fmt.Println("Proof verified.")

	// Output:
	// Predicate: X=x*B || Y=y*B
	// Proof:
	// 00000000  b6 8f 24 dc d3 c0 86 67  42 1d c3 c8 5a 28 62 4d  |..$....gB...Z(bM|
	// 00000010  86 3b c9 69 7c 88 7f 52  9e b3 93 25 2d e6 58 0e  |.;.i|..R...%-.X.|
	// 00000020  2e 49 39 eb a7 6d a0 65  9e 45 f7 c8 98 e9 bd db  |.I9..m.e.E......|
	// 00000030  af 83 ac 80 ed 21 7c c9  ce d1 2d 45 43 05 3e 55  |.....!|...-EC.>U|
	// 00000040  95 3f 7d f5 a8 a4 48 2d  9a 2c 40 27 1c 2c d5 75  |.?}...H-.,@'.,.u|
	// 00000050  f6 57 a9 03 b2 bf ec 8d  e1 8c 59 5b 56 af 59 00  |.W........Y[V.Y.|
	// 00000060  2d 17 6e d0 98 15 24 7e  c6 9e ad c2 55 9e ba 0e  |-.n...$~....U...|
	// 00000070  1f a9 fe 92 47 24 31 a2  a0 88 72 9a 16 2f ab 05  |....G$1...r../..|
	// 00000080  b4 9c 73 96 b3 03 44 c9  3c 8f 6b dd fa 15 d0 dc  |..s...D.<.k.....|
	// 00000090  76 28 8d 01 33 0a 3f 70  e2 72 4d e1 86 d8 07 00  |v(..3.?p.rM.....|
	// 000000a0  ee f3 b2 f4 06 e4 98 a7  24 2f 51 b8 13 b4 b5 69  |........$/Q....i|
	// 000000b0  94 ad 33 b9 c4 e3 95 8b  7f 18 6d 1e f1 07 3e 0d  |..3.......m...>.|
	// Proof verified.
}
