package dkg

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/common"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
)

func TestNewBroadcasterWithNoParticipantsFails(t *testing.T) {
	_, err := newEchoBroadcast(log.DefaultLogger(),
		common.GetAppVersion(),
		"default",
		"localhost:8080",
		[]*drand.Participant{},
		func(p packet) error {
			return nil
		},
	)
	require.Error(t, err)
}

func TestNewBroadcasterWithParticipantsDoesNotFail(t *testing.T) {
	_, err := newEchoBroadcast(log.DefaultLogger(),
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
		func(p packet) error {
			return nil
		},
	)
	require.NoError(t, err)
}
