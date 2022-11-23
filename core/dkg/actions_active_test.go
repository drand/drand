package dkg

import (
	"context"
	"errors"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"testing"
	"time"
)

func TestStartNetwork(t *testing.T) {
	me := NewParticipant()
	anotherParticipant := NewParticipant()
	anotherParticipant.Address = "anotherparticipant.com"
	beaconID := "someBeaconID"

	tests := []struct {
		name                     string
		proposal                 drand.FirstProposalOptions
		prepareMocks             func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.FirstProposalOptions, expectedError error)
		expectedError            error
		expectedNetworkCallCount int
	}{
		{
			name: "valid proposal is stored and does not attempt rollback",
			proposal: drand.FirstProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				Threshold:            1,
				PeriodSeconds:        10,
				Scheme:               "bls-pedersen-chained",
				CatchupPeriodSeconds: 10,
				GenesisTime:          timestamppb.New(time.Now()),
				GenesisSeed:          []byte("cafebabe"),
				Joining:              []*drand.Participant{me, anotherParticipant},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.FirstProposalOptions, expectedError error) {
				identityProvider.On("IdentityFor", beaconID).Return(asIdentity(me))
				store.On("GetCurrent", beaconID).Return(NewFreshState(beaconID), nil)
				network.On("Send", me, proposal.Joining, mock.Anything).Return(nil).Once()
				store.On("SaveCurrent", beaconID, mock.Anything).Return(nil)
			},
			expectedError:            nil,
			expectedNetworkCallCount: 1, // no rollback
		},
		{
			name: "error fetching identity is propagated and network is not called",
			proposal: drand.FirstProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				Threshold:            1,
				PeriodSeconds:        10,
				Scheme:               "bls-pedersen-chained",
				CatchupPeriodSeconds: 10,
				GenesisTime:          timestamppb.New(time.Now()),
				GenesisSeed:          []byte("cafebabe"),
				Joining:              []*drand.Participant{me, anotherParticipant},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.FirstProposalOptions, expectedError error) {
				identityProvider.On("IdentityFor", beaconID).Return(nil, expectedError)
			},
			expectedError:            errors.New("expected identity error"),
			expectedNetworkCallCount: 0,
		},
		{
			name: "error fetching the latest DKG state is propagated and network not called",
			proposal: drand.FirstProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				Threshold:            1,
				PeriodSeconds:        10,
				Scheme:               "bls-pedersen-chained",
				CatchupPeriodSeconds: 10,
				GenesisTime:          timestamppb.New(time.Now()),
				GenesisSeed:          []byte("cafebabe"),
				Joining:              []*drand.Participant{me, anotherParticipant},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.FirstProposalOptions, expectedError error) {
				identityProvider.On("IdentityFor", beaconID).Return(asIdentity(me))
				store.On("GetCurrent", beaconID).Return(nil, expectedError)
			},
			expectedError:            errors.New("expected database error"),
			expectedNetworkCallCount: 0,
		},
		{
			name: "invalid proposal propagates error and network not called",
			proposal: drand.FirstProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				Threshold:            5, // the threshold is higher than the node count
				PeriodSeconds:        10,
				Scheme:               "bls-pedersen-chained",
				CatchupPeriodSeconds: 10,
				GenesisTime:          timestamppb.New(time.Now()),
				GenesisSeed:          []byte("cafebabe"),
				Joining:              []*drand.Participant{me, anotherParticipant},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.FirstProposalOptions, expectedError error) {
				identityProvider.On("IdentityFor", beaconID).Return(asIdentity(me))
				store.On("GetCurrent", beaconID).Return(NewFreshState(beaconID), nil)
			},
			expectedError:            ThresholdHigherThanNodeCount,
			expectedNetworkCallCount: 0,
		},
		{
			name: "any network call failure returns an error and attempts an abort",
			proposal: drand.FirstProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				Threshold:            1,
				PeriodSeconds:        10,
				Scheme:               "bls-pedersen-chained",
				CatchupPeriodSeconds: 10,
				GenesisTime:          timestamppb.New(time.Now()),
				GenesisSeed:          []byte("cafebabe"),
				Joining:              []*drand.Participant{me, anotherParticipant},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.FirstProposalOptions, expectedError error) {
				identityProvider.On("IdentityFor", beaconID).Return(asIdentity(me))
				store.On("GetCurrent", beaconID).Return(NewFreshState(beaconID), nil)
				network.On("Send", me, proposal.Joining, mock.Anything).Return(expectedError)
			},
			expectedError:            errors.New("some mysterious network error"),
			expectedNetworkCallCount: 2, // 1 to send the packet, 1 to abort
		},
		{
			name: "error in saving the state after successful network propagation returns error and attempts rollback",
			proposal: drand.FirstProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				Threshold:            1,
				PeriodSeconds:        10,
				Scheme:               "bls-pedersen-chained",
				CatchupPeriodSeconds: 10,
				GenesisTime:          timestamppb.New(time.Now()),
				GenesisSeed:          []byte("cafebabe"),
				Joining:              []*drand.Participant{me, anotherParticipant},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.FirstProposalOptions, expectedError error) {
				identityProvider.On("IdentityFor", beaconID).Return(asIdentity(me))
				store.On("GetCurrent", beaconID).Return(NewFreshState(beaconID), nil)
				network.On("Send", me, proposal.Joining, mock.Anything).Return(nil)
				store.On("SaveCurrent", beaconID, mock.Anything).Return(expectedError)
			},
			expectedError:            errors.New("some database error"),
			expectedNetworkCallCount: 2, // 1 to send the packet, 1 to abort
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identityProvider := MockIdentityProvider{}
			store := MockStore{}
			network := MockNetwork{}
			process := DKGProcess{
				beaconIdentifier: &identityProvider,
				network:          &network,
				store:            &store,
				log:              log.NewLogger(nil, log.LogDebug),
			}

			test.prepareMocks(&identityProvider, &store, &network, &test.proposal, test.expectedError)

			response, err := process.StartNetwork(context.Background(), &test.proposal)

			if test.expectedError == nil {
				require.NoError(t, err)
				require.False(t, response.IsError)
			} else {
				require.Error(t, err)
				require.ErrorIs(t, err, test.expectedError)
				require.True(t, response.IsError)
				require.Equal(t, test.expectedError.Error(), response.ErrorMessage)
			}

			// we only expect a single send call, because rollback shouldn't be triggered
			network.AssertNumberOfCalls(t, "Send", test.expectedNetworkCallCount)
		})
	}
}

