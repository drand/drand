package dleq

import (
	"testing"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/group/edwards25519"
	"github.com/dedis/kyber/util/random"
	"github.com/stretchr/testify/require"
)

func TestDLEQProof(t *testing.T) {
	suite := edwards25519.NewAES128SHA256Ed25519()
	n := 10
	for i := 0; i < n; i++ {
		// Create some random secrets and base points
		x := suite.Scalar().Pick(random.Stream)
		g := suite.Point().Pick(random.Stream)
		h := suite.Point().Pick(random.Stream)
		proof, xG, xH, err := NewDLEQProof(suite, g, h, x)
		require.Equal(t, err, nil)
		require.Nil(t, proof.Verify(suite, g, h, xG, xH))
	}
}

func TestDLEQProofBatch(t *testing.T) {
	suite := edwards25519.NewAES128SHA256Ed25519()
	n := 10
	x := make([]kyber.Scalar, n)
	g := make([]kyber.Point, n)
	h := make([]kyber.Point, n)
	for i := range x {
		x[i] = suite.Scalar().Pick(random.Stream)
		g[i] = suite.Point().Pick(random.Stream)
		h[i] = suite.Point().Pick(random.Stream)
	}
	proofs, xG, xH, err := NewDLEQProofBatch(suite, g, h, x)
	require.Equal(t, err, nil)
	for i := range proofs {
		require.Nil(t, proofs[i].Verify(suite, g[i], h[i], xG[i], xH[i]))
	}
}

func TestDLEQLengths(t *testing.T) {
	suite := edwards25519.NewAES128SHA256Ed25519()
	n := 10
	x := make([]kyber.Scalar, n)
	g := make([]kyber.Point, n)
	h := make([]kyber.Point, n)
	for i := range x {
		x[i] = suite.Scalar().Pick(random.Stream)
		g[i] = suite.Point().Pick(random.Stream)
		h[i] = suite.Point().Pick(random.Stream)
	}
	// Remove an element to make the test fail
	x = append(x[:5], x[6:]...)
	_, _, _, err := NewDLEQProofBatch(suite, g, h, x)
	require.Equal(t, err, errorDifferentLengths)
}
