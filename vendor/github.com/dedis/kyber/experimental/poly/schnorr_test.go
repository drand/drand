// +build experimental

package poly

import (
	"bytes"
	"fmt"
	"hash"
	"testing"
)

var m string = "Hello World"
var msg hash.Hash

func init() {
	msg = testSuite.Hash()
	msg.Write([]byte(m))
}

func TestHashMessage(t *testing.T) {
	tr := Threshold{T: 4, R: 4, N: 5}
	point := testSuite.Point().Base()
	s1 := new(Schnorr).Init(testSuite, tr, nil)
	s2 := new(Schnorr).Init(testSuite, tr, nil)
	h1, _ := s1.hashMessage(msg.Sum(nil), point)
	h2, _ := s2.hashMessage(msg.Sum(nil), point)
	if !h1.Equal(h2) {
		t.Error("hash message does not produce equal hashes")
	}
}

func TestNewRound(t *testing.T) {
	n := 5
	pl := Threshold{4, n, n}
	schnorrs := generateSchnorrStructs(pl)
	randoms := generateSharedSecrets(pl)
	randoms2 := generateSharedSecrets(pl)
	for i, _ := range schnorrs {
		err := schnorrs[i].NewRound(randoms[i], msg)
		if err != nil {
			t.Error(fmt.Sprintf("NewRound should validate : %v", err))
		}
		err = schnorrs[i].NewRound(randoms2[i], msg)
		if err != nil {
			t.Error(fmt.Sprintf("Second NewRound should validate : %v", err))
		}
		if schnorrs[i].random.Pub.Equal(randoms[i].Pub) {
			t.Error(fmt.Sprintf("Random secret should not be equals"))
		}

	}
}

func TestRevealPartialSig(t *testing.T) {
	n := 6
	pl := Threshold{4, n, n}
	schnorrs := generateSchnorrStructs(pl)
	randoms := generateSharedSecrets(pl)
	for i, _ := range schnorrs {
		err := schnorrs[i].NewRound(randoms[i], msg)
		if err != nil {
			t.Error(fmt.Sprintf("NewRound should validate : %v", err))
		}
		ps := schnorrs[i].RevealPartialSig()
		if ps.Index != schnorrs[i].index() {
			t.Error(fmt.Sprintf("PartialSig does not have the same index as its producer ><"))
		}
		ps2 := schnorrs[i].RevealPartialSig()
		if !(*ps.Part).Equal(*ps2.Part) {
			t.Error(fmt.Sprintf("PartialSig does not return the same partial signature "))
		}
		err = schnorrs[i].verifyPartialSig(ps)
		if err != nil {
			t.Error(fmt.Sprintf("RevealPartialSig's sig could have not been verified : %v", err))
		}
	}
}

func TestAddPartialSig(t *testing.T) {
	n := 6
	pl := Threshold{5, n, n}
	schnorrs := generateSchnorrStructs(pl)
	randoms := generateSharedSecrets(pl)
	for i, _ := range schnorrs {
		err := schnorrs[i].NewRound(randoms[i], msg)
		if err != nil {
			t.Error(fmt.Sprintf("NewRound should validate : %v", err))
		}
	}
	for i, _ := range schnorrs {
		ps := schnorrs[i].RevealPartialSig()
		if ps.Index != schnorrs[i].index() {
			t.Error(fmt.Sprintf("PartialSig does not have the same index as its producer ><"))
		}
		// geive the partial sig to everyone
		for j, _ := range schnorrs {
			if err := schnorrs[j].AddPartialSig(ps); err != nil {
				t.Error(fmt.Sprintf("AddPartialSig should validate (adding partial sig of peer %d to peer %d : %v", ps.Index, schnorrs[j].index(), err))
			}
		}

	}

	// test adding again sig
	ps2 := schnorrs[0].RevealPartialSig()
	err := schnorrs[0].AddPartialSig(ps2)
	if err == nil {
		t.Error(fmt.Sprintf("AddPartialSig 2 times should return an error"))
	}
	// nillify an partial sig then add a wrong one to same index
	schnorrs[0].partials[ps2.Index] = nil
	s := testSuite.Scalar().One()
	ps2.Part = &s
	err = schnorrs[0].AddPartialSig(ps2)
	if err == nil {
		t.Error(fmt.Sprintf("AddPartialSig with wrong sig should return an error"))
	}
}

func TestSchnorrSig(t *testing.T) {
	n := 9
	pl := Threshold{6, n, n}
	schnorrs := generateSchnorrStructs(pl)
	randoms := generateSharedSecrets(pl)
	for i, _ := range schnorrs {
		err := schnorrs[i].NewRound(randoms[i], msg)
		if err != nil {
			t.Error(fmt.Sprintf("NewRound should validate : %v", err))
		}
	}
	for i, _ := range schnorrs {
		ps := schnorrs[i].RevealPartialSig()
		if ps.Index != schnorrs[i].index() {
			t.Error(fmt.Sprintf("PartialSig does not have the same index as its producer ><"))
		}
		// geive the partial sig to everyone
		for j, _ := range schnorrs {
			if err := schnorrs[j].AddPartialSig(ps); err != nil {
				t.Error(fmt.Sprintf("AddPartialSig should validate (adding partial sig of peer %d to peer %d : %v", ps.Index, schnorrs[j].index(), err))
			}
		}
	}
	sig := make([]*SchnorrSig, n)
	for i, _ := range schnorrs {
		s, err := schnorrs[i].Sig()
		if err != nil {
			t.Error(fmt.Sprintf("SchnorrSig should validate : %v", err))
		}
		sig[i] = s
	}
	// test equality of the signature amongst the peers
	for i, _ := range sig {
		for j, _ := range sig[i+1:] {
			if !(*sig[i].Signature).Equal(*sig[j].Signature) {
				t.Error(fmt.Sprintf("SchnorrSig should produce the same signature amongst peer (%d vs %d)", i, j))
			}
			if !(sig[i].Random.Equal(sig[j].Random)) {
				t.Error(fmt.Sprintf("SchnorrSig should produce the same signature (random poly %d != %d", i, j))
			}
		}
	}
}

