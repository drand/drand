package dkg

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/dedis/kyber"
	"github.com/dedis/kyber/group/edwards25519"
	"github.com/dedis/kyber/share"
	"github.com/dedis/kyber/share/pedersen/vss"
	"github.com/dedis/kyber/util/random"
)

var suite = edwards25519.NewAES128SHA256Ed25519()

var nbParticipants = 7

var partPubs []kyber.Point
var partSec []kyber.Scalar

var dkgs []*DistKeyGenerator

func init() {
	partPubs = make([]kyber.Point, nbParticipants)
	partSec = make([]kyber.Scalar, nbParticipants)
	for i := 0; i < nbParticipants; i++ {
		sec, pub := genPair()
		partPubs[i] = pub
		partSec[i] = sec
	}
	dkgs = dkgGen()
}

func TestDKGNewDistKeyGenerator(t *testing.T) {
	long := partSec[0]
	dkg, err := NewDistKeyGenerator(suite, long, partPubs, random.Stream, nbParticipants/2+1)
	assert.Nil(t, err)
	assert.NotNil(t, dkg.dealer)

	sec, _ := genPair()
	_, err = NewDistKeyGenerator(suite, sec, partPubs, random.Stream, nbParticipants/2+1)
	assert.Error(t, err)

}

func TestDKGDeal(t *testing.T) {
	dkg := dkgs[0]

	dks, err := dkg.DistKeyShare()
	assert.Error(t, err)
	assert.Nil(t, dks)

	deals, err := dkg.Deals()
	require.Nil(t, err)
	assert.Len(t, deals, nbParticipants-1)

	for i := range deals {
		assert.NotNil(t, deals[i])
		assert.Equal(t, uint32(0), deals[i].Index)
	}

	v, ok := dkg.verifiers[dkg.index]
	assert.True(t, ok)
	assert.NotNil(t, v)
}

func TestDKGProcessDeal(t *testing.T) {
	dkgs = dkgGen()
	dkg := dkgs[0]
	deals, err := dkg.Deals()
	require.Nil(t, err)

	rec := dkgs[1]
	deal := deals[1]
	assert.Equal(t, int(deal.Index), 0)
	assert.Equal(t, uint32(1), rec.index)

	// verifier don't find itself
	goodP := rec.participants
	rec.participants = make([]kyber.Point, 0)
	resp, err := rec.ProcessDeal(deal)
	assert.Nil(t, resp)
	assert.Error(t, err)
	rec.participants = goodP

	// good deal
	resp, err = rec.ProcessDeal(deal)
	assert.NotNil(t, resp)
	assert.Equal(t, vss.StatusApproval, resp.Response.Status)
	assert.Nil(t, err)
	_, ok := rec.verifiers[deal.Index]
	require.True(t, ok)
	assert.Equal(t, uint32(0), resp.Index)

	// duplicate
	resp, err = rec.ProcessDeal(deal)
	assert.Nil(t, resp)
	assert.Error(t, err)

	// wrong index
	goodIdx := deal.Index
	deal.Index = uint32(nbParticipants + 1)
	resp, err = rec.ProcessDeal(deal)
	assert.Nil(t, resp)
	assert.Error(t, err)
	deal.Index = goodIdx

	// wrong deal
	goodSig := deal.Deal.Signature
	deal.Deal.Signature = randomBytes(len(deal.Deal.Signature))
	resp, err = rec.ProcessDeal(deal)
	assert.Nil(t, resp)
	assert.Error(t, err)
	deal.Deal.Signature = goodSig

}

/*func TestDKGProcessResponse(t *testing.T) {*/
//// first peer generates wrong deal
//// second peer processes it and returns a complaint
//// first peer process the complaint

//dkgs = dkgGen()
//dkg := dkgs[0]
//idxRec := 1
//rec := dkgs[idxRec]
//deal, err := dkg.dealer.PlaintextDeal(idxRec)
//require.Nil(t, err)

