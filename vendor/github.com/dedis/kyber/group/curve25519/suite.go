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

	"github.com/dedis/kyber/group/internal/marshalling"
	"github.com/dedis/kyber/util/random"
	"github.com/dedis/kyber/xof/blake2xb"
)

type SuiteCurve25519 struct {
	ProjectiveCurve
}

// SHA256 hash function
func (s *SuiteCurve25519) Hash() hash.Hash {
	return sha256.New()
}

func (s *SuiteCurve25519) XOF(seed []byte) kyber.XOF {
	return blake2xb.New(seed)
}

func (s *SuiteCurve25519) Read(r io.Reader, objs ...interface{}) error {
	return fixbuf.Read(r, s, objs)
}

func (s *SuiteCurve25519) Write(w io.Writer, objs ...interface{}) error {
	return fixbuf.Write(w, objs)
}

func (s *SuiteCurve25519) New(t reflect.Type) interface{} {
	return marshalling.GroupNew(s, t)
}

func (s *SuiteCurve25519) RandomStream() cipher.Stream {
	return random.New()
}

// NewBlakeSHA256Curve25519 returns a cipher suite based on package
// github.com/dedis/kyber/xof/blake2xb, SHA-256, and Curve25519.
//
// If fullGroup is false, then the group is the prime-order subgroup.
//
// The scalars created by this group implement kyber.Scalar's SetBytes
// method, interpreting the bytes as a big-endian integer, so as to be
// compatible with the Go standard library's big.Int type.
func NewBlakeSHA256Curve25519(fullGroup bool) *SuiteCurve25519 {
	suite := new(SuiteCurve25519)
	suite.Init(Param25519(), fullGroup)
	return suite
}
