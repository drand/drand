//nolint:lll,dupl
package dkg

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/drand/drand/net"

	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/util"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

//nolint:funlen
func TestStartNetwork(t *testing.T) {
	myKeypair := key.NewKeyPair("somebody.com")
	alice, err := util.PublicKeyAsParticipant(myKeypair.Public)
	require.NoError(t, err)

	bob := NewParticipant("bob")
	beaconID := "someBeaconID"

	tests := []struct {
		name                     string
		proposal                 *drand.FirstProposalOptions
		prepareMocks             func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.FirstProposalOptions, expectedError error)
		expectedError            error
		expectedNetworkCallCount int
	}{
		{
			name: "valid proposal is stored and does not attempt rollback",
			proposal: &drand.FirstProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				Threshold:            1,
				PeriodSeconds:        10,
				Scheme:               "pedersen-bls-chained",
				CatchupPeriodSeconds: 10,
				GenesisTime:          timestamppb.New(time.Now()),
				Joining:              []*drand.Participant{alice, bob},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.FirstProposalOptions, expectedError error) {
				identityProvider.On("KeypairFor", beaconID).Return(myKeypair, nil)
				store.On("GetCurrent", beaconID).Return(NewFreshState(beaconID), nil)
				network.On("Send", alice, proposal.Joining, mock.Anything).Return(nil).Once()
				store.On("SaveCurrent", beaconID, mock.Anything).Return(nil)
			},
			expectedError:            nil,
			expectedNetworkCallCount: 1, // no rollback
		},
		{
			name: "error fetching identity is propagated and network is not called",
			proposal: &drand.FirstProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				Threshold:            1,
				PeriodSeconds:        10,
				Scheme:               "pedersen-bls-chained",
				CatchupPeriodSeconds: 10,
				GenesisTime:          timestamppb.New(time.Now()),
				Joining:              []*drand.Participant{alice, bob},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.FirstProposalOptions, expectedError error) {
				identityProvider.On("KeypairFor", beaconID).Return(nil, expectedError)
			},
			expectedError:            errors.New("expected identity error"),
			expectedNetworkCallCount: 0,
		},
		{
			name: "error fetching the latest DKG state is propagated and network not called",
			proposal: &drand.FirstProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				Threshold:            1,
				PeriodSeconds:        10,
				Scheme:               "pedersen-bls-chained",
				CatchupPeriodSeconds: 10,
				GenesisTime:          timestamppb.New(time.Now()),
				Joining:              []*drand.Participant{alice, bob},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.FirstProposalOptions, expectedError error) {
				identityProvider.On("KeypairFor", beaconID).Return(myKeypair, nil)
				store.On("GetCurrent", beaconID).Return(nil, expectedError)
			},
			expectedError:            errors.New("expected database error"),
			expectedNetworkCallCount: 0,
		},
		{
			name: "invalid proposal propagates error and network not called",
			proposal: &drand.FirstProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				Threshold:            5, // the threshold is higher than the node count
				PeriodSeconds:        10,
				Scheme:               "pedersen-bls-chained",
				CatchupPeriodSeconds: 10,
				GenesisTime:          timestamppb.New(time.Now()),
				Joining:              []*drand.Participant{alice, bob},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.FirstProposalOptions, expectedError error) {
				identityProvider.On("KeypairFor", beaconID).Return(myKeypair, nil)
				store.On("GetCurrent", beaconID).Return(NewFreshState(beaconID), nil)
			},
			expectedError:            ErrThresholdHigherThanNodeCount,
			expectedNetworkCallCount: 0,
		},
		{
			name: "any network call failure returns an error and attempts an abort",
			proposal: &drand.FirstProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				Threshold:            1,
				PeriodSeconds:        10,
				Scheme:               "pedersen-bls-chained",
				CatchupPeriodSeconds: 10,
				GenesisTime:          timestamppb.New(time.Now()),
				Joining:              []*drand.Participant{alice, bob},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.FirstProposalOptions, expectedError error) {
				identityProvider.On("KeypairFor", beaconID).Return(myKeypair, nil)
				store.On("GetCurrent", beaconID).Return(NewFreshState(beaconID), nil)
				network.On("Send", alice, proposal.Joining, mock.Anything).Return(expectedError)
			},
			expectedError:            errors.New("some mysterious network error"),
			expectedNetworkCallCount: 2, // 1 to send the packet, 1 to abort
		},
		{
			name: "error in saving the state after successful network propagation returns error and attempts rollback",
			proposal: &drand.FirstProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				Threshold:            1,
				PeriodSeconds:        10,
				Scheme:               "pedersen-bls-chained",
				CatchupPeriodSeconds: 10,
				GenesisTime:          timestamppb.New(time.Now()),
				Joining:              []*drand.Participant{alice, bob},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.FirstProposalOptions, expectedError error) {
				identityProvider.On("KeypairFor", beaconID).Return(myKeypair, nil)
				store.On("GetCurrent", beaconID).Return(NewFreshState(beaconID), nil)
				network.On("Send", alice, proposal.Joining, mock.Anything).Return(nil)
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
				config:           Config{},
			}

			test.prepareMocks(&identityProvider, &store, &network, test.proposal, test.expectedError)

			_, err := process.StartNetwork(context.Background(), test.proposal)

			if test.expectedError == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}

			// we only expect a single send call, because rollback shouldn't be triggered
			network.AssertNumberOfCalls(t, "Send", test.expectedNetworkCallCount)
		})
	}
}

