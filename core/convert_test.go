package core

import (
	"testing"

	"github.com/drand/drand/key"
	pdkg "github.com/drand/drand/protobuf/crypto/dkg"
	"github.com/drand/kyber/share/dkg"
	"github.com/drand/kyber/util/random"
	"github.com/stretchr/testify/require"
)

func TestConvertJustification(t *testing.T) {
	j := new(dkg.JustificationBundle)
	j.Signature = []byte{1, 2, 3}
	j.DealerIndex = 32
	j.Justifications = []dkg.Justification{
		{
			ShareIndex: 10,
			Share:      key.KeyGroup.Scalar().Pick(random.New()),
		},
	}
	proto := justifToProto(j)
	justifProto, ok := proto.Bundle.(*pdkg.Packet_Justification)
	require.True(t, ok)
	require.NotNil(t, justifProto)
	bundle, err := protoToJustif(justifProto.Justification)
	require.NoError(t, err)
	require.Equal(t, j, bundle)
}