//// give a wrong deal
//goodSecret := deal.RndShare.V
//deal.RndShare.V = suite.Scalar().Zero()
//dd, err := dkg.Deals()
//encD := dd[idxRec]
//require.Nil(t, err)
//resp, err := rec.ProcessDeal(encD)
//assert.Nil(t, err)
//require.NotNil(t, resp)
//assert.Equal(t, vss.StatusComplaint, resp.Response.Status)
//deal.RndShare.V = goodSecret
//dd, _ = dkg.Deals()
//encD = dd[idxRec]

//// no verifier tied to Response
//v, ok := dkg.verifiers[0]
//require.NotNil(t, v)
//require.True(t, ok)
//require.NotNil(t, v)
//delete(dkg.verifiers, 0)
//j, err := dkg.ProcessResponse(resp)
//assert.Nil(t, j)
//assert.NotNil(t, err)
//dkg.verifiers[0] = v

//// invalid response
//goodSig := resp.Response.Signature
//resp.Response.Signature = randomBytes(len(goodSig))
//j, err = dkg.ProcessResponse(resp)
//assert.Nil(t, j)
//assert.Error(t, err)
//resp.Response.Signature = goodSig

//// valid complaint from our deal
//j, err = dkg.ProcessResponse(resp)
//assert.NotNil(t, j)
//assert.Nil(t, err)

//// valid complaint from another deal from another peer
//dkg2 := dkgs[2]
//require.Nil(t, err)
//// fake a wrong deal
////deal20, err := dkg2.dealer.PlaintextDeal(0)
////require.Nil(t, err)
//deal21, err := dkg2.dealer.PlaintextDeal(1)
//require.Nil(t, err)
//goodRnd21 := deal21.RndShare.V
//deal21.RndShare.V = suite.Scalar().Zero()
//deals2, err := dkg2.Deals()
//require.Nil(t, err)

//resp12, err := rec.ProcessDeal(deals2[idxRec])
//assert.NotNil(t, resp)
//assert.Equal(t, vss.StatusComplaint, resp12.Response.Status)

//deal21.RndShare.V = goodRnd21
//deals2, err = dkg2.Deals()
//require.Nil(t, err)

//// give it to the first peer
//// process dealer 2's deal
//r, err := dkg.ProcessDeal(deals2[0])
//assert.Nil(t, err)
//assert.NotNil(t, r)

//// process response from peer 1
//j, err = dkg.ProcessResponse(resp12)
//assert.Nil(t, j)
//assert.Nil(t, err)

//// Justification part:
//// give the complaint to the dealer
//j, err = dkg2.ProcessResponse(resp12)
//assert.Nil(t, err)
//assert.NotNil(t, j)

//// hack because all is local, and resp has been modified locally by dkg2's
//// dealer, the status has became "justified"
//resp12.Response.Status = vss.StatusComplaint
//err = dkg.ProcessJustification(j)
//assert.Nil(t, err)

//// remove verifiers
//v = dkg.verifiers[j.Index]
//delete(dkg.verifiers, j.Index)
//err = dkg.ProcessJustification(j)
//assert.Error(t, err)
//dkg.verifiers[j.Index] = v

//}

//func TestDKGSecretCommits(t *testing.T) {
//fullExchange(t)

//dkg := dkgs[0]

//sc, err := dkg.SecretCommits()
//assert.Nil(t, err)
//msg := sc.Hash(suite)
//assert.Nil(t, schnorr.Verify(suite, dkg.pub, msg, sc.Signature))

//dkg2 := dkgs[1]
//// wrong index
//goodIdx := sc.Index
//sc.Index = uint32(nbParticipants + 1)
//cc, err := dkg2.ProcessSecretCommits(sc)
//assert.Nil(t, cc)
//assert.Error(t, err)
//sc.Index = goodIdx

//// not in qual: delete the verifier
//goodV := dkg2.verifiers[uint32(0)]
//delete(dkg2.verifiers, uint32(0))
//cc, err = dkg2.ProcessSecretCommits(sc)
//assert.Nil(t, cc)
//assert.Error(t, err)
//dkg2.verifiers[uint32(0)] = goodV

