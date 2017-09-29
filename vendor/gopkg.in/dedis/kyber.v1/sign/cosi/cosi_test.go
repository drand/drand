package cosi

import (
	"fmt"
	"testing"

	xEd25519 "github.com/bford/golang-x-crypto/ed25519"
	"github.com/bford/golang-x-crypto/ed25519/cosi"
	"github.com/stretchr/testify/assert"
	"gopkg.in/dedis/kyber.v1"
	"gopkg.in/dedis/kyber.v1/group/edwards25519"
	"gopkg.in/dedis/kyber.v1/util/key"
)

var testSuite = edwards25519.NewAES128SHA256Ed25519()

// TestCosiCommitment test if the commitment generation is correct
func TestCosiCommitment(t *testing.T) {
	var length = 5
	cosis := genCosis(length)
	// gen commitments from children
	commitments := genCommitments(cosis[1:])
	root := cosis[0]
	root.Commit(nil, commitments)
	// compute the aggregate commitment ourself...
	aggCommit := testSuite.Point().Null()
	// add commitment of children
	for _, com := range commitments {
		aggCommit = aggCommit.Add(aggCommit, com)
	}
	// add commitment of root
	aggCommit = aggCommit.Add(aggCommit, root.commitment)
	if !aggCommit.Equal(root.aggregateCommitment) {
		t.Fatal("Aggregate Commitment are not equal")
	}
}

func TestCosiChallenge(t *testing.T) {
	cosis := genCosis(5)
	genPostCommitmentPhaseCosi(cosis)
	root, children := cosis[0], cosis[1:]
	msg := []byte("Hello World Cosi\n")
	chal, err := root.CreateChallenge(msg)
	if err != nil {
		t.Fatal("Error during challenge generation")
	}
	for _, child := range children {
		child.Challenge(chal)
		if !child.challenge.Equal(chal) {
			t.Fatal("Error during challenge on children")
		}
	}
}

// TestCosiResponse will test wether the response generation is correct or not
func TestCosiResponse(t *testing.T) {
	msg := []byte("Hello World Cosi")
	// go to the challenge phase
	cosis := genCosis(5)
	genPostChallengePhaseCosi(cosis, msg)
	root, children := cosis[0], cosis[1:]
	var responses []kyber.Scalar

	// for verification later
	aggResponse := testSuite.Scalar().Zero()
	for _, ch := range children {
		// generate the response of each children
		r, err := ch.CreateResponse()
		if err != nil {
			t.Fatal("Error creating response:", err)
		}
		responses = append(responses, r)
		aggResponse = aggResponse.Add(aggResponse, r)
	}
	// pass them up to the root
	_, err := root.Response(responses)
	if err != nil {
		t.Fatal("Response phase failed:", err)
	}

	// verify it
	aggResponse = aggResponse.Add(aggResponse, root.response)
	if !aggResponse.Equal(root.aggregateResponse) {
		t.Fatal("Responses aggregated not equal")
	}
}

// TestCosiSignature test if the signature generation is correct,i.e. if we
// can verify the final signature.
func TestCosiSignature(t *testing.T) {
	msg := []byte("Hello World Cosi")
	nb := 2
	cosis := genCosis(nb)
	genFinalCosi(cosis, msg)
	root, children := cosis[0], cosis[1:]
	var publics []kyber.Point
	// add root public key
	rootPublic := testSuite.Point().Mul(root.private, nil)
	publics = append(publics, rootPublic)
	for _, ch := range children {
		// add children public key
		public := testSuite.Point().Mul(ch.private, nil)
		publics = append(publics, public)
	}
	sig := root.Signature()

	if err := VerifySignature(testSuite, publics, msg, sig); err != nil {
		t.Fatal("Error veriying:", err)
	}
	var Ed25519Publics []xEd25519.PublicKey
	for _, p := range publics {
		buff, err := p.MarshalBinary()
		assert.Nil(t, err)
		Ed25519Publics = append(Ed25519Publics, xEd25519.PublicKey(buff))
	}

	if !cosi.Verify(Ed25519Publics, nil, msg, sig) {
		t.Error("github.com/bforg/golang-x-crypto/ed25519/cosi fork can't verify")
	}
}

