package anon

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"gopkg.in/dedis/kyber.v1"
	"gopkg.in/dedis/kyber.v1/group/edwards25519"
	"gopkg.in/dedis/kyber.v1/util/random"
)

// This example demonstrates signing and signature verification
// using a trivial "anonymity set" of size 1, i.e., no anonymity.
// In this special case the signing scheme devolves to
// producing traditional ElGamal signatures:
// the resulting signatures are exactly the same length
// and represent essentially the same computational cost.
func Example_sign1() {

	// Crypto setup
	suite := edwards25519.NewAES128SHA256Ed25519()
	rand := suite.Cipher([]byte("example"))

	// Create a public/private keypair (X[mine],x)
	X := make([]kyber.Point, 1)
	mine := 0                           // which public key is mine
	x := suite.Scalar().Pick(rand)      // create a private key x
	X[mine] = suite.Point().Mul(x, nil) // corresponding public key X

	// Generate the signature
	M := []byte("Hello World!") // message we want to sign
	sig := Sign(suite, rand, M, Set(X), nil, mine, x)
	fmt.Print("Signature:\n" + hex.Dump(sig))

	// Verify the signature against the correct message
	tag, err := Verify(suite, M, Set(X), nil, sig)
	if err != nil {
		panic(err.Error())
	}
	if tag == nil || len(tag) != 0 {
		panic("Verify returned wrong tag")
	}
	fmt.Println("Signature verified against correct message.")

	// Verify the signature against the wrong message
	BAD := []byte("Goodbye world!")
	tag, err = Verify(suite, BAD, Set(X), nil, sig)
	if err == nil || tag != nil {
		panic("Signature verified against wrong message!?")
	}
	fmt.Println("Verifying against wrong message: " + err.Error())

	// Output:
	// Signature:
	// 00000000  be db 1a 8e 63 21 a2 96  68 17 85 05 e7 aa dc fd  |....c!..h.......|
	// 00000010  09 d8 36 e6 00 39 f8 98  69 4a 70 dc 4e a2 07 07  |..6..9..iJp.N...|
	// 00000020  1f 46 d8 67 4a 71 49 c9  7c d2 8f 2b 75 8c cc 83  |.F.gJqI.|..+u...|
	// 00000030  b4 31 0c 6f 6c 2e 75 70  cd 8b 8e 04 b0 54 4f 07  |.1.ol.up.....TO.|
	// Signature verified against correct message.
	// Verifying against wrong message: invalid signature
}

// This example demonstrates how to create unlinkable anonymity-set signatures,
// and to verify them,
// using a small anonymity set containing three public keys.
func ExampleSign_anonSet() {

	// Crypto setup
	suite := edwards25519.NewAES128SHA256Ed25519()
	rand := suite.Cipher([]byte("example"))

	// Create an anonymity set of random "public keys"
	X := make([]kyber.Point, 3)
	for i := range X { // pick random points
		X[i] = suite.Point().Pick(rand)
	}

	// Make just one of them an actual public/private keypair (X[mine],x)
	mine := 1                           // only the signer knows this
	x := suite.Scalar().Pick(rand)      // create a private key x
	X[mine] = suite.Point().Mul(x, nil) // corresponding public key X

	// Generate the signature
	M := []byte("Hello World!") // message we want to sign
	sig := Sign(suite, rand, M, Set(X), nil, mine, x)
	fmt.Print("Signature:\n" + hex.Dump(sig))

	// Verify the signature against the correct message
	tag, err := Verify(suite, M, Set(X), nil, sig)
	if err != nil {
		panic(err.Error())
	}
	if tag == nil || len(tag) != 0 {
		panic("Verify returned wrong tag")
	}
	fmt.Println("Signature verified against correct message.")

	// Verify the signature against the wrong message
	BAD := []byte("Goodbye world!")
	tag, err = Verify(suite, BAD, Set(X), nil, sig)
	if err == nil || tag != nil {
		panic("Signature verified against wrong message!?")
	}
	fmt.Println("Verifying against wrong message: " + err.Error())

	// Output:
	// Signature:
	// 00000000  a4 00 4d 45 a6 cd f6 b4  1b e0 d1 12 54 49 ee c3  |..ME........TI..|
	// 00000010  d6 8a bd c5 7d 16 b4 a8  ec 6e b2 ce 51 5c fc 0a  |....}....n..Q\..|
	// 00000020  31 68 8d ca 3f 6a 85 a1  0d f1 cf 9d 21 05 83 f2  |1h..?j......!...|
	// 00000030  35 63 b0 65 a8 50 a5 ee  ec 95 f8 fd 78 de 73 08  |5c.e.P......x.s.|
	// 00000040  a7 89 3b ae 4d cb 44 c0  19 49 11 63 25 21 13 ce  |..;.M.D..I.c%!..|
	// 00000050  0e d3 d3 e3 89 af db cd  23 8f 3f 60 06 1b a1 0e  |........#.?`....|
	// 00000060  ee f3 b2 f4 06 e4 98 a7  24 2f 51 b8 13 b4 b5 69  |........$/Q....i|
	// 00000070  94 ad 33 b9 c4 e3 95 8b  7f 18 6d 1e f1 07 3e 0d  |..3.......m...>.|
	// Signature verified against correct message.
	// Verifying against wrong message: invalid signature
}

