//nolint:lll,dupl,funlen,maintidx
package dkg

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/drand/drand/common/key"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/internal/util"
	"github.com/drand/drand/protobuf/drand"
)

// alice, bob and carol are the actors for all the tests
// alice acts as the leader in tests where necessary
var alice = NewParticipant("alice")
var bob = NewParticipant("bob")
var carol = NewParticipant("carol")

func TestProposalValidation(t *testing.T) {
	beaconID := "some-wonderful-beacon-id"
	current := NewCompleteDKGEntry(t, beaconID, Complete, alice)
	tests := []struct {
		name     string
		state    *DBState
		terms    *drand.ProposalTerms
		expected error
	}{
		{
			name:  "valid proposal returns no error",
			state: current,
			terms: func() *drand.ProposalTerms {
				proposal := NewValidProposal(beaconID, 2, alice)
				proposal.Leader = current.Leader
				proposal.Remaining = []*drand.Participant{
					current.Leader,
				}
				return proposal
			}(),
			expected: nil,
		},
		{
			name:  "timeout in the past returns error",
			state: current,
			terms: func() *drand.ProposalTerms {
				proposal := NewValidProposal(beaconID, 2, alice)
				proposal.Timeout = timestamppb.New(time.Now().Add(-10 * time.Hour))
				return proposal
			}(),
			expected: ErrTimeoutReached,
		},
		{
			name:     "non-matching beaconID returns error",
			state:    current,
			terms:    NewValidProposal("some other beacon ID", 2, alice),
			expected: ErrInvalidBeaconID,
		},
		{
			name:  "epoch 0 returns an error",
			state: current,
			terms: func() *drand.ProposalTerms {
				proposal := NewValidProposal(beaconID, 2, alice)
				proposal.Epoch = 0
				return proposal
			}(),
			expected: ErrInvalidEpoch,
		},
		{
			name:  "if epoch is 1, nodes remaining returns an error",
			state: NewFreshState(beaconID),
			terms: func() *drand.ProposalTerms {
				proposal := NewInitialProposal(beaconID, alice)
				proposal.Remaining = []*drand.Participant{
					{
						Address: "somebody.com",
					},
				}
				return proposal
			}(),
			expected: ErrOnlyJoinersAllowedForFirstEpoch,
		},
		{
			name:  "if epoch is 1, nodes leaving returns an error",
			state: NewFreshState(beaconID),
			terms: func() *drand.ProposalTerms {
				proposal := NewInitialProposal(beaconID, alice)
				proposal.Leaving = []*drand.Participant{
					{
						Address: "somebody.com",
					},
				}
				return proposal
			}(),
			expected: ErrOnlyJoinersAllowedForFirstEpoch,
		},
		{
			name:  "if epoch is 1, alice not joining returns an error",
			state: NewFreshState(beaconID),
			terms: func() *drand.ProposalTerms {
				proposal := NewInitialProposal(beaconID, alice)
				proposal.Joining = []*drand.Participant{
					{
						Address: "somebody.com",
					},
				}
				return proposal
			}(),
			expected: ErrLeaderNotJoining,
		},
		{
			name:  "if epoch is > 1, no nodes are remaining returns an error",
			state: current,
			terms: func() *drand.ProposalTerms {
				proposal := NewValidProposal(beaconID, 2, alice)
				proposal.Epoch = 2
				proposal.Joining = []*drand.Participant{
					{Address: "some-joining-node"},
				}
				proposal.Remaining = nil
				return proposal
			}(),
			expected: ErrNoNodesRemaining,
		},
		{
			name:  "if epoch is > 1, alice joining returns an error",
			state: current,
			terms: func() *drand.ProposalTerms {
				proposal := NewValidProposal(beaconID, 2, alice)
				proposal.Epoch = 2
				proposal.Joining = []*drand.Participant{
					proposal.Leader,
				}
				return proposal
			}(),
			expected: ErrLeaderCantJoinAfterFirstEpoch,
		},
		{
			name:  "if epoch is > 1, alice leaving returns an error",
			state: current,
			terms: func() *drand.ProposalTerms {
				proposal := NewValidProposal(beaconID, 2, alice)
				proposal.Leaving = []*drand.Participant{
					proposal.Leader,
				}
				return proposal
			}(),
			expected: ErrLeaderNotRemaining,
		},
		{
			name:  "if epoch is > 1, alice not remaining returns an error",
			state: current,
			terms: func() *drand.ProposalTerms {
				proposal := NewValidProposal(beaconID, 2, alice)
				proposal.Remaining = []*drand.Participant{
					{
						Address: "somebody.com",
					},
				}
				return proposal
			}(),
			expected: ErrLeaderNotRemaining,
		},
		{
			name:  "threshold lower than the number of remaining + joining nodes returns an error",
			state: current,
			terms: func() *drand.ProposalTerms {
				invalidProposal := NewValidProposal(beaconID, 2, alice)
				invalidProposal.Threshold = 2
				invalidProposal.Remaining = []*drand.Participant{}
				return invalidProposal
			}(),
			expected: ErrThresholdHigherThanNodeCount,
		},
		{
			name:  "participants remaining who weren't in the previous epoch returns an error",
			state: current,
			terms: func() *drand.ProposalTerms {
				invalidProposal := NewValidProposal(beaconID, 2, alice)
				invalidProposal.Remaining = []*drand.Participant{
					invalidProposal.Leader,
					{Address: "a node who didn't exist last time"},
				}
				return invalidProposal
			}(),
			expected: ErrRemainingAndLeavingNodesMustExistInCurrentEpoch,
		},
		{
			name:  "participants leaving who weren't in the previous epoch returns an error",
			state: current,
			terms: func() *drand.ProposalTerms {
				invalidProposal := NewValidProposal(beaconID, 2, alice)
				invalidProposal.Leaving = []*drand.Participant{
					{Address: "a node who didn't exist last time"},
				}
				return invalidProposal
			}(),
			expected: ErrRemainingAndLeavingNodesMustExistInCurrentEpoch,
		},
		{
			name: "if current status is Left, any higher epoch value is valid",
			state: func() *DBState {
				details := NewCompleteDKGEntry(t, beaconID, Left, alice)
				details.Epoch = 2
				return details
			}(),
			terms: func() *drand.ProposalTerms {
				validProposal := NewValidProposal(beaconID, 5, alice)
				validProposal.Leader = current.Leader
				validProposal.Remaining = []*drand.Participant{
					current.Leader,
				}
				return validProposal
			}(),
			expected: nil,
		},
		{
			name: "if current status is not Left, a proposed epoch of 1 higher than the previous epoch succeeds",
			state: func() *DBState {
				details := NewCompleteDKGEntry(t, beaconID, Complete, alice)
				details.Epoch = 2
				return details
			}(),
			terms: func() *drand.ProposalTerms {
				return NewValidProposal(beaconID, 3, alice)
			}(),
			expected: nil,
		},
		{
			name: "if current status is not Left, a proposed epoch of > 1 higher returns an error",
			state: func() *DBState {
				details := NewCompleteDKGEntry(t, beaconID, Complete, alice)
				return details
			}(),
			terms: func() *drand.ProposalTerms {
				return NewValidProposal(beaconID, 3, alice)
			}(),
			expected: ErrInvalidEpoch,
		},
		{
			name: "proposed epoch less than the current epoch returns an error",
			state: func() *DBState {
				details := NewCompleteDKGEntry(t, beaconID, Complete, alice)
				details.Epoch = 3
				return details
			}(),
			terms: func() *drand.ProposalTerms {
				return NewValidProposal(beaconID, 2, alice)
			}(),
			expected: ErrInvalidEpoch,
		},
		{
			name: "proposed epoch equal to the current epoch returns an error",
			state: func() *DBState {
				details := NewCompleteDKGEntry(t, beaconID, Left, alice)
				details.Epoch = 3
				return details
			}(),
			terms: func() *drand.ProposalTerms {
				return NewValidProposal(beaconID, 3, alice)
			}(),
			expected: ErrInvalidEpoch,
		},
		{
			name:     "leaving out an existing node in a proposal returns an error",
			state:    NewCompleteDKGEntry(t, beaconID, Complete, alice, bob),
			terms:    NewValidProposal(beaconID, 2, alice),
			expected: ErrMissingNodesInProposal,
		},
		{
			name:     "proposing a remainer who doesn't exist in the current epoch returns an error",
			state:    NewCompleteDKGEntry(t, beaconID, Complete, alice),
			terms:    NewValidProposal(beaconID, 2, alice, bob),
			expected: ErrRemainingAndLeavingNodesMustExistInCurrentEpoch,
		},
		{
			name:  "invalid schemes return an error",
			state: NewCompleteDKGEntry(t, beaconID, Complete, alice),
			terms: func() *drand.ProposalTerms {
				p := NewValidProposal(beaconID, 2, alice, bob)
				p.SchemeID = "something made up"
				return p
			}(),
			expected: ErrInvalidScheme,
		},
		{
			name:  "trying to change the genesis time after the first epoch returns an error",
			state: NewCompleteDKGEntry(t, beaconID, Complete, alice),
			terms: func() *drand.ProposalTerms {
				p := NewValidProposal(beaconID, 2, alice, bob)
				p.Epoch = 2
				p.GenesisTime = timestamppb.New(time.Now())
				return p
			}(),
			expected: ErrGenesisTimeNotEqual,
		},
		{
			name:  "for epoch 1, transition time not equal to genesis time returns an error",
			state: NewFreshState(beaconID),
			terms: func() *drand.ProposalTerms {
				p := NewInitialProposal(beaconID, alice, bob)
				p.GenesisSeed = nil
				p.TransitionTime = timestamppb.New(time.Now())
				return p
			}(),
			expected: ErrTransitionTimeMustBeGenesisTime,
		},
		{
			name:  "for > epoch 1, transition time must not be missing",
			state: NewCompleteDKGEntry(t, beaconID, Complete, alice),
			terms: func() *drand.ProposalTerms {
				p := NewValidProposal(beaconID, 2, alice, bob)
				p.TransitionTime = nil
				return p
			}(),
			expected: ErrTransitionTimeMissing,
		},
		{
			name:  "for > epoch 1, transition time must not be before the genesis time",
			state: NewCompleteDKGEntry(t, beaconID, Complete, alice),
			terms: func() *drand.ProposalTerms {
				p := NewValidProposal(beaconID, 2, alice, bob)
				p.TransitionTime = timestamppb.New(time.Unix(0, 0))
				return p
			}(),
			expected: ErrTransitionTimeBeforeGenesis,
		},
		{
			name:  "for the first epoch, genesis seed cannot be provided",
			state: NewFreshState(beaconID),
			terms: func() *drand.ProposalTerms {
				p := NewValidProposal(beaconID, 1, alice, bob)
				p.GenesisSeed = []byte("deadbeef")
				return p
			}(),
			expected: ErrNoGenesisSeedForFirstEpoch,
		},
		{
			name:  "for non-fresh after first epoch, genesis seed must not change",
			state: NewCompleteDKGEntry(t, beaconID, Complete, alice),
			terms: func() *drand.ProposalTerms {
				p := NewValidProposal(beaconID, 2, alice, bob)
				p.GenesisSeed = []byte("something-random")
				return p
			}(),
			expected: ErrGenesisSeedCannotChange,
		},
		{
			name:  "for fresh joining after first epoch, genesis seed must be provided but can be anything",
			state: NewFreshState(beaconID),
			terms: func() *drand.ProposalTerms {
				p := NewValidProposal(beaconID, 2, alice, bob)
				p.GenesisSeed = []byte("something-random")
				return p
			}(),
			expected: nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ValidateProposal(test.state, test.terms)
			require.Equal(t, test.expected, result, "expected %s, got %s", test.expected, result)
		})
	}
}

