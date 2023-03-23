package dkg

import (
	"testing"

	"github.com/drand/drand/crypto"
	"github.com/drand/drand/test/testlogger"
	"github.com/drand/kyber/share/dkg"

	"github.com/drand/drand/net"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/common"
	"github.com/drand/drand/protobuf/drand"
)

func TestNewBroadcasterWithNoParticipantsFails(t *testing.T) {
	l := testlogger.New(t)
	gateway := net.PrivateGateway{}
	sch, _ := crypto.GetSchemeByIDWithDefault("")
	_, err := newEchoBroadcast(
		gateway.DKGClient,
		l,
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
	l := testlogger.New(t)
	gateway := net.PrivateGateway{}
	sch, _ := crypto.GetSchemeByIDWithDefault("")

	_, err := newEchoBroadcast(
		gateway.DKGClient,
		l,
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