type MockIdentityProvider struct {
	mock.Mock
}

func (d *MockIdentityProvider) IdentityFor(beaconID string) (*key.Identity, error) {
	args := d.Called(beaconID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*key.Identity), args.Error(1)
}

func asIdentity(d *drand.Participant) (*key.Identity, error) {
	public := key.KeyGroup.Point()
	if err := public.UnmarshalBinary(d.PubKey); err != nil {
		return nil, err
	}

	return &key.Identity{
		Addr:      d.Address,
		Key:       public,
		TLS:       d.Tls,
		Signature: nil,
	}, nil
}

type MockNetwork struct {
	mock.Mock
}

func (n *MockNetwork) Send(from *drand.Participant, to []*drand.Participant, action func(client drand.DKGClient) (*drand.GenericResponseMessage, error)) error {
	args := n.Called(from, to, action)
	return args.Error(0)
}

type MockStore struct {
	mock.Mock
}

func (m *MockStore) GetCurrent(beaconID string) (*DKGState, error) {
	args := m.Called(beaconID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DKGState), args.Error(1)
}

func (m *MockStore) GetFinished(beaconID string) (*DKGState, error) {
	args := m.Called(beaconID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DKGState), args.Error(1)
}

func (m *MockStore) SaveCurrent(beaconID string, state *DKGState) error {
	args := m.Called(beaconID, state)
	return args.Error(0)
}

func (m *MockStore) SaveFinished(beaconID string, state *DKGState) error {
	args := m.Called(beaconID, state)
	return args.Error(0)
}

func (m *MockStore) Close() error {
	args := m.Called("Close")
	return args.Error(0)
}