// This example demonstrates the creation of linkable anonymity set signatures,
// and verification, using an anonymity set containing three public keys.
// We produce four signatures, two from each of two private key-holders,
// demonstrating how the resulting verifiable tags distinguish
// signatures by the same key-holder from signatures by different key-holders.
func ExampleSign_linkable() {

	// Crypto setup
	suite := edwards25519.NewAES128SHA256Ed25519()
	rand := suite.Cipher([]byte("example"))

	// Create an anonymity set of random "public keys"
	X := make([]kyber.Point, 3)
	for i := range X { // pick random points
		X[i] = suite.Point().Pick(rand)
	}

	// Make two actual public/private keypairs (X[mine],x)
	mine1 := 1 // only the signer knows this
	mine2 := 2
	x1 := suite.Scalar().Pick(rand) // create a private key x
	x2 := suite.Scalar().Pick(rand)
	X[mine1] = suite.Point().Mul(x1, nil) // corresponding public key X
	X[mine2] = suite.Point().Mul(x2, nil)

	// Generate two signatures using x1 and two using x2
	M := []byte("Hello World!")     // message we want to sign
	S := []byte("My Linkage Scope") // scope for linkage tags
	var sig [4][]byte
	sig[0] = Sign(suite, rand, M, Set(X), S, mine1, x1)
	sig[1] = Sign(suite, rand, M, Set(X), S, mine1, x1)
	sig[2] = Sign(suite, rand, M, Set(X), S, mine2, x2)
	sig[3] = Sign(suite, rand, M, Set(X), S, mine2, x2)
	for i := range sig {
		fmt.Printf("Signature %d:\n%s", i, hex.Dump(sig[i]))
	}

	// Verify the signatures against the correct message
	var tag [4][]byte
	for i := range sig {
		goodtag, err := Verify(suite, M, Set(X), S, sig[i])
		if err != nil {
			panic(err.Error())
		}
		tag[i] = goodtag
		if tag[i] == nil || len(tag[i]) != suite.PointLen() {
			panic("Verify returned invalid tag")
		}
		fmt.Printf("Sig%d tag: %s\n", i,
			hex.EncodeToString(tag[i]))

		// Verify the signature against the wrong message
		BAD := []byte("Goodbye world!")
		badtag, err := Verify(suite, BAD, Set(X), S, sig[i])
		if err == nil || badtag != nil {
			panic("Signature verified against wrong message!?")
		}
	}
	if !bytes.Equal(tag[0], tag[1]) || !bytes.Equal(tag[2], tag[3]) ||
		bytes.Equal(tag[0], tag[2]) {
		panic("tags aren't coming out right!")
	}

	// Output:
	// Signature 0:
	// 00000000  63 39 d4 fb ed 5c 93 83  14 1f 97 0d a6 7c 9f 19  |c9...\.......|..|
	// 00000010  d1 ed 1b 66 85 4d 3c 9a  96 b9 bd af 63 50 77 01  |...f.M<.....cPw.|
	// 00000020  35 e7 59 fa 2b 41 20 f8  b6 48 43 62 91 f1 c6 99  |5.Y.+A ..HCb....|
	// 00000030  0e 64 9c 2c 06 fe 84 75  4f ca 03 7f 28 b5 6d 0c  |.d.,...uO...(.m.|
	// 00000040  b2 52 04 d2 ed 23 4a cd  98 5e bc 26 af 05 5f 32  |.R...#J..^.&.._2|
	// 00000050  f4 d8 fa ce b5 b7 84 d0  38 ea 3e 82 5c e4 93 04  |........8.>.\...|
	// 00000060  31 68 8d ca 3f 6a 85 a1  0d f1 cf 9d 21 05 83 f2  |1h..?j......!...|
	// 00000070  35 63 b0 65 a8 50 a5 ee  ec 95 f8 fd 78 de 73 08  |5c.e.P......x.s.|
	// 00000080  7e 2d ef 97 ca b0 fb e5  69 ce 41 59 e0 2d 8c 0b  |~-......i.AY.-..|
	// 00000090  0e 12 b2 9d bb 24 ca 03  d3 98 ea ff 30 3e a6 c9  |.....$......0>..|
	// Signature 1:
	// 00000000  7b cf d3 b3 bd b0 c3 6e  f0 f1 c4 70 35 aa 35 5c  |{......n...p5.5\|
	// 00000010  61 1c 20 a9 11 3a 7d cf  22 ec 97 b4 7d 21 bf 07  |a. ..:}."...}!..|
	// 00000020  67 70 bb 6e d1 b1 c6 16  2c ea b7 59 4f 1d 13 f8  |gp.n....,..YO...|
	// 00000030  87 6f a8 74 f6 a8 f2 35  38 0a 67 e4 a9 26 3e 02  |.o.t...58.g..&>.|
	// 00000040  fa 2d e2 ef c0 67 ff 24  14 aa a5 99 01 ab de 14  |.-...g.$........|
	// 00000050  1b 12 c3 d2 8c 9e 4f e7  b5 f8 b9 49 2f de e2 0b  |......O....I/...|
	// 00000060  fe e8 5c 0c 56 18 63 19  e2 f4 4d 6f b4 5d 1c ea  |..\.V.c...Mo.]..|
	// 00000070  5d 37 8b 13 9b 2c 7f c6  64 21 5e 38 93 27 f4 06  |]7...,..d!^8.'..|
	// 00000080  7e 2d ef 97 ca b0 fb e5  69 ce 41 59 e0 2d 8c 0b  |~-......i.AY.-..|
	// 00000090  0e 12 b2 9d bb 24 ca 03  d3 98 ea ff 30 3e a6 c9  |.....$......0>..|
	// Signature 2:
	// 00000000  37 26 60 71 58 f7 87 ec  c3 fa aa 36 e8 04 fe cf  |7&`qX......6....|
	// 00000010  f5 3f f9 34 0d 6f 2a 5c  4b 28 43 dd 31 8a 72 02  |.?.4.o*\K(C.1.r.|
	// 00000020  58 c9 50 76 f9 f8 e5 7b  54 fc dd 89 5c 64 54 7c  |X.Pv...{T...\dT||
	// 00000030  52 21 d9 30 0d b5 9b 13  3d 4b 5e d4 c4 fe f5 06  |R!.0....=K^.....|
	// 00000040  1e 91 e3 7b 4b 6a 9d f8  82 d3 42 19 1a bf 94 80  |...{Kj....B.....|
	// 00000050  33 92 bd 73 47 09 71 38  0f 06 23 d7 9e 8e 96 0b  |3..sG.q8..#.....|
	// 00000060  b3 e7 76 d6 ed 60 37 a7  98 38 87 9d 59 bc 7b 82  |..v..`7..8..Y.{.|
	// 00000070  f7 c5 79 f9 1d aa c3 17  5a 13 95 59 de 44 95 02  |..y.....Z..Y.D..|
	// 00000080  30 54 cf 20 49 73 89 56  22 e5 e3 1f b8 27 e2 72  |0T. Is.V"....'.r|
	// 00000090  06 c7 38 48 98 ee 03 1d  f4 7b c7 7e 2c d8 a7 0a  |..8H.....{.~,...|
	// Signature 3:
	// 00000000  ce 35 b2 a9 19 c9 e7 92  25 4a 9b ae c0 c7 85 19  |.5......%J......|
	// 00000010  e6 04 1a 72 42 e3 c7 85  1b b6 a1 df d7 bd 88 0b  |...rB...........|
	// 00000020  45 a7 41 99 c3 ef 1f db  80 40 47 1a 19 b1 57 cd  |E.A......@G...W.|
	// 00000030  19 df c9 a2 db 38 bb 14  b6 1d 64 3f 3e e2 36 03  |.....8....d?>.6.|
	// 00000040  55 66 b1 9c a7 5b ca 61  ba c8 c6 5c 9e 04 80 85  |Uf...[.a...\....|
	// 00000050  e4 64 7f 81 e7 38 6d 97  92 83 65 02 e7 a4 81 05  |.d...8m...e.....|
	// 00000060  f9 aa 40 fd 37 04 ab b7  e3 5d 84 79 4f 45 3b 1f  |..@.7....].yOE;.|
	// 00000070  2b 8a d3 74 9b 91 8c 9a  8d dd f4 4a 44 a0 ea 08  |+..t.......JD...|
	// 00000080  30 54 cf 20 49 73 89 56  22 e5 e3 1f b8 27 e2 72  |0T. Is.V"....'.r|
	// 00000090  06 c7 38 48 98 ee 03 1d  f4 7b c7 7e 2c d8 a7 0a  |..8H.....{.~,...|
	// Sig0 tag: 7e2def97cab0fbe569ce4159e02d8c0b0e12b29dbb24ca03d398eaff303ea6c9
	// Sig1 tag: 7e2def97cab0fbe569ce4159e02d8c0b0e12b29dbb24ca03d398eaff303ea6c9
	// Sig2 tag: 3054cf204973895622e5e31fb827e27206c7384898ee031df47bc77e2cd8a70a
	// Sig3 tag: 3054cf204973895622e5e31fb827e27206c7384898ee031df47bc77e2cd8a70a

}

