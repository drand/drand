package anon

import (
	"bytes"
	"fmt"
	//"testing"
	"encoding/hex"

	"gopkg.in/dedis/kyber.v1"
	"gopkg.in/dedis/kyber.v1/group/edwards25519"
)

func Example_encrypt1() {

	// Crypto setup
	suite := edwards25519.NewAES128SHA256Ed25519()
	rand := suite.Cipher([]byte("example"))

	// Create a public/private keypair (X[mine],x)
	X := make([]kyber.Point, 1)
	mine := 0                           // which public key is mine
	x := suite.Scalar().Pick(rand)      // create a private key x
	X[mine] = suite.Point().Mul(x, nil) // corresponding public key X

	// Encrypt a message with the public key
	M := []byte("Hello World!") // message to encrypt
	C := Encrypt(suite, rand, M, Set(X), false)
	fmt.Printf("Encryption of '%s':\n%s", string(M), hex.Dump(C))

	// Decrypt the ciphertext with the private key
	MM, err := Decrypt(suite, C, Set(X), mine, x, false)
	if err != nil {
		panic(err.Error())
	}
	if !bytes.Equal(M, MM) {
		panic("Decryption failed to reproduce message")
	}
	fmt.Printf("Decrypted: '%s'\n", string(MM))

	// Encryption of 'Hello World!':
	// 00000000  f9 d1 d4 75 7b 0d 76 68  95 08 71 74 9a 87 5f 1e  |...u{.vh..qt.._.|
	// 00000010  0a 22 23 34 bd d1 5b f6  e7 21 3c f3 c9 92 6f bd  |."#4..[..!<...o.|
	// 00000020  b9 87 fd 9c 32 29 43 56  e8 32 59 52 19 1e c0 2b  |....2)CV.2YR...+|
	// 00000030  24 29 31 ff c6 ce ac b9  2f b1 78 14 e9 86 b5 b1  |$)1...../.x.....|
	// 00000040  bf ac 82 f9 d0 c1 98 83  0c a2 af a7 93 8d 6d 00  |..............m.|
	// 00000050  91 eb 5f 48 0d 2b a5 e9  c2 be d6 3c              |.._H.+.....<|
	// Decrypted: 'Hello World!'

}

func ExampleEncrypt_anonSet() {

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

	// Encrypt a message with all the public keys
	M := []byte("Hello World!") // message to encrypt
	C := Encrypt(suite, rand, M, Set(X), false)
	fmt.Printf("Encryption of '%s':\n%s", string(M), hex.Dump(C))

	// Decrypt the ciphertext with the known private key
	MM, err := Decrypt(suite, C, Set(X), mine, x, false)
	if err != nil {
		panic(err.Error())
	}
	if !bytes.Equal(M, MM) {
		panic("Decryption failed to reproduce message")
	}
	fmt.Printf("Decrypted: '%s'\n", string(MM))

	// Encryption of 'Hello World!':
	// 00000000  3c 2e 26 55 5e 9c 59 55  68 91 5c 68 19 e3 10 6a  |<.&U^.YUh.\h...j|
	// 00000010  be 1d 8c fc 52 b1 85 98  31 a9 81 08 24 bb f0 d0  |....R...1...$...|
	// 00000020  db 47 f9 b3 ee 3b 14 f2  2d 8f 0a c9 83 9d 47 1a  |.G...;..-.....G.|
	// 00000030  69 0f a4 b2 5b 44 c8 a0  ca 33 1e c6 04 9d 98 35  |i...[D...3.....5|
	// 00000040  31 cd 3a a9 0b 44 64 d5  a5 54 d4 5d 33 67 9e 2e  |1.:..Dd..T.]3g..|
	// 00000050  35 e8 05 f3 17 c9 a5 14  9f 5b b6 9a c3 ee 57 54  |5........[....WT|
	// 00000060  64 2f c2 06 36 ae aa af  f8 61 9d c3 cd 09 c2 d7  |d/..6....a......|
	// 00000070  74 8d 32 bf 08 cb ef 1d  06 af 35 52 99 1f b1 16  |t.2.......5R....|
	// 00000080  a7 3c 1b 02 8a 5f bd eb  f0 28 94 df 36 44 07 be  |.<..._...(..6D..|
	// 00000090  22 01 7c dc ad 06 09 7a  62 8e 45 98              |".|....zb.E.|
	// Decrypted: 'Hello World!'
}
