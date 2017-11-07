// Since that package does not implement constant time arithmetic operations
// yet, it must be compiled with the "vartime" compilation flag.

// +build vartime

package curve25519

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

type SuiteEd25519 struct {
	ProjectiveCurve
}

// SHA256 hash function
func (s *SuiteEd25519) Hash() hash.Hash {
	return sha256.New()
}

// SHA3/SHAKE128 Sponge Cipher
func (s *SuiteEd25519) Cipher(key []byte, options ...interface{}) kyber.Cipher {
	return sha3.NewShakeCipher128(key, options...)
}

func (s *SuiteEd25519) Read(r io.Reader, objs ...interface{}) error {
	return fixbuf.Read(r, s, objs)
}

func (s *SuiteEd25519) Write(w io.Writer, objs ...interface{}) error {
	return fixbuf.Write(w, objs)
}

func (s *SuiteEd25519) New(t reflect.Type) interface{} {
	return marshalling.GroupNew(s, t)
}

func (s *SuiteEd25519) NewKey(r cipher.Stream) kyber.Scalar {
	if r == nil {
		r = random.Stream
	}
	return s.Scalar().Pick(r)
}

// Ciphersuite based on AES-128, SHA-256, and the Ed25519 curve.
func NewAES128SHA256Ed25519(fullGroup bool) *SuiteEd25519 {
	suite := new(SuiteEd25519)
	suite.Init(Param25519(), fullGroup)
	return suite
}
