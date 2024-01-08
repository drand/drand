//nolint:lll,dupl,funlen
package dkg

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/drand/drand/common/key"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/internal/net"
	"github.com/drand/drand/internal/util"
	drand "github.com/drand/drand/protobuf/dkg"
)

func TestInitialDKG(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	myKeypair, err := key.NewKeyPair("somebody.com:443", sch)
	require.NoError(t, err)

	alice, err := util.PublicKeyAsParticipant(myKeypair.Public)
	require.NoError(t, err)
	bob := NewParticipant("bob")
	carol := NewParticipant("carol")
	beaconID := "someBeaconID"
	tests := []struct {
		name                     string
		proposal                 *drand.FirstProposalOptions
		prepareMocks             func(store *MockStore, client *MockDKGClient, proposal *drand.FirstProposalOptions, expectedError error)
		expectedError            error
		expectedNetworkCallCount int
	}{
		{
			name: "valid proposal with successful gossip sends to all parties except leader",
			proposal: &drand.FirstProposalOptions{
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				Threshold:            1,
				PeriodSeconds:        10,
				Scheme:               sch.Name,
				CatchupPeriodSeconds: 10,
				GenesisTime:          timestamppb.New(time.Now()),
				Joining:              []*drand.Participant{alice, bob, carol},
			},
			prepareMocks: func(store *MockStore, client *MockDKGClient, proposal *drand.FirstProposalOptions, expectedError error) {
				store.On("GetCurrent", beaconID).Return(NewFreshState(beaconID), nil)
				store.On("SaveCurrent", beaconID, mock.Anything).Return(nil)
				client.On("Packet", mock.Anything, mock.Anything).Return(nil, nil)
			},
			expectedError:            nil,
			expectedNetworkCallCount: 2,
		},
		{
			name: "database get failure does not attempt to call network",
			proposal: &drand.FirstProposalOptions{
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				Threshold:            1,
				PeriodSeconds:        10,
				Scheme:               sch.Name,
				CatchupPeriodSeconds: 10,
				GenesisTime:          timestamppb.New(time.Now()),
				Joining:              []*drand.Participant{alice, bob},
			},
			prepareMocks: func(store *MockStore, client *MockDKGClient, proposal *drand.FirstProposalOptions, expectedError error) {
				store.On("GetCurrent", beaconID).Return(nil, expectedError)
			},
			expectedError:            errors.New("some-error"),
			expectedNetworkCallCount: 0,
		},
		{
			name: "database store failure does not attempt to call network",
			proposal: &drand.FirstProposalOptions{
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				Threshold:            1,
				PeriodSeconds:        10,
				Scheme:               sch.Name,
				CatchupPeriodSeconds: 10,
				GenesisTime:          timestamppb.New(time.Now()),
				Joining:              []*drand.Participant{alice, bob},
			},
			prepareMocks: func(store *MockStore, client *MockDKGClient, proposal *drand.FirstProposalOptions, expectedError error) {
				store.On("GetCurrent", beaconID).Return(NewFreshState(beaconID), nil)
				store.On("SaveCurrent", beaconID, mock.Anything).Return(expectedError)
			},
			expectedError:            errors.New("some-error"),
			expectedNetworkCallCount: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identityProvider := MockIdentityProvider{}
			store := MockStore{}
			client := MockDKGClient{}
			process := Process{
				beaconIdentifier: &identityProvider,
				store:            &store,
				internalClient:   &client,
				log:              log.New(nil, log.DebugLevel, true),
				SeenPackets:      make(map[string]bool),
				config:           Config{},
			}

			test.prepareMocks(&store, &client, test.proposal, test.expectedError)
			identityProvider.On("KeypairFor", beaconID).Return(myKeypair, nil)

			_, err = process.Command(context.Background(), &drand.DKGCommand{Command: &drand.DKGCommand_Initial{
				Initial: test.proposal,
			}, Metadata: &drand.CommandMetadata{
				BeaconID: beaconID,
			}})

			if test.expectedError != nil {
				require.Error(t, err, test.expectedError)
			} else {
				require.NoError(t, err)
			}

			client.AssertNumberOfCalls(t, "Packet", test.expectedNetworkCallCount)
		})
	}
}

