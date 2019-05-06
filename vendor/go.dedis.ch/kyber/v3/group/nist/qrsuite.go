package nist

import (
	"crypto/cipher"
	"crypto/sha256"
	"hash"
	"io"
	"math/big"
	"reflect"

	"go.dedis.ch/fixbuf"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/group/internal/marshalling"
	"go.dedis.ch/kyber/v3/util/random"
	"go.dedis.ch/kyber/v3/xof/blake2xb"
)

// QrSuite is a quadratic residue suite
type QrSuite struct {
	ResidueGroup
}

// Hash returns the instance associated with the suite
func (s QrSuite) Hash() hash.Hash {
	return sha256.New()
}

// XOF creates the XOF associated with the suite
func (s QrSuite) XOF(key []byte) kyber.XOF {
	return blake2xb.New(key)
}

// RandomStream returns a cipher.Stream that returns a key stream
// from crypto/rand.
func (s QrSuite) RandomStream() cipher.Stream {
	return random.New()
}

func (s *QrSuite) Read(r io.Reader, objs ...interface{}) error {
	return fixbuf.Read(r, s, objs)
}

func (s *QrSuite) Write(w io.Writer, objs ...interface{}) error {
	return fixbuf.Write(w, objs)
}

// New implements the kyber.encoding interface
func (s *QrSuite) New(t reflect.Type) interface{} {
	return marshalling.GroupNew(s, t)
}

// NewBlakeSHA256QR512 returns a cipher suite based on package
// go.dedis.ch/kyber/v3/xof/blake2xb, SHA-256, and a residue group of
// quadratic residues modulo a 512-bit prime.
//
// This group size should be used only for testing and experimentation.
// 512-bit DSA-style groups are no longer considered secure.
func NewBlakeSHA256QR512() *QrSuite {
	p, _ := new(big.Int).SetString("10198267722357351868598076141027380280417188309231803909918464305012113541414604537422741096561285049775792035177041672305646773132014126091142862443826263", 10)
	q, _ := new(big.Int).SetString("5099133861178675934299038070513690140208594154615901954959232152506056770707302268711370548280642524887896017588520836152823386566007063045571431221913131", 10)
	r := new(big.Int).SetInt64(2)
	g := new(big.Int).SetInt64(4)

	suite := new(QrSuite)
	suite.SetParams(p, q, r, g)
	return suite
}