//nolint:funlen
func TestTimeoutCanOnlyBeCalledFromValidState(t *testing.T) {
	beaconID := "some-wonderful-beacon-id"

	tests := []stateChangeTableTest{
		{
			name:          "fresh state cannot time out",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedError: InvalidStateChange(Fresh, TimedOut),
		},
		{
			name:          "complete state cannot time out",
			startingState: NewCompleteDKGEntry(t, beaconID, Complete, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedError: InvalidStateChange(Complete, TimedOut),
		},
		{
			name:          "timed out state cannot time out",
			startingState: NewCompleteDKGEntry(t, beaconID, TimedOut, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedError: InvalidStateChange(TimedOut, TimedOut),
		},
		{
			name:          "aborted state cannot time out",
			startingState: NewCompleteDKGEntry(t, beaconID, Aborted, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedError: InvalidStateChange(Aborted, TimedOut),
		},
		{
			name:          "left state cannot time out",
			startingState: NewCompleteDKGEntry(t, beaconID, Left, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedError: InvalidStateChange(Left, TimedOut),
		},
		{
			name:          "joined state can time out and changes state",
			startingState: NewCompleteDKGEntry(t, beaconID, Joined, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedResult: NewCompleteDKGEntry(t, beaconID, TimedOut, alice),
		},
		{
			name:          "proposed state can time out and changes state",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposed, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedResult: NewCompleteDKGEntry(t, beaconID, TimedOut, alice),
		},
		{
			name:          "proposing state can time out and changes state",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposing, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedResult: NewCompleteDKGEntry(t, beaconID, TimedOut, alice),
		},
		{
			name:          "executing state cannot time out and changes state",
			startingState: NewCompleteDKGEntry(t, beaconID, Executing, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedResult: NewCompleteDKGEntry(t, beaconID, TimedOut, alice),
		},
		{
			name:          "accepted state can time out and changes state",
			startingState: NewCompleteDKGEntry(t, beaconID, Accepted, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedResult: NewCompleteDKGEntry(t, beaconID, TimedOut, alice),
		},
		{
			name:          "rejected state can time out and changes state",
			startingState: NewCompleteDKGEntry(t, beaconID, Rejected, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedResult: NewCompleteDKGEntry(t, beaconID, TimedOut, alice),
		},
	}

	RunStateChangeTest(t, tests)
}

//nolint:funlen
func TestAbortCanOnlyBeCalledFromValidState(t *testing.T) {
	beaconID := "some-wonderful-beacon-id"
	tests := []stateChangeTableTest{
		{
			name:          "fresh state cannot be aborted",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted(&drand.GossipMetadata{Address: alice.Address})
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Fresh, Aborted),
		},
		{
			name:          "complete state cannot be aborted",
			startingState: NewCompleteDKGEntry(t, beaconID, Complete, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted(&drand.GossipMetadata{Address: alice.Address})
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Complete, Aborted),
		},
		{
			name:          "timed out state cannot be aborted",
			startingState: NewCompleteDKGEntry(t, beaconID, TimedOut, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted(&drand.GossipMetadata{Address: alice.Address})
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(TimedOut, Aborted),
		},
		{
			name:          "aborted state cannot be aborted",
			startingState: NewCompleteDKGEntry(t, beaconID, Aborted, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted(&drand.GossipMetadata{Address: alice.Address})
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Aborted, Aborted),
		},
		{
			name:          "left state can be aborted and changes state",
			startingState: NewCompleteDKGEntry(t, beaconID, Left, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted(&drand.GossipMetadata{Address: alice.Address})
			},
			expectedResult: NewCompleteDKGEntry(t, beaconID, Aborted, alice),
			expectedError:  nil,
		},
		{
			name:          "joined state can be aborted and changes state",
			startingState: NewCompleteDKGEntry(t, beaconID, Joined, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted(&drand.GossipMetadata{Address: alice.Address})
			},
			expectedResult: NewCompleteDKGEntry(t, beaconID, Aborted, alice),
			expectedError:  nil,
		},
		{
			name:          "proposed state can be aborted and changes state",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposed, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted(&drand.GossipMetadata{Address: alice.Address})
			},
			expectedResult: NewCompleteDKGEntry(t, beaconID, Aborted, alice),
			expectedError:  nil,
		},
		{
			name:          "proposing state can be aborted and changes state",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposing, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted(&drand.GossipMetadata{Address: alice.Address})
			},
			expectedResult: NewCompleteDKGEntry(t, beaconID, Aborted, alice),
		},
		{
			name:          "executing state cannot be aborted",
			startingState: NewCompleteDKGEntry(t, beaconID, Executing, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted(&drand.GossipMetadata{Address: alice.Address})
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Executing, Aborted),
		},
		{
			name:          "accepted state can be aborted and changes state",
			startingState: NewCompleteDKGEntry(t, beaconID, Accepted, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted(&drand.GossipMetadata{Address: alice.Address})
			},
			expectedResult: NewCompleteDKGEntry(t, beaconID, Aborted, alice),
			expectedError:  nil,
		},
		{
			name:          "rejected state can be aborted and changes state",
			startingState: NewCompleteDKGEntry(t, beaconID, Rejected, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted(&drand.GossipMetadata{Address: alice.Address})
			},
			expectedResult: NewCompleteDKGEntry(t, beaconID, Aborted, alice),
			expectedError:  nil,
		},
		{
			name:          "non-leader cannot abort",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposing, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted(&drand.GossipMetadata{Address: bob.Address})
			},
			expectedError: ErrOnlyLeaderCanAbort,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestJoiningADKGFromProposal(t *testing.T) {
	beaconID := "some-wonderful-beacon-id"
	tests := []stateChangeTableTest{
		{
			name: "fresh state can join with a valid proposal",
			startingState: func() *DBState {
				s, _ := NewFreshState(beaconID).Proposed(bob, NewInitialProposal(beaconID, alice, bob), &drand.GossipMetadata{Address: alice.Address})
				return s
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Joined(alice, nil)
			},
			expectedResult: func() *DBState {
				proposal := NewValidProposal(beaconID, 1, alice)
				return &DBState{
					BeaconID:       beaconID,
					State:          Joined,
					Epoch:          1,
					Leader:         proposal.Leader,
					Threshold:      proposal.Threshold,
					SchemeID:       proposal.SchemeID,
					GenesisTime:    proposal.GenesisTime.AsTime(),
					TransitionTime: proposal.TransitionTime.AsTime(),
					CatchupPeriod:  time.Duration(proposal.CatchupPeriodSeconds) * time.Second,
					BeaconPeriod:   time.Duration(proposal.BeaconPeriodSeconds) * time.Second,
					Timeout:        proposal.Timeout.AsTime(),
					Remaining:      nil,
					Joining:        []*drand.Participant{alice, bob},
					Leaving:        nil,
					FinalGroup:     nil,
					KeyShare:       nil,
				}
			}(),
			expectedError: nil,
		},
		{
			name: "fresh state join fails if self not present in joining",
			startingState: func() *DBState {
				return NewCompleteDKGEntry(t, beaconID, Proposed, alice, bob)
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Joined(alice, nil)
			},
			expectedError: ErrCannotJoinIfNotInJoining,
		},
		{
			name: "joining after first epoch without group file fails",
			startingState: func() *DBState {
				entry := NewCompleteDKGEntry(t, beaconID, Proposed, bob)
				entry.Epoch = 2
				return entry
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Joined(alice, nil)
			},
			expectedError: ErrJoiningAfterFirstEpochNeedsGroupFile,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestProposingDKGFromFresh(t *testing.T) {
	beaconID := "some-wonderful-beacon-id"
	tests := []stateChangeTableTest{
		{
			name:          "Proposing a valid DKG changes state to Proposing",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(alice, NewInitialProposal(beaconID, alice))
			},
			expectedResult: func() *DBState {
				proposal := NewValidProposal(beaconID, 1, alice)
				return &DBState{
					BeaconID:       beaconID,
					Epoch:          1,
					State:          Proposing,
					Leader:         alice,
					Threshold:      proposal.Threshold,
					SchemeID:       proposal.SchemeID,
					GenesisTime:    proposal.GenesisTime.AsTime(),
					TransitionTime: proposal.TransitionTime.AsTime(),
					CatchupPeriod:  time.Duration(proposal.CatchupPeriodSeconds) * time.Second,
					BeaconPeriod:   time.Duration(proposal.BeaconPeriodSeconds) * time.Second,
					Timeout:        proposal.Timeout.AsTime(),
					Remaining:      nil,
					Joining:        []*drand.Participant{alice},
					Leaving:        nil,
					FinalGroup:     nil,
				}
			}(),
			expectedError: nil,
		},
		{
			name:          "Proposing an invalid DKG returns an error",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				invalidProposal := NewValidProposal(beaconID, 0, alice)

				return in.Proposing(alice, invalidProposal)
			},
			expectedResult: nil,
			expectedError:  ErrInvalidEpoch,
		},
		{
			name:          "Proposing a DKG as non-alice returns an error",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				someRandomPerson := &drand.Participant{
					Address: "somebody-that-isnt-me.com",
				}

				return in.Proposing(alice, NewInitialProposal(beaconID, someRandomPerson))
			},
			expectedResult: nil,
			expectedError:  ErrCannotProposeAsNonLeader,
		},
		{
			name:          "Proposing a DKG with epoch > 1 when fresh state returns an error",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(alice, NewValidProposal(beaconID, 2, alice))
			},
			expectedResult: nil,
			expectedError:  ErrInvalidEpoch,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestProposingDKGFromNonFresh(t *testing.T) {
	beaconID := "some-wonderful-beacon-id"
	tests := []stateChangeTableTest{
		{
			name:          "Proposing a valid DKG from Complete changes state to Proposing",
			startingState: NewCompleteDKGEntry(t, beaconID, Complete, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(alice, NewValidProposal(beaconID, 2, alice))
			},
			expectedResult: func() *DBState {
				proposal := NewValidProposal(beaconID, 2, alice)
				return &DBState{
					BeaconID:       beaconID,
					Epoch:          2,
					State:          Proposing,
					Threshold:      proposal.Threshold,
					SchemeID:       proposal.SchemeID,
					GenesisTime:    proposal.GenesisTime.AsTime(),
					TransitionTime: proposal.TransitionTime.AsTime(),
					GenesisSeed:    proposal.GenesisSeed,
					CatchupPeriod:  time.Duration(proposal.CatchupPeriodSeconds) * time.Second,
					BeaconPeriod:   time.Duration(proposal.BeaconPeriodSeconds) * time.Second,
					Timeout:        proposal.Timeout.AsTime(),
					Leader:         alice,
					Remaining:      proposal.Remaining,
					Joining:        nil,
					Leaving:        nil,
					FinalGroup:     nil,
				}
			}(),
			expectedError: nil,
		},
		{
			name:          "Proposing a valid DKG from Aborted changes state to Proposing",
			startingState: NewCompleteDKGEntry(t, beaconID, Aborted, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(alice, NewValidProposal(beaconID, 2, alice))
			},
			expectedResult: func() *DBState {
				proposal := NewValidProposal(beaconID, 2, alice)
				return &DBState{
					BeaconID:       beaconID,
					Epoch:          2,
					State:          Proposing,
					Threshold:      proposal.Threshold,
					SchemeID:       proposal.SchemeID,
					GenesisTime:    proposal.GenesisTime.AsTime(),
					TransitionTime: proposal.TransitionTime.AsTime(),
					GenesisSeed:    proposal.GenesisSeed,
					CatchupPeriod:  time.Duration(proposal.CatchupPeriodSeconds) * time.Second,
					BeaconPeriod:   time.Duration(proposal.BeaconPeriodSeconds) * time.Second,
					Timeout:        proposal.Timeout.AsTime(),
					Leader:         alice,
					Remaining:      proposal.Remaining,
					Joining:        nil,
					Leaving:        nil,
					FinalGroup:     nil,
				}
			}(),
			expectedError: nil,
		},
		{
			name:          "Proposing a valid DKG after Timeout changes state to Proposing",
			startingState: NewCompleteDKGEntry(t, beaconID, TimedOut, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(alice, NewValidProposal(beaconID, 2, alice))
			},
			expectedResult: func() *DBState {
				proposal := NewValidProposal(beaconID, 2, alice)
				return &DBState{
					BeaconID:       beaconID,
					Epoch:          2,
					State:          Proposing,
					Threshold:      proposal.Threshold,
					SchemeID:       proposal.SchemeID,
					GenesisTime:    proposal.GenesisTime.AsTime(),
					TransitionTime: proposal.TransitionTime.AsTime(),
					GenesisSeed:    proposal.GenesisSeed,
					CatchupPeriod:  time.Duration(proposal.CatchupPeriodSeconds) * time.Second,
					BeaconPeriod:   time.Duration(proposal.BeaconPeriodSeconds) * time.Second,
					Timeout:        proposal.Timeout.AsTime(),
					Leader:         alice,
					Remaining:      proposal.Remaining,
					Joining:        nil,
					Leaving:        nil,
					FinalGroup:     nil,
				}
			}(),
			expectedError: nil,
		},
		{
			name:          "cannot propose a DKG when already joined",
			startingState: NewCompleteDKGEntry(t, beaconID, Joined, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(alice, NewValidProposal(beaconID, 2, alice))
			},
			expectedError: InvalidStateChange(Joined, Proposing),
		},
		{
			name:          "proposing a DKG when leaving returns error",
			startingState: NewCompleteDKGEntry(t, beaconID, Left, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(alice, NewValidProposal(beaconID, 2, alice))
			},
			expectedError: InvalidStateChange(Left, Proposing),
		},
		{
			name:          "proposing a DKG when already proposing returns an error",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposing, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(alice, NewValidProposal(beaconID, 2, alice))
			},
			expectedError: InvalidStateChange(Proposing, Proposing),
		},
		{
			name:          "proposing a DKG when one has already been proposed returns an error",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposed, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(alice, NewValidProposal(beaconID, 2, alice))
			},

			expectedError: InvalidStateChange(Proposed, Proposing),
		},
		{
			name:          "proposing a DKG during execution returns an error",
			startingState: NewCompleteDKGEntry(t, beaconID, Executing, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(alice, NewValidProposal(beaconID, 2, alice))
			},
			expectedError: InvalidStateChange(Executing, Proposing),
		},
		{
			name:          "proposing a DKG after acceptance returns an error",
			startingState: NewCompleteDKGEntry(t, beaconID, Accepted, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(alice, NewValidProposal(beaconID, 2, alice))
			},
			expectedError: InvalidStateChange(Accepted, Proposing),
		},
		{
			name:          "proposing a DKG after rejection returns an error",
			startingState: NewCompleteDKGEntry(t, beaconID, Rejected, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(alice, NewValidProposal(beaconID, 2, alice))
			},
			expectedError: InvalidStateChange(Rejected, Proposing),
		},
	}

	RunStateChangeTest(t, tests)
}

//nolint:funlen
func TestProposedDKG(t *testing.T) {
	beaconID := "some-wonderful-beacon-id"
	tests := []stateChangeTableTest{
		{
			name:          "Being proposed a valid DKG from Complete changes state to Proposed",
			startingState: NewCompleteDKGEntry(t, beaconID, Complete, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				me := bob
				proposal := NewValidProposal(beaconID, 2, alice, bob)
				metadata := drand.GossipMetadata{BeaconID: beaconID, Address: alice.Address}
				return in.Proposed(me, proposal, &metadata)
			},
			expectedResult: func() *DBState {
				proposal := NewValidProposal(beaconID, 2, alice, bob)
				return &DBState{
					BeaconID:       beaconID,
					Epoch:          2,
					State:          Proposed,
					Threshold:      proposal.Threshold,
					SchemeID:       proposal.SchemeID,
					GenesisTime:    proposal.GenesisTime.AsTime(),
					TransitionTime: proposal.TransitionTime.AsTime(),
					GenesisSeed:    proposal.GenesisSeed,
					CatchupPeriod:  time.Duration(proposal.CatchupPeriodSeconds) * time.Second,
					BeaconPeriod:   time.Duration(proposal.BeaconPeriodSeconds) * time.Second,
					Timeout:        proposal.Timeout.AsTime(),
					Leader:         alice,
					Remaining:      proposal.Remaining,
					Joining:        proposal.Joining,
					Leaving:        proposal.Leaving,
					FinalGroup:     nil,
				}
			}(),
			expectedError: nil,
		},
		{
			name:          "Being proposed a valid DKG with epoch 1 from Fresh state changes state to Proposed",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				me := bob
				proposal := NewInitialProposal(beaconID, alice, bob)
				metadata := drand.GossipMetadata{BeaconID: beaconID, Address: alice.Address}
				return in.Proposed(me, proposal, &metadata)
			},
			expectedResult: func() *DBState {
				proposal := NewInitialProposal(beaconID, alice, bob)
				return &DBState{
					BeaconID:       beaconID,
					Epoch:          1,
					State:          Proposed,
					Threshold:      proposal.Threshold,
					SchemeID:       proposal.SchemeID,
					GenesisTime:    proposal.GenesisTime.AsTime(),
					TransitionTime: proposal.TransitionTime.AsTime(),
					GenesisSeed:    proposal.GenesisSeed,
					CatchupPeriod:  time.Duration(proposal.CatchupPeriodSeconds) * time.Second,
					BeaconPeriod:   time.Duration(proposal.BeaconPeriodSeconds) * time.Second,
					Timeout:        proposal.Timeout.AsTime(),
					Leader:         alice,
					Remaining:      proposal.Remaining,
					Joining:        proposal.Joining,
					Leaving:        proposal.Leaving,
					FinalGroup:     nil,
				}
			}(),
			expectedError: nil,
		},
		{
			name:          "Being proposed a valid DKG but without me included in some way returns an error",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				me := bob
				proposal := NewInitialProposal(beaconID, alice)
				metadata := drand.GossipMetadata{BeaconID: beaconID, Address: alice.Address}
				return in.Proposed(me, proposal, &metadata)
			},
			expectedError: ErrSelfMissingFromProposal,
		},
		{
			name:          "Being proposed a valid DKG with epoch > 1 from Fresh state changes state to Proposed",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				me := bob
				proposal := NewValidProposal(beaconID, 2, alice, bob)
				metadata := drand.GossipMetadata{BeaconID: beaconID, Address: alice.Address}
				return in.Proposed(me, proposal, &metadata)
			},
			expectedResult: func() *DBState {
				proposal := NewValidProposal(beaconID, 2, alice, bob)
				return &DBState{
					BeaconID:       beaconID,
					Epoch:          proposal.Epoch,
					State:          Proposed,
					Threshold:      proposal.Threshold,
					SchemeID:       proposal.SchemeID,
					GenesisTime:    proposal.GenesisTime.AsTime(),
					TransitionTime: proposal.TransitionTime.AsTime(),
					GenesisSeed:    proposal.GenesisSeed,
					CatchupPeriod:  time.Duration(proposal.CatchupPeriodSeconds) * time.Second,
					BeaconPeriod:   time.Duration(proposal.BeaconPeriodSeconds) * time.Second,
					Timeout:        proposal.Timeout.AsTime(),
					Leader:         alice,
					Remaining:      proposal.Remaining,
					Joining:        proposal.Joining,
					Leaving:        nil,
					FinalGroup:     nil,
				}
			}(),
			expectedError: nil,
		},
		{
			name:          "Being proposed a valid DKG from state Executing returns an error",
			startingState: NewCompleteDKGEntry(t, beaconID, Executing, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				me := bob
				proposal := NewValidProposal(beaconID, 2, alice, bob)
				metadata := drand.GossipMetadata{BeaconID: beaconID, Address: alice.Address}
				return in.Proposed(me, proposal, &metadata)
			},
			expectedError: InvalidStateChange(Executing, Proposed),
		},
		{
			name:          "Being proposed a DKG by somebody who isn't the alice returns an error",
			startingState: NewCompleteDKGEntry(t, beaconID, Aborted, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				me := bob
				proposal := NewValidProposal(beaconID, 2, alice, bob)
				metadata := drand.GossipMetadata{BeaconID: beaconID, Address: carol.Address}
				return in.Proposed(me, proposal, &metadata)
			},
			expectedError: ErrCannotProposeAsNonLeader,
		},
		{
			name:          "Being proposed an otherwise invalid DKG returns an error",
			startingState: NewCompleteDKGEntry(t, beaconID, Aborted, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				me := bob
				proposal := NewValidProposal(beaconID, 0, alice)
				metadata := drand.GossipMetadata{BeaconID: beaconID, Address: alice.Address}
				return in.Proposed(me, proposal, &metadata)
			},
			expectedError: ErrInvalidEpoch,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestAcceptingDKG(t *testing.T) {
	beaconID := "some-wonderful-beacon-id"
	tests := []stateChangeTableTest{
		{
			name:          "valid proposal can be accepted",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposed, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Accepted(bob)
			},
			expectedResult: NewCompleteDKGEntry(t, beaconID, Accepted, alice, bob),
			expectedError:  nil,
		},
		{
			name:          "cannot accept a fresh proposal",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Accepted(alice)
			},
			expectedError: InvalidStateChange(Fresh, Accepted),
		},
		{
			name:          "cannot accept own proposal",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposing, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Accepted(alice)
			},
			expectedError: InvalidStateChange(Proposing, Accepted),
		},
		{
			name:          "cannot accept a proposal i've already rejected",
			startingState: NewCompleteDKGEntry(t, beaconID, Rejected, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Accepted(alice)
			},
			expectedError: InvalidStateChange(Rejected, Accepted),
		},
		{
			name:          "cannot accept a proposal that has already timed out",
			startingState: PastTimeout(NewCompleteDKGEntry(t, beaconID, Proposed, alice)),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Accepted(alice)
			},
			expectedError: ErrTimeoutReached,
		},
		{
			name: "cannot accept a proposal where I am leaving",
			startingState: func() *DBState {
				details := NewCompleteDKGEntry(t, beaconID, Proposed, alice)
				details.Leaving = []*drand.Participant{alice}
				return details
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Accepted(alice)
			},
			expectedError: ErrCannotAcceptProposalWhereLeaving,
		},
		{
			name: "cannot accept a proposal where I am joining",
			startingState: func() *DBState {
				details := NewCompleteDKGEntry(t, beaconID, Proposed, alice)
				details.Joining = []*drand.Participant{alice}
				return details
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Accepted(alice)
			},
			expectedError: ErrCannotAcceptProposalWhereJoining,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestRejectingDKG(t *testing.T) {
	beaconID := "some-wonderful-beacon-id"

	tests := []stateChangeTableTest{
		{
			name:          "valid proposal can be rejected",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposed, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Rejected(alice)
			},
			expectedResult: NewCompleteDKGEntry(t, beaconID, Rejected, alice, bob),
			expectedError:  nil,
		},
		{
			name:          "cannot reject a fresh proposal",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Rejected(alice)
			},
			expectedError: InvalidStateChange(Fresh, Rejected),
		},
		{
			name:          "cannot reject own proposal",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposing, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Rejected(alice)
			},
			expectedError: InvalidStateChange(Proposing, Rejected),
		},
		{
			name:          "cannot rejected a proposal i've already accepted",
			startingState: NewCompleteDKGEntry(t, beaconID, Accepted, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Rejected(alice)
			},
			expectedError: InvalidStateChange(Accepted, Rejected),
		},
		{
			name:          "cannot reject a proposal that has already timed out",
			startingState: PastTimeout(NewCompleteDKGEntry(t, beaconID, Proposed, alice)),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Rejected(alice)
			},
			expectedError: ErrTimeoutReached,
		},
		{
			name: "cannot reject a proposal where I am leaving",
			startingState: func() *DBState {
				details := NewCompleteDKGEntry(t, beaconID, Proposed, alice)
				details.Leaving = []*drand.Participant{alice}
				return details
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Rejected(alice)
			},
			expectedError: ErrCannotRejectProposalWhereLeaving,
		},
		{
			name: "cannot reject a proposal where I am joining",
			startingState: func() *DBState {
				details := NewCompleteDKGEntry(t, beaconID, Proposed, alice)
				details.Joining = []*drand.Participant{alice}
				return details
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Rejected(alice)
			},
			expectedError: ErrCannotRejectProposalWhereJoining,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestLeftDKG(t *testing.T) {
	beaconID := "some-wonderful-beacon-id"

	tests := []stateChangeTableTest{
		{
			name: "can leave valid proposal that contains me as a leaver",
			startingState: func() *DBState {
				details := NewCompleteDKGEntry(t, beaconID, Proposed, alice)
				details.Leaving = []*drand.Participant{alice}
				return details
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Left(alice)
			},
			expectedResult: func() *DBState {
				details := NewCompleteDKGEntry(t, beaconID, Left, alice)
				details.Leaving = []*drand.Participant{alice}
				return details
			}(),
			expectedError: nil,
		},
		{
			name: "can leave valid proposal immediately if I have just joined it",
			startingState: func() *DBState {
				details := NewCompleteDKGEntry(t, beaconID, Joined, alice)
				details.Joining = []*drand.Participant{alice}
				return details
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Left(alice)
			},
			expectedResult: func() *DBState {
				details := NewCompleteDKGEntry(t, beaconID, Left, alice)
				details.Joining = []*drand.Participant{alice}
				return details
			}(),
			expectedError: nil,
		},
		{
			name:          "trying to leave if not a leaver returns an error",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposed, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Left(bob)
			},
			expectedError: ErrCannotLeaveIfNotALeaver,
		},
		{
			name: "attempting to leave if timeout has been reached returns an error",
			startingState: func() *DBState {
				details := PastTimeout(NewCompleteDKGEntry(t, beaconID, Proposed, alice, bob))
				details.Leaving = []*drand.Participant{bob}
				return details
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Left(bob)
			},
			expectedError: ErrTimeoutReached,
		},
		{
			name: "cannot leave if proposal already complete",
			startingState: func() *DBState {
				details := NewCompleteDKGEntry(t, beaconID, Complete, alice, bob)
				details.Leaving = []*drand.Participant{bob}
				return details
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Left(bob)
			},
			expectedError: InvalidStateChange(Complete, Left),
		},
	}

	RunStateChangeTest(t, tests)
}

func TestExecutingDKG(t *testing.T) {
	beaconID := "some-wonderful-beacon-id"
	tests := []stateChangeTableTest{
		{
			name:          "executing valid proposal that I have accepted succeeds",
			startingState: NewCompleteDKGEntry(t, beaconID, Accepted, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Executing(alice, &drand.GossipMetadata{Address: alice.Address})
			},
			expectedResult: NewCompleteDKGEntry(t, beaconID, Executing, alice, bob),
			expectedError:  nil,
		},
		{
			name:          "executing a valid proposal that I have rejected returns an error",
			startingState: NewCompleteDKGEntry(t, beaconID, Rejected, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Executing(alice, &drand.GossipMetadata{Address: alice.Address})
			},
			expectedError: InvalidStateChange(Rejected, Executing),
		},
		{
			name:          "executing a proposal after time out returns an error",
			startingState: PastTimeout(NewCompleteDKGEntry(t, beaconID, Accepted, alice, bob)),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Executing(alice, &drand.GossipMetadata{Address: alice.Address})
			},
			expectedError: ErrTimeoutReached,
		},
		{
			name:          "executing a valid proposal that I am not joining or remaining in returns an error (but shouldn't have been possible anyway)",
			startingState: NewCompleteDKGEntry(t, beaconID, Accepted, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Executing(alice, &drand.GossipMetadata{Address: bob.Address})
			},
			expectedError: ErrCannotExecuteIfNotJoinerOrRemainer,
		},
		{
			name: "executing as a leaver transitions me to Left",
			startingState: func() *DBState {
				state := NewCompleteDKGEntry(t, beaconID, Proposed, alice, bob)
				state.Leaving = append(state.Leaving, bob)
				return state
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Executing(bob, &drand.GossipMetadata{Address: alice.Address})
			},
			expectedResult: func() *DBState {
				state := NewCompleteDKGEntry(t, beaconID, Left, alice, bob)
				state.Leaving = append(state.Leaving, bob)
				return state
			}(),
		},
		{
			name:          "a non-leader node attempting to execute the proposal returns an error",
			startingState: NewCompleteDKGEntry(t, beaconID, Accepted, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Executing(bob, &drand.GossipMetadata{Address: bob.Address})
			},
			expectedError: ErrOnlyLeaderCanTriggerExecute,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestEviction(t *testing.T) {
	beaconID := "some-wonderful-beacon-id"
	tests := []stateChangeTableTest{
		{
			name:          "can be evicted from an executing DKG (e.g. if evicted)",
			startingState: NewCompleteDKGEntry(t, beaconID, Executing, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Evicted()
			},
			expectedError: nil,
		},
		{
			name:          "can be evicted from a timed out DKG (in case you missed the eviction)",
			startingState: NewCompleteDKGEntry(t, beaconID, TimedOut, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Evicted()
			},
			expectedError: nil,
		},
		{
			name:          "cannot be evicted from a DKG before execution",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposed, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Evicted()
			},
			expectedError: InvalidStateChange(Proposed, Evicted),
		},
	}
	RunStateChangeTest(t, tests)
}

func TestCompleteDKG(t *testing.T) {
	beaconID := "some-wonderful-beacon-id"
	finalGroup := key.Group{}
	keyShare := key.Share{}

	tests := []stateChangeTableTest{
		{
			name:          "completing a valid executing proposal succeeds",
			startingState: NewCompleteDKGEntry(t, beaconID, Executing, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Complete(&finalGroup, &keyShare)
			},
			expectedResult: func() *DBState {
				d := NewCompleteDKGEntry(t, beaconID, Complete, alice, bob)
				d.FinalGroup = &finalGroup
				d.GenesisSeed = finalGroup.GetGenesisSeed()
				d.KeyShare = &keyShare
				return d
			}(),
			expectedError: nil,
		},
		{
			name:          "completing a non-executing proposal returns an error",
			startingState: NewCompleteDKGEntry(t, beaconID, Accepted, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Complete(&finalGroup, &keyShare)
			},
			expectedError: InvalidStateChange(Accepted, Complete),
		},
		{
			name:          "completing a proposal after time out returns an error",
			startingState: PastTimeout(NewCompleteDKGEntry(t, beaconID, Executing, alice, bob)),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Complete(&finalGroup, &keyShare)
			},
			expectedError: ErrTimeoutReached,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestReceivedAcceptance(t *testing.T) {
	beaconID := "some-wonderful-beacon-id"

	tests := []stateChangeTableTest{
		{
			name:          "receiving a valid acceptance for a proposal adds it to the list of acceptors",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposing, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedAcceptance(bob, &drand.GossipMetadata{Address: bob.Address})
			},
			expectedResult: func() *DBState {
				d := NewCompleteDKGEntry(t, beaconID, Proposing, alice, bob)
				d.Acceptors = []*drand.Participant{bob}
				return d
			}(),
			expectedError: nil,
		},
		{
			name:          "receiving an acceptance for a proposal who isn't the person who makes it should error",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposing, bob, alice, carol),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedAcceptance(alice, &drand.GossipMetadata{Address: bob.Address})
			},
			expectedError: ErrInvalidAcceptor,
		},
		{
			name:          "receiving an acceptance from somebody who isn't a remainer returns an error",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposing, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				who := drand.Participant{Address: "who is this?!?"}
				return in.ReceivedAcceptance(&who, &drand.GossipMetadata{Address: who.Address})
			},
			expectedError: ErrUnknownAcceptor,
		},
		{
			name:          "receiving acceptance from non-proposing state returns an error",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposed, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedAcceptance(bob, &drand.GossipMetadata{Address: bob.Address})
			},
			expectedError: InvalidStateChange(Proposed, Proposing),
		},
		{
			name: "acceptances are appended to acceptors",
			startingState: func() *DBState {
				d := NewCompleteDKGEntry(t, beaconID, Proposing, alice, bob, carol)
				d.Acceptors = []*drand.Participant{carol}
				return d
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedAcceptance(bob, &drand.GossipMetadata{Address: bob.Address})
			},
			expectedResult: func() *DBState {
				d := NewCompleteDKGEntry(t, beaconID, Proposing, alice, bob, carol)
				d.Acceptors = []*drand.Participant{carol, bob}
				return d
			}(),
			expectedError: nil,
		},
		{
			name: "duplicate acceptance returns an error",
			startingState: func() *DBState {
				d := NewCompleteDKGEntry(t, beaconID, Proposing, alice, bob)
				d.Acceptors = []*drand.Participant{bob}
				return d
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedAcceptance(bob, &drand.GossipMetadata{Address: bob.Address})
			},
			expectedError: ErrDuplicateAcceptance,
		},
		{
			name: "if a party has rejected and they send an acceptance, they are moved into acceptance",
			startingState: func() *DBState {
				d := NewCompleteDKGEntry(t, beaconID, Proposing, alice, bob)
				d.Rejectors = []*drand.Participant{bob}
				return d
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedAcceptance(bob, &drand.GossipMetadata{Address: bob.Address})
			},
			expectedResult: func() *DBState {
				d := NewCompleteDKGEntry(t, beaconID, Proposing, alice, bob)
				d.Acceptors = []*drand.Participant{bob}
				return d
			}(),
			expectedError: nil,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestReceivedRejection(t *testing.T) {
	beaconID := "some-wonderful-beacon-id"
	tests := []stateChangeTableTest{
		{
			name:          "receiving a valid rejection for a proposal I made adds it to the list of rejectors",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposing, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedRejection(bob, &drand.GossipMetadata{Address: bob.Address})
			},
			expectedResult: func() *DBState {
				d := NewCompleteDKGEntry(t, beaconID, Proposing, alice, bob)
				d.Rejectors = []*drand.Participant{bob}
				return d
			}(),
			expectedError: nil,
		},
		{
			name:          "receiving a rejection from a person who didn't send it returns an error",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposing, bob, alice),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedRejection(bob, &drand.GossipMetadata{Address: alice.Address})
			},
			expectedError: ErrInvalidRejector,
		},
		{
			name:          "receiving a rejection from somebody who isn't a remainer returns an error",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposing, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				who := drand.Participant{Address: "who is this?!?"}
				return in.ReceivedRejection(&who, &drand.GossipMetadata{Address: who.Address})
			},
			expectedError: ErrUnknownRejector,
		},
		{
			name:          "receiving rejection from non-proposing state returns an error",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposed, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedRejection(bob, &drand.GossipMetadata{Address: bob.Address})
			},
			expectedError: InvalidStateChange(Proposed, Proposing),
		},
		{
			name: "rejections are appended to rejectors",
			startingState: func() *DBState {
				d := NewCompleteDKGEntry(t, beaconID, Proposing, alice, bob)
				d.Rejectors = []*drand.Participant{carol}
				return d
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedRejection(bob, &drand.GossipMetadata{Address: bob.Address})
			},
			expectedResult: func() *DBState {
				d := NewCompleteDKGEntry(t, beaconID, Proposing, alice, bob)
				d.Rejectors = []*drand.Participant{carol, bob}
				return d
			}(),
			expectedError: nil,
		},
		{
			name: "duplicate rejection returns an error",
			startingState: func() *DBState {
				d := NewCompleteDKGEntry(t, beaconID, Proposing, alice, bob)
				d.Rejectors = []*drand.Participant{bob}
				return d
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedRejection(bob, &drand.GossipMetadata{Address: bob.Address})
			},
			expectedError: ErrDuplicateRejection,
		},
		{
			name: "if a party has accepted and they send a rejection, they are moved into rejectors",
			startingState: func() *DBState {
				d := NewCompleteDKGEntry(t, beaconID, Proposing, alice, bob)
				d.Acceptors = []*drand.Participant{bob}
				return d
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedRejection(bob, &drand.GossipMetadata{Address: bob.Address})
			},
			expectedResult: func() *DBState {
				d := NewCompleteDKGEntry(t, beaconID, Proposing, alice, bob)
				d.Rejectors = []*drand.Participant{bob}
				return d
			}(),
			expectedError: nil,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestCompletion(t *testing.T) {
	beaconID := "some-wonderful-beacon-id"
	group := key.Group{
		GenesisSeed: []byte("deadbeef"),
	}
	keyShare := key.Share{}
	tests := []stateChangeTableTest{
		{
			name:          "receiving a valid share and group file succeeds",
			startingState: NewCompleteDKGEntry(t, beaconID, Executing, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Complete(&group, &keyShare)
			},
			expectedResult: func() *DBState {
				d := NewCompleteDKGEntry(t, beaconID, Complete, alice, bob)
				d.KeyShare = &keyShare
				d.FinalGroup = &group
				d.GenesisSeed = group.GenesisSeed
				return d
			}(),
		},
		{
			name:          "cannot complete from non-executing state",
			startingState: NewCompleteDKGEntry(t, beaconID, Proposed, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Complete(&group, &keyShare)
			},
			expectedError: InvalidStateChange(Proposed, Complete),
		},
		{
			name:          "empty group file fails",
			startingState: NewCompleteDKGEntry(t, beaconID, Executing, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Complete(nil, &keyShare)
			},
			expectedError: ErrFinalGroupCannotBeEmpty,
		},

		{
			name:          "empty key share fails",
			startingState: NewCompleteDKGEntry(t, beaconID, Executing, alice, bob),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Complete(&group, nil)
			},
			expectedError: ErrKeyShareCannotBeEmpty,
		},
	}

	RunStateChangeTest(t, tests)

}

type stateChangeTableTest struct {
	name           string
	startingState  *DBState
	expectedResult *DBState
	expectedError  error
	transitionFn   func(starting *DBState) (*DBState, error)
}

func RunStateChangeTest(t *testing.T, tests []stateChangeTableTest) {
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := test.transitionFn(test.startingState)
			require.Equal(t, test.expectedError, err, "expected %s error but got %s", test.expectedError, err)
			if test.expectedResult != nil {
				matching := test.expectedResult.Equals(result)
				require.True(t, matching)
				if !matching {
					fmt.Println("NOTE: the below comparison will show mismatching pointers, but the check actually deep equals everything:")
					require.EqualValues(t, test.expectedResult, result)
				}
			}
		})
	}
}