//// invalid sig
//goodSig := sc.Signature
//sc.Signature = randomBytes(len(goodSig))
//cc, err = dkg2.ProcessSecretCommits(sc)
//assert.Nil(t, cc)
//assert.Error(t, err)
//sc.Signature = goodSig
//// invalid session id
//goodSid := sc.SessionID
//sc.SessionID = randomBytes(len(goodSid))
//cc, err = dkg2.ProcessSecretCommits(sc)
//assert.Nil(t, cc)
//assert.Error(t, err)
//sc.SessionID = goodSid

//// wrong commitments
//goodPoint := sc.Commitments[0]
//sc.Commitments[0] = suite.Point().Null()
//msg = sc.Hash(suite)
//sig, err := schnorr.Sign(suite, dkg.long, msg)
//require.Nil(t, err)
//goodSig = sc.Signature
//sc.Signature = sig
//cc, err = dkg2.ProcessSecretCommits(sc)
//assert.NotNil(t, cc)
//assert.Nil(t, err)
//sc.Commitments[0] = goodPoint
//sc.Signature = goodSig

//// all fine
//cc, err = dkg2.ProcessSecretCommits(sc)
//assert.Nil(t, cc)
//assert.Nil(t, err)
//}

//func TestDKGComplaintCommits(t *testing.T) {
//fullExchange(t)

//var scs []*SecretCommits
//for _, dkg := range dkgs {
//sc, err := dkg.SecretCommits()
//require.Nil(t, err)
//scs = append(scs, sc)
//}

//for _, sc := range scs {
//for _, dkg := range dkgs {
//cc, err := dkg.ProcessSecretCommits(sc)
//assert.Nil(t, err)
//assert.Nil(t, cc)
//}
//}

//// change the sc for the second one
//wrongSc := &SecretCommits{}
//wrongSc.Index = scs[0].Index
//wrongSc.SessionID = scs[0].SessionID
//wrongSc.Commitments = make([]kyber.Point, len(scs[0].Commitments))
//copy(wrongSc.Commitments, scs[0].Commitments)
////goodScCommit := scs[0].Commitments[0]
//wrongSc.Commitments[0] = suite.Point().Null()
//msg := wrongSc.Hash(suite)
//wrongSc.Signature, _ = schnorr.Sign(suite, dkgs[0].long, msg)

//dkg := dkgs[1]
//cc, err := dkg.ProcessSecretCommits(wrongSc)
//assert.Nil(t, err)
//assert.NotNil(t, cc)

//dkg2 := dkgs[2]
//// ComplaintCommits: wrong index
//goodIndex := cc.Index
//cc.Index = uint32(nbParticipants)
//rc, err := dkg2.ProcessComplaintCommits(cc)
//assert.Nil(t, rc)
//assert.Error(t, err)
//cc.Index = goodIndex

//// invalid signature
//goodSig := cc.Signature
//cc.Signature = randomBytes(len(cc.Signature))
//rc, err = dkg2.ProcessComplaintCommits(cc)
//assert.Nil(t, rc)
//assert.Error(t, err)
//cc.Signature = goodSig

//// no verifiers
//v := dkg2.verifiers[uint32(0)]
//delete(dkg2.verifiers, uint32(0))
//rc, err = dkg2.ProcessComplaintCommits(cc)
//assert.Nil(t, rc)
//assert.Error(t, err)
//dkg2.verifiers[uint32(0)] = v

//// deal does not verify
//goodDeal := cc.Deal
//cc.Deal = &vss.Deal{
//SessionID:   goodDeal.SessionID,
//SecShare:    goodDeal.SecShare,
//RndShare:    goodDeal.RndShare,
//T:           goodDeal.T,
//Commitments: goodDeal.Commitments,
//}
//rc, err = dkg2.ProcessComplaintCommits(cc)
//assert.Nil(t, rc)
//assert.Error(t, err)
//cc.Deal = goodDeal