func TestReshare(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	myKeypair, err := key.NewKeyPair("somebody.com:443", sch)
	require.NoError(t, err)

	alice, err := util.PublicKeyAsParticipant(myKeypair.Public)
	require.NoError(t, err)
	bob := NewParticipant("bob")
	carol := NewParticipant("carol")
	beaconID := "someBeaconID"
	currentState := NewCompleteDKGEntry(t, beaconID, Complete, alice, bob)
	validProposal := drand.ProposalOptions{
		Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
		Threshold:            1,
		CatchupPeriodSeconds: 10,
		Joining:              []*drand.Participant{carol},
		Remaining:            []*drand.Participant{alice, bob},
	}

	tests := []struct {
		name                     string
		proposal                 *drand.ProposalOptions
		prepareMocks             func(store *MockStore, client *MockDKGClient, proposal *drand.ProposalOptions, expectedError error)
		expectedError            error
		validateOutput           func(output *DBState)
		expectedNetworkCallCount int
	}{
		{
			name:     "valid proposal with successful gossip sends to all parties except leader",
			proposal: &validProposal,
			prepareMocks: func(store *MockStore, client *MockDKGClient, proposal *drand.ProposalOptions, expectedError error) {
				store.On("GetCurrent", beaconID).Return(currentState, nil)
				store.On("SaveCurrent", beaconID, mock.Anything).Return(nil)
				client.On("Packet", mock.Anything, mock.Anything).Return(nil, nil)
			},
			validateOutput: func(output *DBState) {
				require.Equal(t, Proposing, output.State)
				require.Equal(t, uint32(2), output.Epoch)
			},
			expectedError:            nil,
			expectedNetworkCallCount: 2,
		},
		{
			name:     "database get failure does not attempt to call network",
			proposal: &validProposal,
			prepareMocks: func(store *MockStore, client *MockDKGClient, proposal *drand.ProposalOptions, expectedError error) {
				store.On("GetCurrent", beaconID).Return(nil, expectedError)
			},
			expectedError:            errors.New("some-error"),
			expectedNetworkCallCount: 0,
		},
		{
			name:     "database store failure does not attempt to call network",
			proposal: &validProposal,
			prepareMocks: func(store *MockStore, client *MockDKGClient, proposal *drand.ProposalOptions, expectedError error) {
				store.On("GetCurrent", beaconID).Return(currentState, nil)
				store.On("SaveCurrent", beaconID, mock.Anything).Return(expectedError)
			},
			expectedError:            errors.New("some-error"),
			expectedNetworkCallCount: 0,
		},
		{
			name:     "valid proposal after abort does not change epoch",
			proposal: &validProposal,
			prepareMocks: func(store *MockStore, client *MockDKGClient, proposal *drand.ProposalOptions, expectedError error) {
				abortedState := NewCompleteDKGEntry(t, beaconID, Aborted, alice, bob)
				abortedState.Epoch = uint32(2)
				store.On("GetCurrent", beaconID).Return(abortedState, nil)
				store.On("SaveCurrent", beaconID, mock.Anything).Return(nil)
				client.On("Packet", mock.Anything, mock.Anything).Return(nil, nil)
			},
			validateOutput: func(output *DBState) {
				require.Equal(t, Proposing, output.State)
				require.Equal(t, uint32(2), output.Epoch)
			},
			expectedError:            nil,
			expectedNetworkCallCount: 2,
		},
		{
			name:     "valid proposal after timeout does not change epoch",
			proposal: &validProposal,
			prepareMocks: func(store *MockStore, client *MockDKGClient, proposal *drand.ProposalOptions, expectedError error) {
				timedOutState := NewCompleteDKGEntry(t, beaconID, TimedOut, alice, bob)
				timedOutState.Epoch = uint32(2)
				store.On("GetCurrent", beaconID).Return(timedOutState, nil)
				store.On("SaveCurrent", beaconID, mock.Anything).Return(nil)
				client.On("Packet", mock.Anything, mock.Anything).Return(nil, nil)
			},
			validateOutput: func(output *DBState) {
				require.Equal(t, Proposing, output.State)
				require.Equal(t, uint32(2), output.Epoch)
			},
			expectedError:            nil,
			expectedNetworkCallCount: 2,
		},
		{
			name:     "valid proposal after fail does not change epoch",
			proposal: &validProposal,
			prepareMocks: func(store *MockStore, client *MockDKGClient, proposal *drand.ProposalOptions, expectedError error) {
				failedState := NewCompleteDKGEntry(t, beaconID, Failed, alice, bob)
				failedState.Epoch = uint32(2)
				store.On("GetCurrent", beaconID).Return(failedState, nil)
				store.On("SaveCurrent", beaconID, mock.Anything).Return(nil)
				client.On("Packet", mock.Anything, mock.Anything).Return(nil, nil)
			},
			validateOutput: func(output *DBState) {
				require.Equal(t, Proposing, output.State)
				require.Equal(t, uint32(2), output.Epoch)
			},
			expectedError:            nil,
			expectedNetworkCallCount: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identityProvider := MockIdentityProvider{}
			store := MockStore{}
			client := MockDKGClient{}
			process := Process{
				beaconIdentifier: &identityProvider,
				store:            &store,
				internalClient:   &client,
				log:              log.New(nil, log.DebugLevel, true),
				SeenPackets:      make(map[string]bool),
				config:           Config{},
			}

			test.prepareMocks(&store, &client, test.proposal, test.expectedError)
			identityProvider.On("KeypairFor", beaconID).Return(myKeypair, nil)

			_, err = process.Command(context.Background(), &drand.DKGCommand{Command: &drand.DKGCommand_Resharing{
				Resharing: test.proposal,
			}, Metadata: &drand.CommandMetadata{
				BeaconID: beaconID,
			}})

			if test.expectedError != nil {
				require.Error(t, err, test.expectedError)
			} else {
				require.NoError(t, err)
			}
			// this tests the DBState stored
			for _, c := range store.Calls {
				if c.Method == "SaveCurrent" && test.validateOutput != nil {
					test.validateOutput(c.Arguments[1].(*DBState))
				}
			}

			client.AssertNumberOfCalls(t, "Packet", test.expectedNetworkCallCount)
		})
	}
}