var benchMessage = []byte("Hello World!")

var benchPubOpenSSL, benchPriOpenSSL = benchGenKeysOpenSSL(100)
var benchSig1OpenSSL = benchGenSigOpenSSL(1)
var benchSig10OpenSSL = benchGenSigOpenSSL(10)
var benchSig100OpenSSL = benchGenSigOpenSSL(100)

var benchPubEd25519, benchPriEd25519 = benchGenKeysEd25519(100)
var benchSig1Ed25519 = benchGenSigEd25519(1)
var benchSig10Ed25519 = benchGenSigEd25519(10)
var benchSig100Ed25519 = benchGenSigEd25519(100)

func benchGenKeys(g kyber.Group,
	nkeys int) ([]kyber.Point, kyber.Scalar) {

	rand := random.Stream

	// Create an anonymity set of random "public keys"
	X := make([]kyber.Point, nkeys)
	for i := range X { // pick random points
		X[i] = g.Point().Pick(rand)
	}

	// Make just one of them an actual public/private keypair (X[mine],x)
	x := g.Scalar().Pick(rand)
	X[0] = g.Point().Mul(x, nil)

	return X, x
}

func benchGenKeysOpenSSL(nkeys int) ([]kyber.Point, kyber.Scalar) {
	return benchGenKeys(edwards25519.NewAES128SHA256Ed25519(), nkeys)
}
func benchGenSigOpenSSL(nkeys int) []byte {
	suite := edwards25519.NewAES128SHA256Ed25519()
	rand := suite.Cipher([]byte("example"))
	return Sign(suite, rand, benchMessage,
		Set(benchPubOpenSSL[:nkeys]), nil,
		0, benchPriOpenSSL)
}