////  no commitments
//sc := dkg2.commitments[uint32(0)]
//delete(dkg2.commitments, uint32(0))
//rc, err = dkg2.ProcessComplaintCommits(cc)
//assert.Nil(t, rc)
//assert.Error(t, err)
//dkg2.commitments[uint32(0)] = sc

//// secret commits are passing the check
//rc, err = dkg2.ProcessComplaintCommits(cc)
//assert.Nil(t, rc)
//assert.Error(t, err)

//[>
//TODO find a way to be the malicious guys,i.e.
//make a deal which validates, but revealing the commitments coefficients makes
//the check fails.
//f is the secret polynomial
//g is the "random" one
//[f(i) + g(i)]*G == [F + G](i)
//but
//f(i)*G != F(i)

//goodV := cc.Deal.SecShare.V
//goodDSig := cc.Deal.Signature
//cc.Deal.SecShare.V = suite.Scalar().Zero()
//msg = msgDeal(cc.Deal)
//sig, _ := sign.Schnorr(suite, dkgs[cc.DealerIndex].long, msg)
//cc.Deal.Signature = sig
//msg = msgCommitComplaint(cc)
//sig, _ = sign.Schnorr(suite, dkgs[cc.Index].long, msg)
//goodCCSig := cc.Signature
//cc.Signature = sig
//rc, err = dkg2.ProcessComplaintCommits(cc)
//assert.Nil(t, err)
//assert.NotNil(t, rc)
//cc.Deal.SecShare.V = goodV
//cc.Deal.Signature = goodDSig
//cc.Signature = goodCCSig
//*/

//}

//func TestDKGReconstructCommits(t *testing.T) {
//fullExchange(t)

//var scs []*SecretCommits
//for _, dkg := range dkgs {
//sc, err := dkg.SecretCommits()
//require.Nil(t, err)
//scs = append(scs, sc)
//}

//// give the secret commits to all dkgs but the second one
//for _, sc := range scs {
//for _, dkg := range dkgs[2:] {
//cc, err := dkg.ProcessSecretCommits(sc)
//assert.Nil(t, err)
//assert.Nil(t, cc)
//}
//}

//// peer 1 wants to reconstruct coeffs from dealer 1
//rc := &ReconstructCommits{
//Index:       1,
//DealerIndex: 0,
//Share:       dkgs[uint32(1)].verifiers[uint32(0)].Deal().SecShare,
//SessionID:   dkgs[uint32(1)].verifiers[uint32(0)].Deal().SessionID,
//}
//msg := rc.Hash(suite)
//rc.Signature, _ = schnorr.Sign(suite, dkgs[1].long, msg)

//dkg2 := dkgs[2]
//// reconstructed already set
//dkg2.reconstructed[0] = true
//assert.Nil(t, dkg2.ProcessReconstructCommits(rc))
//delete(dkg2.reconstructed, uint32(0))

//// commitments not invalidated by any complaints
//assert.Error(t, dkg2.ProcessReconstructCommits(rc))
//delete(dkg2.commitments, uint32(0))

//// invalid index
//goodI := rc.Index
//rc.Index = uint32(nbParticipants)
//assert.Error(t, dkg2.ProcessReconstructCommits(rc))
//rc.Index = goodI

//// invalid sig
//goodSig := rc.Signature
//rc.Signature = randomBytes(len(goodSig))
//assert.Error(t, dkg2.ProcessReconstructCommits(rc))
//rc.Signature = goodSig

//// all fine
//assert.Nil(t, dkg2.ProcessReconstructCommits(rc))

