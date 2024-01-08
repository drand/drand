package drand

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/dkg"
	drand "github.com/drand/drand/v2/protobuf/dkg"
)

func TestDKGPrintModelConversion(t *testing.T) {
	now := time.Date(2023, 1, 1, 1, 1, 2, 0, time.UTC)
	genesisSeed, err := hex.DecodeString("deadbeef")
	require.NoError(t, err)
	entry := drand.DKGEntry{
		BeaconID:    "banana",
		State:       uint32(dkg.Complete),
		Epoch:       3,
		Threshold:   2,
		Timeout:     timestamppb.New(now),
		GenesisTime: timestamppb.New(now.Add(1 * time.Minute)),
		GenesisSeed: genesisSeed,
		Leader:      NewParticipant("alice"),
		Remaining:   []*drand.Participant{NewParticipant("alice"), NewParticipant("bob"), NewParticipant("carol")},
		Joining:     []*drand.Participant{NewParticipant("david")},
		Leaving:     nil,
		Acceptors:   nil,
		Rejectors:   nil,
		FinalGroup:  []string{"alice", "bob", "carol"},
	}
	printModel := convert(&entry)

	require.Equal(t, "banana", printModel.BeaconID)
	require.Equal(t, "Complete", printModel.Status)
	require.Equal(t, "3", printModel.Epoch)
	require.Equal(t, "2", printModel.Threshold)
	require.Equal(t, "2023-01-01T01:01:02Z", printModel.Timeout)
	require.Equal(t, "2023-01-01T01:02:02Z", printModel.GenesisTime)
	require.Equal(t, "deadbeef", printModel.GenesisSeed)
	require.Equal(t, "alice", printModel.Leader)
	require.Equal(t, "[\n\talice,\n\tbob,\n\tcarol,\n]", printModel.Remaining)
	require.Equal(t, `[david]`, printModel.Joining)
	require.Equal(t, "[]", printModel.Leaving)
	require.Equal(t, "[]", printModel.Accepted)
	require.Equal(t, "[]", printModel.Rejected)
	require.Equal(t, "[\n\talice,\n\tbob,\n\tcarol,\n]", printModel.FinalGroup)
}

func TestNilDoesntPanic(t *testing.T) {
	require.Equal(t, convert(nil), printModel{})
}

func NewParticipant(name string) *drand.Participant {
	sch, _ := crypto.GetSchemeFromEnv()
	k, _ := key.NewKeyPair(name, sch)
	pk, _ := k.Public.Key.MarshalBinary()
	return &drand.Participant{
		Address: name,
		Key:     pk,
		Tls:     false,
	}
}