//nolint:funlen
func TestStartProposal(t *testing.T) {
	myKeypair := key.NewKeyPair("somebody.com")
	alice, err := util.PublicKeyAsParticipant(myKeypair.Public)
	require.NoError(t, err)
	bob := NewParticipant("bob")
	beaconID := "someBeaconID"
	startState := DBState{
		BeaconID:      beaconID,
		Epoch:         1,
		State:         Complete,
		Threshold:     1,
		Timeout:       time.Now(),
		SchemeID:      "pedersen-bls-chained",
		CatchupPeriod: 10,
		BeaconPeriod:  10,
		Leader:        alice,
		Remaining:     nil,
		Joining:       []*drand.Participant{alice},
		Leaving:       nil,
		Acceptors:     []*drand.Participant{alice},
		Rejectors:     nil,
		FinalGroup:    &key.Group{},
	}

	tests := []struct {
		name                     string
		proposal                 *drand.ProposalOptions
		prepareMocks             func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.ProposalOptions, expectedError error)
		expectedError            error
		expectedNetworkCallCount int
	}{
		{
			name: "valid proposal is stored and does not attempt rollback",
			proposal: &drand.ProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				TransitionTime:       timestamppb.New(time.Now().Add(10 * time.Second)),
				Threshold:            1,
				CatchupPeriodSeconds: 10,
				Joining:              []*drand.Participant{bob},
				Remaining:            []*drand.Participant{alice},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.ProposalOptions, expectedError error) {
				allParticipants := util.Concat(proposal.Joining, proposal.Remaining)
				identityProvider.On("KeypairFor", beaconID).Return(myKeypair, nil)
				store.On("GetFinished", beaconID).Return(&startState, nil)
				network.On("Send", alice, allParticipants, mock.Anything).Return(nil)
				store.On("SaveCurrent", beaconID, mock.Anything).Return(nil)
			},
			expectedError:            nil,
			expectedNetworkCallCount: 1, // no rollback
		},
		{
			name: "error fetching identity is propagated and network is not called",
			proposal: &drand.ProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				Threshold:            1,
				CatchupPeriodSeconds: 10,
				Joining:              []*drand.Participant{bob},
				Remaining:            []*drand.Participant{alice},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.ProposalOptions, expectedError error) {
				identityProvider.On("KeypairFor", beaconID).Return(nil, expectedError)
			},
			expectedError:            errors.New("some identity error"),
			expectedNetworkCallCount: 0,
		},
		{
			name: "error fetching the latest DKG state is propagated and network not called",
			proposal: &drand.ProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				Threshold:            1,
				CatchupPeriodSeconds: 10,
				Joining:              []*drand.Participant{bob},
				Remaining:            []*drand.Participant{alice},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.ProposalOptions, expectedError error) {
				identityProvider.On("KeypairFor", beaconID).Return(myKeypair, nil)
				store.On("GetFinished", beaconID).Return(nil, expectedError)
			},
			expectedError:            errors.New("some database error"),
			expectedNetworkCallCount: 0,
		},
		{
			name: "invalid proposal propagates error and network not called",
			proposal: &drand.ProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				Threshold:            5, // threshold higher than node count
				CatchupPeriodSeconds: 10,
				Joining:              []*drand.Participant{bob},
				Remaining:            []*drand.Participant{alice},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.ProposalOptions, expectedError error) {
				allParticipants := util.Concat(proposal.Joining, proposal.Remaining)
				identityProvider.On("KeypairFor", beaconID).Return(myKeypair, nil)
				store.On("GetFinished", beaconID).Return(&startState, nil)
				network.On("Send", alice, allParticipants, mock.Anything).Return(nil)
				store.On("SaveCurrent", beaconID, mock.Anything).Return(nil)
			},
			expectedError:            ErrThresholdHigherThanNodeCount,
			expectedNetworkCallCount: 0,
		},
		{
			name: "any network call failure returns an error and attempts an abort",
			proposal: &drand.ProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				TransitionTime:       timestamppb.New(time.Now().Add(10 * time.Second)),
				Threshold:            1,
				CatchupPeriodSeconds: 10,
				Joining:              []*drand.Participant{bob},
				Remaining:            []*drand.Participant{alice},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.ProposalOptions, expectedError error) {
				allParticipants := util.Concat(proposal.Joining, proposal.Remaining)
				identityProvider.On("KeypairFor", beaconID).Return(myKeypair, nil)
				store.On("GetFinished", beaconID).Return(&startState, nil)
				network.On("Send", alice, allParticipants, mock.Anything).Return(expectedError)
			},
			expectedError:            errors.New("some network error"),
			expectedNetworkCallCount: 2, // attempts a rollback
		},
		{
			name: "error in saving the state after successful network propagation returns error and attempts rollback",
			proposal: &drand.ProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				TransitionTime:       timestamppb.New(time.Now().Add(10 * time.Second)),
				Threshold:            1,
				CatchupPeriodSeconds: 10,
				Joining:              []*drand.Participant{bob},
				Remaining:            []*drand.Participant{alice},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.ProposalOptions, expectedError error) {
				allParticipants := util.Concat(proposal.Joining, proposal.Remaining)
				identityProvider.On("KeypairFor", beaconID).Return(myKeypair, nil)
				store.On("GetFinished", beaconID).Return(&startState, nil)
				network.On("Send", alice, allParticipants, mock.Anything).Return(nil)
				store.On("SaveCurrent", beaconID, mock.Anything).Return(expectedError)
			},
			expectedError:            errors.New("some database error"),
			expectedNetworkCallCount: 2, // attempts rollback
		},
		{
			name: "error signaling to leavers of the proposal does not attempt rollback",
			proposal: &drand.ProposalOptions{
				BeaconID:             beaconID,
				Timeout:              timestamppb.New(time.Now().Add(1 * time.Hour)),
				TransitionTime:       timestamppb.New(time.Now().Add(10 * time.Second)),
				Threshold:            1,
				CatchupPeriodSeconds: 10,
				Joining:              []*drand.Participant{bob},
				Remaining:            []*drand.Participant{alice},
				Leaving:              []*drand.Participant{carol},
			},
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork, proposal *drand.ProposalOptions, expectedError error) {
				startState := DBState{
					BeaconID:      beaconID,
					Epoch:         1,
					State:         Complete,
					Threshold:     1,
					Timeout:       time.Now(),
					SchemeID:      "pedersen-bls-chained",
					CatchupPeriod: 10,
					BeaconPeriod:  10,
					Leader:        alice,
					Remaining:     nil,
					Joining:       []*drand.Participant{alice, carol},
					Leaving:       nil,
					Acceptors:     []*drand.Participant{alice, carol},
					Rejectors:     nil,
					FinalGroup:    &key.Group{},
				}
				resharers := util.Concat(proposal.Joining, proposal.Remaining)
				leavers := []*drand.Participant{carol}
				identityProvider.On("KeypairFor", beaconID).Return(myKeypair, nil)
				store.On("GetFinished", beaconID).Return(&startState, nil)
				network.On("Send", alice, resharers, mock.Anything).Return(nil)
				network.On("Send", alice, leavers, mock.Anything).Return(errors.New("carol is offline"))
				store.On("SaveCurrent", beaconID, mock.Anything).Return(nil)
			},
			expectedError:            nil,
			expectedNetworkCallCount: 2, // 1 for resharers, 1 for carol, but no rollback
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
				config:           Config{},
			}

			test.prepareMocks(&identityProvider, &store, &network, test.proposal, test.expectedError)

			_, err := process.StartProposal(context.Background(), test.proposal)

			if test.expectedError == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}

			// we only expect a single send call, because rollback shouldn't be triggered
			network.AssertNumberOfCalls(t, "Send", test.expectedNetworkCallCount)
		})
	}
}

