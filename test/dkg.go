package test

// returns a list of private shares along with the list of public coefficients
import (
	"testing"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/share"
	"github.com/dedis/kyber/util/random"
	"github.com/stretchr/testify/require"
)

// of the public polynomial
func SimulateDKG(test *testing.T, g kyber.Group, n, t int) ([]*share.PriShare, []kyber.Point) {
	// Run an n-fold Pedersen VSS (= DKG)
	priPolys := make([]*share.PriPoly, n)
	priShares := make([][]*share.PriShare, n)
	pubPolys := make([]*share.PubPoly, n)
	pubShares := make([][]*share.PubShare, n)
	for i := 0; i < n; i++ {
		priPolys[i] = share.NewPriPoly(g, t, nil, random.New())
		priShares[i] = priPolys[i].Shares(n)
		pubPolys[i] = priPolys[i].Commit(nil)
		pubShares[i] = pubPolys[i].Shares(n)
	}

	// Verify VSS shares
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			sij := priShares[i][j]
			// s_ij * G
			sijG := g.Point().Base().Mul(sij.V, nil)
			require.True(test, sijG.Equal(pubShares[i][j].V))
		}
	}

	// Create private DKG shares
	dkgShares := make([]*share.PriShare, n)
	for i := 0; i < n; i++ {
		acc := g.Scalar().Zero()
		for j := 0; j < n; j++ { // assuming all participants are in the qualified set
			acc = g.Scalar().Add(acc, priShares[j][i].V)
		}
		dkgShares[i] = &share.PriShare{i, acc}
	}

	// Create public DKG commitments (= verification vector)
	dkgCommits := make([]kyber.Point, t)
	for k := 0; k < t; k++ {
		acc := g.Point().Null()
		for i := 0; i < n; i++ { // assuming all participants are in the qualified set
			_, coeff := pubPolys[i].Info()
			acc = g.Point().Add(acc, coeff[k])
		}
		dkgCommits[k] = acc
	}

	// Check that the private DKG shares verify against the public DKG commits
	dkgPubPoly := share.NewPubPoly(g, nil, dkgCommits)
	for i := 0; i < n; i++ {
		require.True(test, dkgPubPoly.Check(dkgShares[i]))
	}
	return dkgShares, dkgCommits
}
