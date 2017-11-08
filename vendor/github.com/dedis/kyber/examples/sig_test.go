package examples

import (
	"bytes"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/group/edwards25519"
)

type Suite interface {
	kyber.Group
	Cipher(key []byte, options ...interface{}) kyber.Cipher
	kyber.Encoding
}

// A basic, verifiable signature
type basicSig struct {
	C kyber.Scalar // challenge
	R kyber.Scalar // response
}

// Returns a secret that depends on on a message and a point
func hashSchnorr(suite Suite, message []byte, p kyber.Point) kyber.Scalar {
	pb, _ := p.MarshalBinary()
	c := suite.Cipher(pb)
	c.Message(nil, nil, message)
	return suite.Scalar().Pick(c)
}

// This simplified implementation of Schnorr Signatures is based on
// crypto/anon/sig.go
// The ring structure is removed and
// The anonimity set is reduced to one public key = no anonimity
func SchnorrSign(suite Suite, random cipher.Stream, message []byte,
	privateKey kyber.Scalar) []byte {

	// Create random secret v and public point commitment T
	v := suite.Scalar().Pick(random)
	T := suite.Point().Mul(v, nil)

	// Create challenge c based on message and T
	c := hashSchnorr(suite, message, T)

	// Compute response r = v - x*c
	r := suite.Scalar()
	r.Mul(privateKey, c).Sub(v, r)

	// Return verifiable signature {c, r}
	// Verifier will be able to compute v = r + x*c
	// And check that hashElgamal for T and the message == c
	buf := bytes.Buffer{}
	sig := basicSig{c, r}
	_ = suite.Write(&buf, &sig)
	return buf.Bytes()
}

func SchnorrVerify(suite Suite, message []byte, publicKey kyber.Point,
	signatureBuffer []byte) error {

	// Decode the signature
	buf := bytes.NewBuffer(signatureBuffer)
	sig := basicSig{}
	if err := suite.Read(buf, &sig); err != nil {
		return err
	}
	r := sig.R
	c := sig.C

	// Compute base**(r + x*c) == T
	var P, T kyber.Point
	P = suite.Point()
	T = suite.Point()
	T.Add(T.Mul(r, nil), P.Mul(c, publicKey))

	// Verify that the hash based on the message and T
	// matches the challange c from the signature
	c = hashSchnorr(suite, message, T)
	if !c.Equal(sig.C) {
		return errors.New("invalid signature")
	}

	return nil
}

// Example of using Schnorr
func Example_schnorr() {
	// Crypto setup
	group := edwards25519.NewAES128SHA256Ed25519()
	rand := group.Cipher([]byte("example"))

	// Create a public/private keypair (X,x)
	x := group.Scalar().Pick(rand) // create a private key x
	X := group.Point().Mul(x, nil) // corresponding public key X

	// Generate the signature
	M := []byte("Hello World!") // message we want to sign
	sig := SchnorrSign(group, rand, M, x)
	fmt.Print("Signature:\n" + hex.Dump(sig))

	// Verify the signature against the correct message
	err := SchnorrVerify(group, M, X, sig)
	if err != nil {
		panic(err.Error())
	}
	fmt.Println("Signature verified against correct message.")

	// Output:
	// Signature:
	// 00000000  d4 64 bd ac 8a 06 d9 71  f4 ae a1 da e1 c5 55 d5  |.d.....q......U.|
	// 00000010  f7 89 50 10 a5 d9 99 52  b0 c4 f2 ba f9 37 67 02  |..P....R.....7g.|
	// 00000020  35 3e 9b ac e6 dd d1 98  f6 19 88 37 4d e3 4f 5c  |5>.........7M.O\|
	// 00000030  36 de a7 bf b9 f0 06 2b  72 6f 81 b7 59 19 c6 00  |6......+ro..Y...|
	// Signature verified against correct message.
}