func TestAbort(t *testing.T) {
	myKeypair := key.NewKeyPair("somebody.com")
	alice, err := util.PublicKeyAsParticipant(myKeypair.Public)
	require.NoError(t, err)
	bob := NewParticipant("bob")
	beaconID := "someBeaconID"
	startState := DBState{
		BeaconID:      beaconID,
		Epoch:         2,
		State:         Proposing,
		Threshold:     1,
		Timeout:       time.Now(),
		SchemeID:      "pedersen-bls-chained",
		CatchupPeriod: 10,
		BeaconPeriod:  10,
		Leader:        alice,
		Remaining:     []*drand.Participant{alice, bob},
		Joining:       nil,
		Leaving:       nil,
		Acceptors:     nil,
		Rejectors:     nil,
		FinalGroup:    &key.Group{},
	}

	tests := []struct {
		name                     string
		prepareMocks             func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork)
		expectedError            error
		expectedNetworkCallCount int
	}{
		{
			name: "leader can trigger abort",
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork) {
				identityProvider.On("KeypairFor", beaconID).Return(myKeypair, nil)
				store.On("GetCurrent", beaconID).Return(&startState, nil)
				network.On("Send", alice, startState.Remaining, mock.Anything).Return(nil)
				store.On("SaveCurrent", beaconID, mock.Anything).Return(nil)
			},
			expectedError:            nil,
			expectedNetworkCallCount: 1,
		},
		{
			name: "non-leader triggering abort returns an error",
			prepareMocks: func(identityProvider *MockIdentityProvider, store *MockStore, network *MockNetwork) {
				identityProvider.On("KeypairFor", beaconID).Return(myKeypair, nil)

				startState.Leader = bob
				store.On("GetCurrent", beaconID).Return(&startState, nil)
			},
			expectedError:            errors.New("cannot abort the DKG if you aren't the leader"),
			expectedNetworkCallCount: 0,
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
				config:           Config{},
			}

			test.prepareMocks(&identityProvider, &store, &network)

			_, err := process.StartAbort(context.Background(), &drand.AbortOptions{BeaconID: beaconID})

			if test.expectedError == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}

			// we only expect a single send call, because rollback shouldn't be triggered
			network.AssertNumberOfCalls(t, "Send", test.expectedNetworkCallCount)
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

type MockNetwork struct {
	mock.Mock
}

func (n *MockNetwork) Send(from *drand.Participant, to []*drand.Participant, action func(client net.DKGClient, peer net.Peer) (*drand.EmptyResponse, error)) error {
	args := n.Called(from, to, action)
	return args.Error(0)
}
func (n *MockNetwork) SendIgnoringConnectionError(from *drand.Participant, to []*drand.Participant, action func(client net.DKGClient, peer net.Peer) (*drand.EmptyResponse, error)) error {
	args := n.Called(from, to, action)
	return args.Error(0)
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
