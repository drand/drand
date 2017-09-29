package dss

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/dedis/kyber.v1"
	"gopkg.in/dedis/kyber.v1/group/edwards25519"
	"gopkg.in/dedis/kyber.v1/share/rabin/dkg"
	"gopkg.in/dedis/kyber.v1/sign/eddsa"
	"gopkg.in/dedis/kyber.v1/sign/schnorr"
	"gopkg.in/dedis/kyber.v1/util/random"
)

var suite = edwards25519.NewAES128SHA256Ed25519()

var nbParticipants = 7
var t = nbParticipants/2 + 1

var partPubs []kyber.Point
var partSec []kyber.Scalar

var longterms []*dkg.DistKeyShare
var randoms []*dkg.DistKeyShare

var dss []*DSS

func init() {
	partPubs = make([]kyber.Point, nbParticipants)
	partSec = make([]kyber.Scalar, nbParticipants)
	for i := 0; i < nbParticipants; i++ {
		sec, pub := genPair()
		partPubs[i] = pub
		partSec[i] = sec
	}
	longterms = genDistSecret()
	randoms = genDistSecret()
}

func TestDSSNew(t *testing.T) {
	dss, err := NewDSS(suite, partSec[0], partPubs, longterms[0], randoms[0], []byte("hello"), 4)
	assert.NotNil(t, dss)
	assert.Nil(t, err)

	dss, err = NewDSS(suite, suite.Scalar().Zero(), partPubs, longterms[0], randoms[0], []byte("hello"), 4)
	assert.Nil(t, dss)
	assert.Error(t, err)
}

func TestDSSPartialSigs(t *testing.T) {
	dss0 := getDSS(0)
	dss1 := getDSS(1)
	ps0, err := dss0.PartialSig()
	assert.Nil(t, err)
	assert.NotNil(t, ps0)
	assert.Len(t, dss0.partials, 1)
	// second time should not affect list
	ps0, err = dss0.PartialSig()
	assert.Nil(t, err)
	assert.NotNil(t, ps0)
	assert.Len(t, dss0.partials, 1)

	// wrong index
	goodI := ps0.Partial.I
	ps0.Partial.I = 100
	assert.Error(t, dss1.ProcessPartialSig(ps0))
	ps0.Partial.I = goodI

	// wrong Signature
	goodSig := ps0.Signature
	ps0.Signature = randomBytes(len(ps0.Signature))
	assert.Error(t, dss1.ProcessPartialSig(ps0))
	ps0.Signature = goodSig

	// invalid partial sig
	goodV := ps0.Partial.V
	ps0.Partial.V = suite.Scalar().Zero()
	ps0.Signature, err = schnorr.Sign(suite, dss0.secret, ps0.Hash(suite))
	require.Nil(t, err)
	err = dss1.ProcessPartialSig(ps0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not valid")
	ps0.Partial.V = goodV
	ps0.Signature = goodSig

	// fine
	err = dss1.ProcessPartialSig(ps0)
	assert.Nil(t, err)

	// already received
	assert.Error(t, dss1.ProcessPartialSig(ps0))

	// if not enough partial signatures, can't generate signature
	buff, err := dss1.Signature()
	assert.Nil(t, buff)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not enough")

	// enough partial sigs ?
	for i := 2; i < nbParticipants; i++ {
		dss := getDSS(i)
		ps, err := dss.PartialSig()
		require.Nil(t, err)
		require.Nil(t, dss1.ProcessPartialSig(ps))
	}
	assert.True(t, dss1.EnoughPartialSig())
}

func TestDSSSignature(t *testing.T) {
	dsss := make([]*DSS, nbParticipants)
	pss := make([]*PartialSig, nbParticipants)
	for i := 0; i < nbParticipants; i++ {
		dsss[i] = getDSS(i)
		ps, err := dsss[i].PartialSig()
		require.Nil(t, err)
		require.NotNil(t, ps)
		pss[i] = ps
	}
	for i, dss := range dsss {
		for j, ps := range pss {
			if i == j {
				continue
			}
			require.Nil(t, dss.ProcessPartialSig(ps))
		}
	}
	// issue and verify signature
	dss0 := dsss[0]
	buff, err := dss0.Signature()
	assert.NotNil(t, buff)
	assert.Nil(t, err)
	err = eddsa.Verify(longterms[0].Public(), dss0.msg, buff)
	assert.Nil(t, err)
	assert.Nil(t, Verify(longterms[0].Public(), dss0.msg, buff))
}

func getDSS(i int) *DSS {
	dss, err := NewDSS(suite, partSec[i], partPubs, longterms[i], randoms[i], []byte("hello"), t)
	if dss == nil || err != nil {
		panic("nil dss")
	}
	return dss
}

func genDistSecret() []*dkg.DistKeyShare {
	dkgs := make([]*dkg.DistKeyGenerator, nbParticipants)
	for i := 0; i < nbParticipants; i++ {
		dkg, err := dkg.NewDistKeyGenerator(suite, partSec[i], partPubs, random.Stream, nbParticipants/2+1)
		if err != nil {
			panic(err)
		}
		dkgs[i] = dkg
	}
	// full secret sharing exchange
	// 1. broadcast deals
	resps := make([]*dkg.Response, 0, nbParticipants*nbParticipants)
	for _, dkg := range dkgs {
		deals, err := dkg.Deals()
		if err != nil {
			panic(err)
		}
		for i, d := range deals {
			resp, err := dkgs[i].ProcessDeal(d)
			if err != nil {
				panic(err)
			}
			if !resp.Response.Approved {
				panic("wrong approval")
			}
			resps = append(resps, resp)
		}
	}
	// 2. Broadcast responses
	for _, resp := range resps {
		for h, dkg := range dkgs {
			// ignore all messages from ourself
			if resp.Response.Index == uint32(h) {
				continue
			}
			j, err := dkg.ProcessResponse(resp)
			if err != nil || j != nil {
				panic("wrongProcessResponse")
			}
		}
	}
	// 4. Broadcast secret commitment
	for i, dkg := range dkgs {
		scs, err := dkg.SecretCommits()
		if err != nil {
			panic("wrong SecretCommits")
		}
		for j, dkg2 := range dkgs {
			if i == j {
				continue
			}
			cc, err := dkg2.ProcessSecretCommits(scs)
			if err != nil || cc != nil {
				panic("wrong ProcessSecretCommits")
			}
		}
	}

	// 5. reveal shares
	dkss := make([]*dkg.DistKeyShare, len(dkgs))
	for i, dkg := range dkgs {
		dks, err := dkg.DistKeyShare()
		if err != nil {
			panic(err)
		}
		dkss[i] = dks
	}
	return dkss

}
func genPair() (kyber.Scalar, kyber.Point) {
	sc := suite.Scalar().Pick(random.Stream)
	return sc, suite.Point().Mul(sc, nil)
}

func randomBytes(n int) []byte {
	var buff = make([]byte, n)
	_, _ = rand.Read(buff[:])
	return buff
}