func NewParticipant(name string) *drand.Participant {
	sch, _ := crypto.GetSchemeFromEnv()
	k, _ := key.NewKeyPair(name, sch)
	pk, _ := k.Public.Key.MarshalBinary()
	return &drand.Participant{
		Address: name,
		Tls:     false,
		PubKey:  pk,
	}
}

// NewCompleteDKGEntry returns a full DKG state (minus some key material) for epoch 1 - consider it the result of the first DKG
func NewCompleteDKGEntry(t *testing.T, beaconID string, status Status, previousLeader *drand.Participant, others ...*drand.Participant) *DBState {
	state := DBState{
		BeaconID:       beaconID,
		Epoch:          1,
		State:          status,
		Threshold:      1,
		Timeout:        time.Unix(2549084715, 0).UTC(), // this will need updated in 2050 :^)
		SchemeID:       crypto.DefaultSchemeID,
		GenesisTime:    time.Unix(1669718523, 0).UTC(),
		GenesisSeed:    []byte("deadbeef"),
		TransitionTime: time.Unix(1669718523, 0).UTC(),
		CatchupPeriod:  5 * time.Second,
		BeaconPeriod:   10 * time.Second,

		Leader:    previousLeader,
		Remaining: append(others, previousLeader),
		Joining:   nil,
		Leaving:   nil,

		Acceptors: nil,
		Rejectors: nil,

		FinalGroup: nil,
		KeyShare:   nil,
	}
	sch, _ := crypto.GetSchemeFromEnv()
	nodes, err := util.TryMapEach[*key.Node](state.Remaining, func(index int, p *drand.Participant) (*key.Node, error) {
		n, err := util.ToKeyNode(index, p, sch)
		return &n, err
	})
	require.NoError(t, err, "error mapping participants to node")

	group := key.Group{
		Threshold:      int(state.Threshold),
		Period:         state.BeaconPeriod,
		Scheme:         sch,
		ID:             state.BeaconID,
		CatchupPeriod:  state.CatchupPeriod,
		Nodes:          nodes,
		GenesisTime:    state.GenesisTime.Unix(),
		GenesisSeed:    state.GenesisSeed,
		TransitionTime: state.TransitionTime.Unix(),
		PublicKey:      nil,
	}

	state.FinalGroup = &group

	return &state
}

