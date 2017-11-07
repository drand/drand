package proof

import (
	"encoding/hex"
	"fmt"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/group/edwards25519"
)

// This example shows how to build classic ElGamal-style digital signatures
// using the Camenisch/Stadler proof framework and HashProver.
func Example_hashProve1() {

	// Crypto setup
	suite := edwards25519.NewAES128SHA256Ed25519()
	rand := suite.Cipher([]byte("example"))
	B := suite.Point().Base() // standard base point

	// Create a public/private keypair (X,x)
	x := suite.Scalar().Pick(rand) // create a private key x
	X := suite.Point().Mul(x, nil) // corresponding public key X

	// Generate a proof that we know the discrete logarithm of X.
	M := "Hello World!" // message we want to sign
	rep := Rep("X", "x", "B")
	sec := map[string]kyber.Scalar{"x": x}
	pub := map[string]kyber.Point{"B": B, "X": X}
	prover := rep.Prover(suite, sec, pub, nil)
	proof, _ := HashProve(suite, M, rand, prover)
	fmt.Print("Signature:\n" + hex.Dump(proof))

	// Verify the signature against the correct message M.
	verifier := rep.Verifier(suite, pub)
	err := HashVerify(suite, M, verifier, proof)
	if err != nil {
		panic("signature failed to verify!")
	}
	fmt.Println("Signature verified against correct message M.")

	// Now verify the signature against the WRONG message.
	BAD := "Goodbye World!"
	verifier = rep.Verifier(suite, pub)
	err = HashVerify(suite, BAD, verifier, proof)
	fmt.Println("Signature verify against wrong message: " + err.Error())

	// Output:
	// Signature:
	// 00000000  27 fd 13 c3 6e e6 df a5  00 aa 0c 93 a7 b8 21 4b  |'...n.........!K|
	// 00000010  a5 cf 26 c2 a0 99 68 b0  a0 36 9d 7a de 92 95 7a  |..&...h..6.z...z|
	// 00000020  1f 46 d8 67 4a 71 49 c9  7c d2 8f 2b 75 8c cc 83  |.F.gJqI.|..+u...|
	// 00000030  b4 31 0c 6f 6c 2e 75 70  cd 8b 8e 04 b0 54 4f 07  |.1.ol.up.....TO.|
	// Signature verified against correct message M.
	// Signature verify against wrong message: invalid proof: commit mismatch
}