func TestJoin(t *testing.T) {
	sch, _ := crypto.GetSchemeFromEnv()
	myKeypair, err := key.NewKeyPair("somebody.com:443", sch)
	require.NoError(t, err)

	alice, err := util.PublicKeyAsParticipant(myKeypair.Public)
	require.NoError(t, err)
	bob := NewParticipant("bob")
	beaconID := "someBeaconID"

	var groupFile bytes.Buffer
	pub, err := myKeypair.Public.Key.MarshalBinary()
	require.NoError(t, err)
	err = toml.NewEncoder(&groupFile).Encode(&key.GroupTOML{
		Threshold:     2,
		Period:        "5s",
		CatchupPeriod: "5s",
		Nodes: []*key.NodeTOML{
			{
				PublicTOML: &key.PublicTOML{
					Address:    alice.Address,
					SchemeName: sch.Name,
					Signature:  "deadbeef",
					Key:        hex.EncodeToString(pub),
				},
				Index: 1,
			},
			{
				PublicTOML: &key.PublicTOML{
					Address:    bob.Address,
					SchemeName: sch.Name,
					Signature:  "deadbeef",
					Key:        hex.EncodeToString(pub),
				},
				Index: 2,
			},
			{
				PublicTOML: &key.PublicTOML{
					Address:    carol.Address,
					SchemeName: sch.Name,
					Signature:  "deadbeef",
					Key:        hex.EncodeToString(pub),
				},
				Index: 3,
			},
		},
	})
	require.NoError(t, err)

	tests := []struct {
		name                     string
		joinOptions              *drand.JoinOptions
		prepareMocks             func(store *MockStore, client *MockDKGClient, expectedError error)
		expectedError            error
		validateOutput           func(output *DBState)
		expectedNetworkCallCount int
	}{
		{
			name:        "join on first epoch succeeds without group file but does not gossip anything",
			joinOptions: &drand.JoinOptions{},
			prepareMocks: func(store *MockStore, client *MockDKGClient, expectedError error) {
				current := NewCompleteDKGEntry(t, beaconID, Proposed, bob)
				current.Joining = []*drand.Participant{alice, bob, carol}
				current.Remaining = nil
				store.On("GetCurrent", beaconID).Return(current, nil)
				store.On("SaveCurrent", beaconID, mock.Anything).Return(nil)
				client.On("Packet", mock.Anything, mock.Anything).Return(nil, nil)
			},
			validateOutput: func(output *DBState) {
				require.Equal(t, Joined, output.State)
			},
			expectedNetworkCallCount: 0,
		},
		{
			name:        "join on second epoch succeeds with group file but does not gossip anything",
			joinOptions: &drand.JoinOptions{GroupFile: groupFile.Bytes()},
			prepareMocks: func(store *MockStore, client *MockDKGClient, expectedError error) {
				current := NewCompleteDKGEntry(t, beaconID, Proposed, bob, carol)
				current.Epoch = 2
				current.Joining = []*drand.Participant{alice}
				store.On("GetCurrent", beaconID).Return(current, nil)
				store.On("SaveCurrent", beaconID, mock.Anything).Return(nil)
				client.On("Packet", mock.Anything, mock.Anything).Return(nil, nil)
			},
			validateOutput: func(output *DBState) {
				require.Equal(t, Joined, output.State)
			},
			expectedNetworkCallCount: 0,
		},
		{
			name:        "join on second epoch succeeds without group file fails",
			joinOptions: &drand.JoinOptions{GroupFile: nil},
			prepareMocks: func(store *MockStore, client *MockDKGClient, expectedError error) {
				current := NewCompleteDKGEntry(t, beaconID, Proposed, alice, bob)
				current.Epoch = 2
				current.Joining = []*drand.Participant{carol}
				store.On("GetCurrent", beaconID).Return(current, nil)
				store.On("SaveCurrent", beaconID, mock.Anything).Return(nil)
				client.On("Packet", mock.Anything, mock.Anything).Return(nil, nil)
			},
			validateOutput: func(output *DBState) {
				require.Equal(t, Joined, output.State)
			},
			expectedError:            errors.New("group file required to join after the first epoch"),
			expectedNetworkCallCount: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identityProvider := MockIdentityProvider{}
			store := MockStore{}
			client := MockDKGClient{}
			process := Process{
				beaconIdentifier: &identityProvider,
				store:            &store,
				internalClient:   &client,
				log:              log.New(nil, log.DebugLevel, true),
				SeenPackets:      make(map[string]bool),
				config:           Config{},
			}

			test.prepareMocks(&store, &client, test.expectedError)
			identityProvider.On("KeypairFor", beaconID).Return(myKeypair, nil)

			_, err = process.Command(context.Background(), &drand.DKGCommand{Command: &drand.DKGCommand_Join{
				Join: test.joinOptions,
			}, Metadata: &drand.CommandMetadata{
				BeaconID: beaconID,
			}})

			if test.expectedError != nil {
				require.Error(t, err, test.expectedError)
			} else {
				require.NoError(t, err)
			}

			// this tests the DBState stored
			for _, c := range store.Calls {
				if c.Method == "SaveCurrent" && test.validateOutput != nil {
					test.validateOutput(c.Arguments[1].(*DBState))
				}
			}

			client.AssertNumberOfCalls(t, "Packet", test.expectedNetworkCallCount)
		})
	}
}

