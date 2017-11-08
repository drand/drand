// +build vartime

package nist

import (
	"crypto/cipher"
	"crypto/sha256"
	"hash"
	"io"
	"reflect"

	"github.com/dedis/fixbuf"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/cipher/sha3"
	"github.com/dedis/kyber/group/internal/marshalling"
	"github.com/dedis/kyber/util/random"
)

type Suite128 struct {
	p256
}

// SHA256 hash function
func (s *Suite128) Hash() hash.Hash {
	return sha256.New()
}

// SHA3/SHAKE128 Sponge Cipher
func (s *Suite128) Cipher(key []byte, options ...interface{}) kyber.Cipher {
	return sha3.NewShakeCipher128(key, options...)
}

func (s *Suite128) Read(r io.Reader, objs ...interface{}) error {
	return fixbuf.Read(r, s, objs)
}

func (s *Suite128) Write(w io.Writer, objs ...interface{}) error {
	return fixbuf.Write(w, objs)
}

func (s *Suite128) New(t reflect.Type) interface{} {
	return marshalling.GroupNew(s, t)
}

func (s *Suite128) NewKey(rand cipher.Stream) kyber.Scalar {
	if rand == nil {
		rand = random.Stream
	}
	return s.Scalar().Pick(rand)
}

// Ciphersuite based on AES-128, SHA-256, and the NIST P-256 elliptic curve.
func NewAES128SHA256P256() *Suite128 {
	suite := new(Suite128)
	suite.p256.Init()
	return suite
}
