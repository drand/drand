package dkg

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common/testlogger"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/net"
	drand "github.com/drand/drand/v2/protobuf/dkg"
	"github.com/drand/kyber/share/dkg"
)

func TestMapSet(t *testing.T) {
	s := newMapSet()

	// Test empty set
	h1 := hash([]byte("hash1"))
	require.False(t, s.exists(h1))

	// Test put and exists
	s.put(h1)
	require.True(t, s.exists(h1))

	// Test that different hash doesn't exist
	h2 := hash([]byte("hash2"))
	require.False(t, s.exists(h2))

	// Test adding second hash
	s.put(h2)
	require.True(t, s.exists(h1))
	require.True(t, s.exists(h2))

	// Test duplicate put doesn't cause issues
	s.put(h1)
	require.True(t, s.exists(h1))
}

func TestMapSetWithEmptyHash(t *testing.T) {
	s := newMapSet()

	// Test with empty hash
	emptyHash := hash([]byte{})
	require.False(t, s.exists(emptyHash))
	s.put(emptyHash)
	require.True(t, s.exists(emptyHash))
}

func TestNewBroadcasterWithNoParticipantsFails(t *testing.T) {
	l := testlogger.New(t)
	ctx := context.Background()
	gateway := net.PrivateGateway{}
	sch, _ := crypto.GetSchemeFromEnv()
	_, err := newEchoBroadcast(
		ctx,
		gateway.DKGClient,
		l,
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