func benchGenKeysEd25519(nkeys int) ([]kyber.Point, kyber.Scalar) {
	return benchGenKeys(edwards25519.NewAES128SHA256Ed25519(), nkeys)
}
func benchGenSigEd25519(nkeys int) []byte {
	suite := edwards25519.NewAES128SHA256Ed25519()
	rand := suite.Cipher([]byte("example"))
	return Sign(suite, rand, benchMessage,
		Set(benchPubEd25519[:nkeys]), nil,
		0, benchPriEd25519)
}

func benchSign(suite Suite, pub []kyber.Point, pri kyber.Scalar,
	niter int) {
	rand := suite.Cipher([]byte("example"))
	for i := 0; i < niter; i++ {
		Sign(suite, rand, benchMessage, Set(pub), nil, 0, pri)
	}
}

func benchVerify(suite Suite, pub []kyber.Point,
	sig []byte, niter int) {
	for i := 0; i < niter; i++ {
		tag, err := Verify(suite, benchMessage, Set(pub), nil, sig)
		if tag == nil || err != nil {
			panic("benchVerify failed")
		}
	}
}

func BenchmarkSign1OpenSSL(b *testing.B) {
	benchSign(edwards25519.NewAES128SHA256Ed25519(),
		benchPubOpenSSL[:1], benchPriOpenSSL, b.N)
}
func BenchmarkSign10OpenSSL(b *testing.B) {
	benchSign(edwards25519.NewAES128SHA256Ed25519(),
		benchPubOpenSSL[:10], benchPriOpenSSL, b.N)
}
func BenchmarkSign100OpenSSL(b *testing.B) {
	benchSign(edwards25519.NewAES128SHA256Ed25519(),
		benchPubOpenSSL[:100], benchPriOpenSSL, b.N)
}

