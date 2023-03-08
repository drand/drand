package dkg

import (
	"testing"

	"github.com/drand/drand/crypto"
	"github.com/drand/kyber/share/dkg"

	"github.com/drand/drand/net"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/common"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
)

func TestNewBroadcasterWithNoParticipantsFails(t *testing.T) {
	gateway := net.PrivateGateway{}
	sch, _ := crypto.GetSchemeByIDWithDefault("")
	_, err := newEchoBroadcast(
		gateway.DKGClient,
		log.DefaultLogger(),
		common.GetAppVersion(),
		"default",
		"localhost:8080",
		[]*drand.Participant{},
		sch,
		&dkg.Config{},
	)
	require.Error(t, err)
}

func TestNewBroadcasterWithParticipantsDoesNotFail(t *testing.T) {
	gateway := net.PrivateGateway{}
	sch, _ := crypto.GetSchemeByIDWithDefault("")

	_, err := newEchoBroadcast(
		gateway.DKGClient,
		log.DefaultLogger(),
		common.GetAppVersion(),
		"default",
		"localhost:8080",
		[]*drand.Participant{
			{
				Address:   "localhost:1234",
				Tls:       false,
				PubKey:    []byte("0000000"),
				Signature: []byte("1111111"),
			},
		},
		sch,
		&dkg.Config{},
	)
	require.NoError(t, err)
}