func TestCosiSignatureWithMask(t *testing.T) {
	msg := []byte("Hello World Cosi")
	nb := 5
	fail := 2
	cosis, publics := genCosisFailing(nb, fail)
	genFinalCosi(cosis, msg)
	root := cosis[0]
	sig := root.Signature()

	if err := VerifySignature(testSuite, publics, msg, sig); err != nil {
		t.Fatal("Error veriying:", err)
	}

	var Ed25519Publics []xEd25519.PublicKey
	for _, p := range publics {
		buff, err := p.MarshalBinary()
		assert.Nil(t, err)
		Ed25519Publics = append(Ed25519Publics, xEd25519.PublicKey(buff))
	}

	if !cosi.Verify(Ed25519Publics, cosi.ThresholdPolicy(3), msg, sig) {
		t.Error("github.com/bforg/golang-x-crypto/ed25519/cosi fork can't verify")
	}
	if cosi.Verify(Ed25519Publics, cosi.ThresholdPolicy(4), msg, sig) {
		t.Error("github.com/bforg/golang-x-crypto/ed25519/cosi fork can't verify")
	}

}

func genKeyPair(nb int) ([]*key.Pair, []kyber.Point) {
	var kps []*key.Pair
	var publics []kyber.Point
	for i := 0; i < nb; i++ {
		kp := key.NewKeyPair(testSuite)
		kps = append(kps, kp)
		publics = append(publics, kp.Public)
	}
	return kps, publics
}

func genCosis(nb int) []*CoSi {
	kps, publics := genKeyPair(nb)
	var cosis []*CoSi
	for _, kp := range kps {
		cosis = append(cosis, NewCosi(testSuite, kp.Secret, publics))
	}
	return cosis
}

func genCosisFailing(nb int, failing int) (cosis []*CoSi, allPublics []kyber.Point) {
	kps, publics := genKeyPair(nb)
	allPublics = publics
	for i := 0; i < nb-failing; i++ {
		cosis = append(cosis, NewCosi(testSuite, kps[i].Secret, allPublics))
	}
	for i := range cosis {
		for j := nb - failing; j < nb; j++ {
			cosis[i].SetMaskBit(j, false)
		}
	}
	return
}

func genCommitments(cosis []*CoSi) []kyber.Point {
	commitments := make([]kyber.Point, len(cosis))
	for i := range cosis {
		commitments[i] = cosis[i].CreateCommitment(nil)
	}
	return commitments
}

// genPostCommitmentPhaseCosi returns the Root and its Children Cosi. They have
// already made the Commitment phase.
func genPostCommitmentPhaseCosi(cosis []*CoSi) {
	commitments := genCommitments(cosis[1:])
	root := cosis[0]
	root.Commit(nil, commitments)
}

func genPostChallengePhaseCosi(cosis []*CoSi, msg []byte) {
	genPostCommitmentPhaseCosi(cosis)
	chal, _ := cosis[0].CreateChallenge(msg)
	for _, ch := range cosis[1:] {
		ch.Challenge(chal)
	}
}

func genFinalCosi(cosis []*CoSi, msg []byte) error {
	genPostChallengePhaseCosi(cosis, msg)
	children := cosis[1:]
	root := cosis[0]
	// go to the challenge phase
	var responses []kyber.Scalar
	for _, ch := range children {
		resp, err := ch.CreateResponse()
		if err != nil {
			panic("Aie")
		}
		responses = append(responses, resp)
	}
	// pass them up to the root
	_, err := root.Response(responses)
	if err != nil {
		return fmt.Errorf("Response phase failed:%v", err)
	}
	return nil
}
