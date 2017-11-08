// Package key provide a wrapper to handle conveniently an asymmetric key pair.
package key

import (
	"crypto/cipher"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/util/random"
)

// Suite represents the list of functionalities needed by this package.
type Suite kyber.Group

// Pair represents a public/private keypair
// together with the ciphersuite the key was generated from.
type Pair struct {
	Suite  Suite        // Ciphersuite this keypair is for
	Public kyber.Point  // Public key
	Secret kyber.Scalar // Secret key
	Hiding kyber.Hiding // Hiding type of the public key
}

// NewKeyPair directly creates a secret/public key pair
func NewKeyPair(suite Suite) *Pair {
	kp := new(Pair)
	kp.Gen(suite, random.Stream)
	return kp
}

// NewHidingKeyPair creates a secret/public key pair and makes sure the
// the public key is hiding-encodable under the field keypair.Hiding.
func NewHidingKeyPair(suite Suite) *Pair {
	kp := new(Pair)
	kp.GenHiding(suite, random.Stream)
	return kp
}

// Gen creates a fresh public/private keypair with the given ciphersuite,
// using a given source of cryptographic randomness.
func (p *Pair) Gen(suite Suite, random cipher.Stream) {
	p.Suite = suite
	p.Secret = suite.NewKey(random)
	p.Public = suite.Point().Mul(p.Secret, nil)
}

// GenHiding tries to generate private / public key pair as long as the public
// key is not hiding-encodable.
func (p *Pair) GenHiding(suite Suite, rand cipher.Stream) {
	p.Gen(suite, rand)
	Xh := p.Public.(kyber.Hiding)
	for {
		Xb := Xh.HideEncode(rand) // try to encode as uniform blob
		if Xb != nil {
			p.Hiding = Xh
			return // success
		}
		p.Gen(suite, rand)
	}
}
