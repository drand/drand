package dkg

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/testlogger"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/net"
	drand "github.com/drand/drand/v2/protobuf/dkg"
	"github.com/drand/kyber/share/dkg"
)

func TestNewBroadcasterWithNoParticipantsFails(t *testing.T) {
	l := testlogger.New(t)
	ctx := context.Background()
	gateway := net.PrivateGateway{}
	sch, _ := crypto.GetSchemeFromEnv()
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
	sch, _ := crypto.GetSchemeFromEnv()

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
				Key:       []byte("0000000"),
				Signature: []byte("1111111"),
			},
		},
		sch,
		&dkg.Config{},
	)
	require.NoError(t, err)
}
