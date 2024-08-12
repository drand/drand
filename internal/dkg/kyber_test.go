package dkg_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/kyber/share/dkg"
)

func TestPedersenDkgCheckEmptyList(t *testing.T) {
	t.Skip("unskip once a new Kyber library is released")
	c := dkg.Config{
		Suite:          nil,
		Longterm:       nil,
		OldNodes:       make([]dkg.Node, 0),
		PublicCoeffs:   nil,
		NewNodes:       make([]dkg.Node, 0),
		Share:          nil,
		Threshold:      0,
		OldThreshold:   0,
		Reader:         nil,
		UserReaderOnly: false,
		FastSync:       false,
		Nonce:          nil,
		Auth:           nil,
		Log:            nil,
	}
	_, err := dkg.NewDistKeyHandler(&c)
	require.ErrorContains(t, err, "dkg: can't run with empty node list")
}
