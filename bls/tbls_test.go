package bls

import (
	"testing"

	"github.com/dedis/kyber/share"
	"github.com/dedis/kyber/util/random"

	"github.com/dedis/drand/pbc"
	"github.com/stretchr/testify/require"
	"github.com/dedis/kyber"
)

var pairing = pbc.NewPairingFp254BNb()
var g1 = pairing.G1()
var g2 = pairing.G2()

var nbParticipants = 7
var threshold = nbParticipants/2 + 1

func TestThresholdBLS(t *testing.T) {
	_, priPoly := genShares()
	pubPoly := priPoly.Commit(g2.Point().Base())
	// verify integrity of a share
	xiG := g2.Point().Mul(priPoly.Eval(0).V, nil)
	xiG2 := pubPoly.Eval(0).V
	require.Equal(t, xiG.String(), xiG2.String())

	// perform threshold signing and verify one
	msg := []byte("Hello World")
	tsig := ThresholdSign(pairing, priPoly.Eval(0), msg)
	require.True(t, ThresholdVerify(pairing, pubPoly, msg, tsig))

	// perform for all of them and verify all individuals sig and aggregate sig
	sigs := make([]*ThresholdSig, nbParticipants)
	for i, s := range priPoly.Shares(nbParticipants) {
		sigs[i] = ThresholdSign(pairing, s, msg)
	}
	sig, err := AggregateSignatures(pairing, pubPoly, msg, sigs, nbParticipants, threshold)
	require.Nil(t, err)
	require.Nil(t, Verify(pairing, pubPoly.Commit(), msg, sig))
}

// public keys are over g2
func genShares() (kyber.Scalar, *share.PriPoly) {
	secret := g2.Scalar().Pick(random.Stream)
	pri := share.NewPriPoly(g2, threshold, secret, random.Stream)
	return secret, pri
}
