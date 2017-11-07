// +build experimental

package poly

import (
	"errors"
	"fmt"
	"hash"

	"github.com/dedis/kyber/abstract"
)

// This file describes the Distributed Threshold Schnorr Signature

// Schnorr holds the data necessary to complete a distributed schnorr signature
// and will implement the necessary methods.
// You can setup a schnorr struct with a LongTerm shared secret
// and when you want to sign something, you will have to:
//  - Start a new round specifying the random shared secret chosen and the message to sign
//  - Generate the partial signature of the current node
//  - Collect every others partial signature
//  - Generate the signature
//  - Do whatever you want to do with
//  - Start a new round with the same schnorr struct
// If you want to verify a given signature, use
// schnorr.VerifySignature(SchnorrSig, msg)
// CAREFUL: your schnorr signature is a LONG TERM signature, you must keep the same during
// all rounds, else you won't be able to verify any signatures. The following have to stay
// the same:
//  - LongTerm sharedSecret
//  - PolyInfo
// If you know these are the same throughout differents rounds, you can create many schnorr structs. This is
// definitly NOT the way it is intented to be used, so use it at your own risks.
type Schnorr struct {

	// The info describing which kind of polynomials we using, on which groups etc
	info Threshold

	// the suite used
	suite abstract.Suite

	// The long-term shared secret evaluated by receivers
	longterm *SharedSecret

	////////////////////////////////////////////////////
	// For each round, we have the following members :

	// hash is the hash of the message
	hash *abstract.Scalar

	// The short term shared secret, only to be used for this signature,
	// i.e. the random secret in the regular schnorr signature
	random *SharedSecret

	// The partial signatures of each other peer (i.e. receiver)
	partials []*SchnorrPartialSig

	/////////////////////////////////////////////////////
}

// Partial Schnorr Sig represents the partial signatures that each peer must generate in order to
// create the "global" signature. This struct must be sent across each peer for each peer
type SchnorrPartialSig struct {
	// The index of this partial signature regarding the global one
	// same as the "receiver" index in the joint.go code
	Index int

	// The partial signature itself
	Part *abstract.Scalar
}

// SchnorrSig represents the final signature of a distribtued threshold schnorr signature
// which can be verified against a message
// This struct is not intended to be constructed manually but can be:
//  - produced by the Schnorr struct
//  - verified against a Schnorr struct
type SchnorrSig struct {

	// the signature itself
	Signature *abstract.Scalar

	// the random public polynomial used during the signature generation
	Random *PubPoly
}

// Instantiates a Schnorr struct. A wrapper around Init
func NewSchnorr(suite abstract.Suite, info Threshold, longterm *SharedSecret) *Schnorr {
	return new(Schnorr).Init(suite, info, longterm)
}

// Initializes the Schnorr struct
func (s *Schnorr) Init(suite abstract.Suite, info Threshold, longterm *SharedSecret) *Schnorr {
	s.suite = suite
	s.info = info
	s.longterm = longterm
	return s
}

// Sets the random key for the d.schnorr algo + sets the msg to be signed.
// You call this function when you want a new signature to be issued on a specific message.
// The security of the distributed schnorr signature protocol is the same as for the regular :
// The random secret "must be fresh* for "each* signature / signed message (hence the 'NewRound')
func (s *Schnorr) NewRound(random *SharedSecret, h hash.Hash) error {
	s.random = random
	s.hash = nil
	s.partials = make([]*SchnorrPartialSig, s.info.N)
	hash, err := s.hashMessage(h.Sum(nil), s.random.Pub.SecretCommit())
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to hash the message with the given shared secret : %v", err))
	}
	s.hash = &hash

	return nil
}

// Returns a hash of the message and the random secret:
// H( m || V )
// Returns an error if something went wrong with the marshalling
func (s *Schnorr) hashMessage(msg []byte, v abstract.Point) (abstract.Scalar, error) {
	vb, err := v.MarshalBinary()
	if err != nil {
		return nil, err
	}
	c := s.suite.Cipher(vb)
	c.Message(nil, nil, msg)
	return s.suite.Scalar().Pick(c), nil
}

// Verifies if the received structures are good and
// tests the partials shares if there are some
func (s *Schnorr) verify() error {
	if s.longterm.Index != s.random.Index {
		return errors.New("The index for the longterm shared secret and the random secret differs for this peer.")
	}
	nsig := 0
	for i, _ := range s.partials {
		if s.partials[i] != nil {
			nsig += 1
		}
	}
	if nsig < s.info.T {
		return errors.New(fmt.Sprintf("Received to few Partial Signatures (%d vs %d) to complete a global schnorr signature", len(s.partials), s.info.T))
	}
	return nil
}

// Verifies if a given partial signature can be checked against the longterm and random secrets
// of the schnorr structures.
func (s *Schnorr) verifyPartialSig(ps *SchnorrPartialSig) error {
	// compute the left part of the equation
	left := s.suite.Point().Mul(s.suite.Point().Base(), *ps.Part)
	// compute the right part of the equation
	right := s.suite.Point().Add(s.random.Pub.Eval(ps.Index), s.suite.Point().Mul(s.longterm.Pub.Eval(ps.Index), *s.hash))
	if !left.Equal(right) {
		return errors.New(fmt.Sprintf("Partial Signature of peer %d could not be validated.", ps.Index))
	}
	return nil
}