//// packet already received
//var found bool
//for _, p := range dkg2.pendingReconstruct[rc.DealerIndex] {
//if p.Index == rc.Index {
//found = true
//break
//}
//}
//assert.True(t, found)
//assert.False(t, dkg2.Finished())
//// generate enough secret commits  to recover the secret
//for _, dkg := range dkgs[2:] {
//rc = &ReconstructCommits{
//SessionID:   dkg.verifiers[uint32(0)].Deal().SessionID,
//Index:       dkg.index,
//DealerIndex: 0,
//Share:       dkg.verifiers[uint32(0)].Deal().SecShare,
//}
//msg := rc.Hash(suite)
//rc.Signature, _ = schnorr.Sign(suite, dkg.long, msg)

//if dkg2.reconstructed[uint32(0)] {
//break
//}
//// invalid session ID
//goodSID := rc.SessionID
//rc.SessionID = randomBytes(len(goodSID))
//require.Error(t, dkg2.ProcessReconstructCommits(rc))
//rc.SessionID = goodSID

//_ = dkg2.ProcessReconstructCommits(rc)
//}
//assert.True(t, dkg2.reconstructed[uint32(0)])
//com := dkg2.commitments[uint32(0)]
//assert.NotNil(t, com)
//assert.Equal(t, dkgs[0].dealer.SecretCommit().String(), com.Commit().String())

//assert.True(t, dkg2.Finished())
//}

func TestDistKeyShare(t *testing.T) {
	fullExchange(t)

	for _, dkg := range dkgs {
		assert.True(t, dkg.Certified())
	}
	// verify integrity of shares etc
	dkss := make([]*DistKeyShare, nbParticipants)
	for i, dkg := range dkgs {
		dks, err := dkg.DistKeyShare()
		require.Nil(t, err)
		require.NotNil(t, dks)
		dkss[i] = dks
		assert.Equal(t, dkg.index, uint32(dks.Share.I))
	}

	shares := make([]*share.PriShare, nbParticipants)
	for i, dks := range dkss {
		assert.True(t, checkDks(dks, dkss[0]), "dist key share not equal %d vs %d", dks.Share.I, 0)
		shares[i] = dks.Share
	}

	secret, err := share.RecoverSecret(suite, shares, nbParticipants, nbParticipants)
	assert.Nil(t, err)

	commitSecret := suite.Point().Mul(secret, nil)
	assert.Equal(t, dkss[0].Public().String(), commitSecret.String())
}

func dkgGen() []*DistKeyGenerator {
	dkgs := make([]*DistKeyGenerator, nbParticipants)
	for i := 0; i < nbParticipants; i++ {
		dkg, err := NewDistKeyGenerator(suite, partSec[i], partPubs, random.Stream, nbParticipants/2+1)
		if err != nil {
			panic(err)
		}
		dkgs[i] = dkg
	}
	return dkgs
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
func checkDks(dks1, dks2 *DistKeyShare) bool {
	if len(dks1.Commits) != len(dks2.Commits) {
		return false
	}
	for i, p := range dks1.Commits {
		if !p.Equal(dks2.Commits[i]) {
			return false
		}
	}
	return true
}

func fullExchange(t *testing.T) {
	dkgs = dkgGen()
	// full secret sharing exchange
	// 1. broadcast deals
	resps := make([]*Response, 0, nbParticipants*nbParticipants)
	for _, dkg := range dkgs {
		deals, err := dkg.Deals()
		require.Nil(t, err)
		for i, d := range deals {
			resp, err := dkgs[i].ProcessDeal(d)
			require.Nil(t, err)
			require.Equal(t, vss.StatusApproval, resp.Response.Status)
			resps = append(resps, resp)
		}
	}
	// 2. Broadcast responses
	for _, resp := range resps {
		for _, dkg := range dkgs {
			// ignore all messages from ourself
			if resp.Response.Index == dkg.index {
				continue
			}
			j, err := dkg.ProcessResponse(resp)
			require.Nil(t, err)
			require.Nil(t, j)
		}
	}
	// 3. make sure everyone has the same QUAL set
	for _, dkg := range dkgs {
		for _, dkg2 := range dkgs {
			require.True(t, dkg.isInQUAL(dkg2.index))
		}
	}

}
