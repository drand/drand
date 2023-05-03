package dkg

import (
	"context"
	"github.com/drand/drand/crypto"
	"testing"

	"github.com/drand/drand/common"
	"github.com/drand/drand/internal/test/testlogger"
	"github.com/drand/kyber/share/dkg"

	"github.com/drand/drand/internal/net"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/protobuf/drand"
)

func TestNewBroadcasterWithNoParticipantsFails(t *testing.T) {
	l := testlogger.New(t)
	ctx := context.Background()
	gateway := net.PrivateGateway{}
	sch, _ := crypto.GetSchemeByIDWithDefault("")
	_, err := newEchoBroadcast(
		ctx,
		gateway.DKGClient,
		l,
		common.GetAppVersion(),
		"default",
		"127.0.0.1:8080",
		[]*drand.Participant{},
		sch,
		&dkg.Config{},
	)
	require.Error(t, err)
}

func TestNewBroadcasterWithParticipantsDoesNotFail(t *testing.T) {
	l := testlogger.New(t)
	ctx := context.Background()
	gateway := net.PrivateGateway{}
	sch, _ := crypto.GetSchemeByIDWithDefault("")

	_, err := newEchoBroadcast(
		ctx,
		gateway.DKGClient,
		l,
		common.GetAppVersion(),
		"default",
		"127.0.0.1:8080",
		[]*drand.Participant{
			{
				Address:   "127.0.0.1:1234",
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