type MockIdentityProvider struct {
	mock.Mock
}

func (d *MockIdentityProvider) KeypairFor(beaconID string) (*key.Pair, error) {
	args := d.Called(beaconID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*key.Pair), args.Error(1)
}

type MockStore struct {
	mock.Mock
}

func (m *MockStore) GetCurrent(beaconID string) (*DBState, error) {
	args := m.Called(beaconID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DBState), args.Error(1)
}

func (m *MockStore) GetFinished(beaconID string) (*DBState, error) {
	args := m.Called(beaconID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DBState), args.Error(1)
}

func (m *MockStore) SaveCurrent(beaconID string, state *DBState) error {
	args := m.Called(beaconID, state)
	return args.Error(0)
}

func (m *MockStore) SaveFinished(beaconID string, state *DBState) error {
	args := m.Called(beaconID, state)
	return args.Error(0)
}

func (m *MockStore) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockStore) MigrateFromGroupfile(beaconID string, group *key.Group, share *key.Share) error {
	args := m.Called(beaconID, group, share)
	return args.Error(0)
}

type MockDKGClient struct {
	mock.Mock
}

func (m *MockDKGClient) Command(_ context.Context, _ net.Peer, _ *drand.DKGCommand, _ ...grpc.CallOption) (*drand.EmptyDKGResponse, error) {
	panic("implement me")
}

func (m *MockDKGClient) Packet(_ context.Context, _ net.Peer, in *drand.GossipPacket, _ ...grpc.CallOption) (*drand.EmptyDKGResponse, error) {
	args := m.Called(in)
	return nil, args.Error(0)
}

func (m *MockDKGClient) DKGStatus(_ context.Context, _ net.Peer, _ *drand.DKGStatusRequest, _ ...grpc.CallOption) (*drand.DKGStatusResponse, error) {
	panic("implement me")
}

func (m *MockDKGClient) BroadcastDKG(_ context.Context, _ net.Peer, in *drand.DKGPacket, _ ...grpc.CallOption) (*drand.EmptyDKGResponse, error) {
	args := m.Called(in)
	return nil, args.Error(0)
}