func BenchmarkVerify1OpenSSL(b *testing.B) {
	benchVerify(edwards25519.NewAES128SHA256Ed25519(),
		benchPubOpenSSL[:1], benchSig1OpenSSL, b.N)
}
func BenchmarkVerify10OpenSSL(b *testing.B) {
	benchVerify(edwards25519.NewAES128SHA256Ed25519(),
		benchPubOpenSSL[:10], benchSig10OpenSSL, b.N)
}
func BenchmarkVerify100OpenSSL(b *testing.B) {
	benchVerify(edwards25519.NewAES128SHA256Ed25519(),
		benchPubOpenSSL[:100], benchSig100OpenSSL, b.N)
}

func BenchmarkSign1Ed25519(b *testing.B) {
	benchSign(edwards25519.NewAES128SHA256Ed25519(),
		benchPubEd25519[:1], benchPriEd25519, b.N)
}
func BenchmarkSign10Ed25519(b *testing.B) {
	benchSign(edwards25519.NewAES128SHA256Ed25519(),
		benchPubEd25519[:10], benchPriEd25519, b.N)
}
func BenchmarkSign100Ed25519(b *testing.B) {
	benchSign(edwards25519.NewAES128SHA256Ed25519(),
		benchPubEd25519[:100], benchPriEd25519, b.N)
}

func BenchmarkVerify1Ed25519(b *testing.B) {
	benchVerify(edwards25519.NewAES128SHA256Ed25519(),
		benchPubEd25519[:1], benchSig1Ed25519, b.N)
}
func BenchmarkVerify10Ed25519(b *testing.B) {
	benchVerify(edwards25519.NewAES128SHA256Ed25519(),
		benchPubEd25519[:10], benchSig10Ed25519, b.N)
}
func BenchmarkVerify100Ed25519(b *testing.B) {
	benchVerify(edwards25519.NewAES128SHA256Ed25519(),
		benchPubEd25519[:100], benchSig100Ed25519, b.N)
}