// This example implements Linkable Ring Signatures (LRS) generically
// using the Camenisch/Stadler proof framework and HashProver.
//
// A ring signature proves that the signer owns one of a list of public keys,
// without revealing anything about which public key the signer actually owns.
// A linkable ring signature (LRS) is the same but includes a linkage tag,
// which the signer proves to correspond 1-to-1 with the signer's key,
// but whose relationship to the private key remains secret
// from anyone who does not hold the private key.
// A key-holder who signs multiple messages in the same public "linkage scope"
// will be forced to use the same linkage tag in each such signature,
// enabling others to tell whether two signatures in a given scope
// were produced by the same or different signers.
//
// This scheme is conceptually similar to that of Liu/Wei/Wong in
// "Linkable and Anonymous Signature for Ad Hoc Groups".
// This example implementation is less space-efficient, however,
// because it uses the generic HashProver for Fiat-Shamir noninteractivity
// instead of Liu/Wei/Wong's customized hash-ring structure.
//
func Example_hashProve2() {

	// Crypto setup
	suite := edwards25519.NewAES128SHA256Ed25519()
	rand := suite.Cipher([]byte("example"))
	B := suite.Point().Base() // standard base point

	// Create an anonymity ring of random "public keys"
	X := make([]kyber.Point, 3)
	for i := range X { // pick random points
		X[i] = suite.Point().Pick(rand)
	}

	// Make just one of them an actual public/private keypair (X[mine],x)
	mine := 2                           // only the signer knows this
	x := suite.Scalar().Pick(rand)      // create a private key x
	X[mine] = suite.Point().Mul(x, nil) // corresponding public key X

	// Produce the correct linkage tag for the signature,
	// as a pseudorandom base point multiplied by our private key.
	linkScope := []byte("The Linkage Scope")
	linkHash := suite.Cipher(linkScope)
	linkBase := suite.Point().Pick(linkHash)
	linkTag := suite.Point().Mul(x, linkBase)

	// Generate the proof predicate: an OR branch for each public key.
	sec := map[string]kyber.Scalar{"x": x}
	pub := map[string]kyber.Point{"B": B, "BT": linkBase, "T": linkTag}
	preds := make([]Predicate, len(X))
	for i := range X {
		name := fmt.Sprintf("X[%d]", i) // "X[0]","X[1]",...
		pub[name] = X[i]                // public point value

		// Predicate indicates knowledge of the private key for X[i]
		// and correspondence of the key with the linkage tag
		preds[i] = And(Rep(name, "x", "B"), Rep("T", "x", "BT"))
	}
	pred := Or(preds...) // make a big Or predicate
	fmt.Printf("Linkable Ring Signature Predicate:\n%s\n", pred.String())

	// The prover needs to know which Or branch (mine) is actually true.
	choice := make(map[Predicate]int)
	choice[pred] = mine

	// Generate the signature
	M := "Hello World!" // message we want to sign
	prover := pred.Prover(suite, sec, pub, choice)
	proof, _ := HashProve(suite, M, rand, prover)
	fmt.Print("Linkable Ring Signature:\n" + hex.Dump(proof))

	// Verify the signature
	verifier := pred.Verifier(suite, pub)
	err := HashVerify(suite, M, verifier, proof)
	if err != nil {
		panic("signature failed to verify!")
	}
	fmt.Println("Linkable Ring Signature verified.")

	// Output:
	// Linkable Ring Signature Predicate:
	// (X[0]=x*B && T=x*BT) || (X[1]=x*B && T=x*BT) || (X[2]=x*B && T=x*BT)
	// Linkable Ring Signature:
	// 00000000  45 85 60 73 be bc 55 10  0e 40 44 59 99 ce 5c 76  |E.`s..U..@DY..\v|
	// 00000010  f5 ac 0e 6c e6 00 b0 93  01 41 5b e9 9c 39 fe d0  |...l.....A[..9..|
	// 00000020  65 71 9c 31 f7 b9 ce 81  57 b3 6b 47 41 54 2b d7  |eq.1....W.kGAT+.|
	// 00000030  f8 15 b6 a2 bc 2d b3 e0  fe c2 77 09 6d 93 b4 69  |.....-....w.m..i|
	// 00000040  2f ab 85 4d 65 b1 b6 eb  d8 16 96 5f ae 47 38 1d  |/..Me......_.G8.|
	// 00000050  a7 69 cf 0e 24 04 ff 0a  ec 54 24 4e 09 c0 ec d5  |.i..$....T$N....|
	// 00000060  c7 e9 cd 3c 93 2b 52 f7  f6 ba bc 89 03 0e bd 2d  |...<.+R........-|
	// 00000070  bc be 3a d9 b0 5f cf ba  a8 f7 a7 38 57 e3 67 d0  |..:.._.....8W.g.|
	// 00000080  ef de 78 87 05 20 9a d2  43 2a ff 77 36 62 6a 1a  |..x.. ..C*.w6bj.|
	// 00000090  53 07 d4 34 a9 d5 b4 39  7d 0b 99 c8 23 76 2e b9  |S..4...9}...#v..|
	// 000000a0  48 e3 19 0e 76 69 14 0e  9a 6a ef 6c be 4a df af  |H...vi...j.l.J..|
	// 000000b0  7c 6b 00 8c 7d a0 e4 33  b2 91 cc b4 18 69 ca c0  ||k..}..3.....i..|
	// 000000c0  fd 83 93 4f ca fa ed 2f  b6 43 27 e3 a5 4b a1 0c  |...O.../.C'..K..|
	// 000000d0  b3 fb 4b 2c 82 2e 32 a0  83 12 34 f6 c6 8b 93 03  |..K,..2...4.....|
	// 000000e0  ee f3 b2 f4 06 e4 98 a7  24 2f 51 b8 13 b4 b5 69  |........$/Q....i|
	// 000000f0  94 ad 33 b9 c4 e3 95 8b  7f 18 6d 1e f1 07 3e 0d  |..3.......m...>.|
	// 00000100  c3 86 a1 24 1b 3e e1 59  d5 bd 70 a1 ff f9 7c 07  |...$.>.Y..p...|.|
	// 00000110  8c 9c 52 f7 47 34 46 c9  1a 05 4b 68 57 49 c7 0e  |..R.G4F...KhWI..|
	// 00000120  31 68 8d ca 3f 6a 85 a1  0d f1 cf 9d 21 05 83 f2  |1h..?j......!...|
	// 00000130  35 63 b0 65 a8 50 a5 ee  ec 95 f8 fd 78 de 73 08  |5c.e.P......x.s.|
	// 00000140  35 e7 59 fa 2b 41 20 f8  b6 48 43 62 91 f1 c6 99  |5.Y.+A ..HCb....|
	// 00000150  0e 64 9c 2c 06 fe 84 75  4f ca 03 7f 28 b5 6d 0c  |.d.,...uO...(.m.|
	// 00000160  1d 63 e5 73 26 c4 9f 61  62 5b 5c 34 70 66 d0 e4  |.c.s&..ab[\4pf..|
	// 00000170  ec ca b8 ee 9a 50 07 6b  0c 75 5a a6 77 b3 20 0f  |.....P.k.uZ.w. .|
	// Linkable Ring Signature verified.
}