func NewInitialProposal(beaconID string, leader *drand.Participant, others ...*drand.Participant) *drand.ProposalTerms {
	return &drand.ProposalTerms{
		BeaconID:             beaconID,
		Epoch:                1,
		Leader:               leader,
		Threshold:            1,
		Timeout:              timestamppb.New(time.Unix(2549084715, 0).UTC()), // this will need updated in 2050 :^)
		GenesisTime:          timestamppb.New(time.Unix(1669718523, 0).UTC()),
		TransitionTime:       timestamppb.New(time.Unix(1669718523, 0).UTC()),
		CatchupPeriodSeconds: 5,
		BeaconPeriodSeconds:  10,
		SchemeID:             crypto.DefaultSchemeID,
		Joining:              append([]*drand.Participant{leader}, others...),
	}
}

func NewValidProposal(beaconID string, epoch uint32, leader *drand.Participant, others ...*drand.Participant) *drand.ProposalTerms {
	return &drand.ProposalTerms{
		BeaconID:             beaconID,
		Epoch:                epoch,
		Leader:               leader,
		Threshold:            1,
		Timeout:              timestamppb.New(time.Unix(2549084715, 0).UTC()), // this will need updated in 2050 :^)
		GenesisTime:          timestamppb.New(time.Unix(1669718523, 0).UTC()),
		GenesisSeed:          []byte("deadbeef"),
		TransitionTime:       timestamppb.New(time.Unix(1669718523, 0).UTC()),
		CatchupPeriodSeconds: 5,
		BeaconPeriodSeconds:  10,
		SchemeID:             crypto.DefaultSchemeID,
		Remaining:            append([]*drand.Participant{leader}, others...),
	}
}

func PastTimeout(d *DBState) *DBState {
	d.Timeout = time.Now().Add(-1 * time.Minute).UTC()
	return d
}