// Returns the index of the peer holding this schnorr struct
// the index of its share in the polynomials used
func (s *Schnorr) index() int {
	return s.longterm.Index
}

// Reveals the partial signature for this peer
// Si = Ri + H(m || V) * Pi
// with :
//  - Ri = share of the random secret for peer i
//  - V  = public commitment of the random secret (i.e. Public random poly evaluated at point 0 )
//  - Pi = share of the longterm secret for peer i
// This signature is to be sent to each other peer
func (s *Schnorr) RevealPartialSig() *SchnorrPartialSig {
	hash := s.suite.Scalar().Set(*s.hash)
	sigma := s.suite.Scalar().Zero()
	sigma = sigma.Add(sigma, *s.random.Share)
	// H(m||v) * Pi
	hashed := s.suite.Scalar().Mul(hash, *s.longterm.Share)
	// Ri + H(m||V) * Pi
	sigma = sigma.Add(sigma, hashed)
	psc := &SchnorrPartialSig{
		Index: s.index(),
		Part:  &sigma,
	}

	return psc
}

// Receives a signature from other peers,
// adds it to its list of partial signatures and verifies it
// It returns an error if
// - it can not validate this given partial signature
//   against the longterm and random shared secret
// - there is already a partial signature added for this index
// NOTE : let s = RevealPartialSig(), s is NOT added automatically to the
// set of partial signature, for now you have to do it yourself by calling
// AddPartialSig(s)
func (s *Schnorr) AddPartialSig(ps *SchnorrPartialSig) error {
	if ps.Index >= s.info.N {
		return errors.New(fmt.Sprintf("Cannot add signature with index %d whereas schnorr could have max %s partial signatures", ps.Index, s.info.N))
	}
	if s.partials[ps.Index] != nil {
		return errors.New(fmt.Sprintf("A Partial Signature has already been added for this index %d", ps.Index))
	}
	if err := s.verifyPartialSig(ps); err != nil {
		return errors.New(fmt.Sprintf("Partial signature to add is not valid : %v", err))
	}
	s.partials[ps.Index] = ps
	return nil
}

// Generates the global schnorr signature
// by reconstructing the secret contained in the partial responses
func (s *Schnorr) Sig() (*SchnorrSig, error) {
	// automatic verification
	// TODO : change this into a bool flag or public method ?
	if err := s.verify(); err != nil {
		return nil, err
	}

	pri := PriShares{}
	pri.Empty(s.suite, s.info.T, s.info.N)
	for i, ps := range s.partials {
		// Skip the partials we did not receive
		if ps == nil {
			continue
		}
		pri.SetShare(ps.Index, *s.partials[i].Part)
	}

	// lagrange interpolation to compute the gamma
	gamma := pri.Secret()

	sig := &SchnorrSig{
		Random:    s.random.Pub,
		Signature: &gamma,
	}

	return sig, nil
}

// Verifies if a given signature is correct regarding the message.
// NOTE: This belongs to the schnorr structs however it can be called at any time you want.
// This check is static, meaning it only needs the longterm shared secret, and the signature to
// check. Think of the schnorr signature as a black box having two inputs:
//  - a message to be signed + a random secret ==> NewRound
//  - a message + a signature to check on ==> VerifySchnorrSig
func (s *Schnorr) VerifySchnorrSig(sig *SchnorrSig, h hash.Hash) error {
	// gamma * G
	left := s.suite.Point().Mul(s.suite.Point().Base(), *sig.Signature)

	randomCommit := sig.Random.SecretCommit()
	publicCommit := s.longterm.Pub.SecretCommit()
	hash, err := s.hashMessage(h.Sum(nil), randomCommit)
	if err != nil {
		return err
	}

	// RandomSecretCommit + H(...) * LongtermSecretCommit
	right := s.suite.Point().Add(randomCommit, s.suite.Point().Mul(publicCommit, hash))
	if !left.Equal(right) {
		return errors.New("Signature could not have been verified against the message")
	}
	return nil
}

//// DECODING / ENCODING /////

// Use the Schnorr struct to produce already initialized SchnorrSig.
// The PartialSchnorrSig can be serialized directly.

func (pss *SchnorrPartialSig) Equal(pss2 *SchnorrPartialSig) bool {
	return pss.Index == pss2.Index && (*pss.Part).Equal(*pss2.Part)
}

func (s *Schnorr) EmptySchnorrSig() *SchnorrSig {
	return new(SchnorrSig).Init(s.suite, s.info)
}

// Initialises the struct so it can decode itself
func (s *SchnorrSig) Init(suite abstract.Suite, info Threshold) *SchnorrSig {
	s.Random = new(PubPoly).Init(suite, info.T, suite.Point().Base())
	return s
}

// Tests on equality
func (s *SchnorrSig) Equal(s2 *SchnorrSig) bool {
	return s.Random.Equal(s2.Random) && (*s.Signature).Equal(*s2.Signature)
}