func TestVerifySchnorrSig(t *testing.T) {
	n := 4
	tt := 4
	pl := Threshold{T: tt,
		R: n,
		N: n,
	}
	schnorrs := generateSchnorrStructs(pl)
	randoms := generateSharedSecrets(pl)
	for i, _ := range schnorrs {
		err := schnorrs[i].NewRound(randoms[i], msg)
		if err != nil {
			t.Error(fmt.Sprintf("NewRound should validate : %v", err))
		}
	}
	for i, _ := range schnorrs {
		ps := schnorrs[i].RevealPartialSig()
		if ps.Index != schnorrs[i].index() {
			t.Error(fmt.Sprintf("PartialSig does not have the same index as its producer ><"))
		}
		// geive the partial sig to everyone
		for j, _ := range schnorrs {
			if err := schnorrs[j].AddPartialSig(ps); err != nil {
				t.Error(fmt.Sprintf("AddPartialSig should validate (adding partial sig of peer %d to peer %d : %v", ps.Index, schnorrs[j].index(), err))
			}
		}
	}
	sig := make([]*SchnorrSig, len(schnorrs))
	for i, _ := range schnorrs {
		s, err := schnorrs[i].Sig()
		if err != nil {
			t.Error(fmt.Sprintf("SchnorrSig should validate : %v", err))
		}
		sig[i] = s
	}
	// Verify the signature amongst each peers
	for i, _ := range schnorrs {
		newMsg := testSuite.Hash()
		newMsg.Write([]byte(m))
		err := schnorrs[i].VerifySchnorrSig(sig[0], newMsg)
		if err != nil {
			t.Error(fmt.Sprintf("VerifySchnorrSig on peer %d should validate the signature : %v", i, err))
		}
	}
}

func TestPartialSchnorrSigMarshalling(t *testing.T) {
	n := 10
	pl := Threshold{7, n, n}
	schnorrs := generateSchnorrStructs(pl)
	randoms := generateSharedSecrets(pl)
	for i, _ := range schnorrs {
		err := schnorrs[i].NewRound(randoms[i], msg)
		if err != nil {
			t.Error(fmt.Sprintf("NewRound should validate : %v", err))
		}
	}
	ps := schnorrs[0].RevealPartialSig()
	b := new(bytes.Buffer)
	err := testSuite.Write(b, ps)
	if err != nil {
		t.Error(fmt.Sprintf("MarshalBinary on PartialSchnorrSig did not work : %v", err))
	}
	buf := b.Bytes()
	bufReader := bytes.NewBuffer(buf)
	ps2 := new(SchnorrPartialSig)
	err = testSuite.Read(bufReader, ps2)
	if err != nil {
		t.Error(fmt.Sprintf("UnmarshalBinary on PartialSchnorrSig did not work : %v", err))
	}
	if !ps.Equal(ps2) {
		t.Error(fmt.Sprintf("Unmarshalled partial sig should be equal to the original"))
	}

}

func TestSchnorrSigMarshalling(t *testing.T) {
	n := 3
	pl := Threshold{2, n, n}
	schnorrs := generateSchnorrStructs(pl)
	randoms := generateSharedSecrets(pl)
	for i, _ := range schnorrs {
		err := schnorrs[i].NewRound(randoms[i], msg)
		if err != nil {
			t.Error(fmt.Sprintf("NewRound should validate : %v", err))
		}
	}
	for i, _ := range schnorrs {
		ps := schnorrs[i].RevealPartialSig()
		if ps.Index != schnorrs[i].index() {
			t.Error(fmt.Sprintf("PartialSig does not have the same index as its producer ><"))
		}
		// geive the partial sig to everyone
		for j, _ := range schnorrs {
			if err := schnorrs[j].AddPartialSig(ps); err != nil {
				t.Error(fmt.Sprintf("AddPartialSig should validate (adding partial sig of peer %d to peer %d : %v", ps.Index, schnorrs[j].index(), err))
			}
		}
	}
	s, err := schnorrs[0].Sig()
	if err != nil {
		t.Error(fmt.Sprintf("SchnorrSig should validate : %v", err))
	}
	b := new(bytes.Buffer)
	err = testSuite.Write(b, s)
	if err != nil {
		t.Error(fmt.Sprintf("SchnorrSig had error while Marshalling %v", err))
	}
	s2 := schnorrs[0].EmptySchnorrSig()
	err = testSuite.Read(bytes.NewBuffer(b.Bytes()), s2)
	if err != nil {
		t.Error(fmt.Sprintf("SchnorrSig Unmarshaling should have been correct : %v", err))
	}

	if !s2.Equal(s) {
		t.Error(fmt.Sprintf("SchnorrSig structs should have been equals"))
	}

}
