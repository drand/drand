//nolint:lll,dupl,funlen,maintidx
package dkg

import (
	"testing"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestProposalValidation(t *testing.T) {
	beaconID := "default"
	me := NewParticipant()
	someoneElse := NewParticipant()
	someoneElse.Address = "someoneelse.com"
	current := NewFullDKGEntry(beaconID, Complete, me)
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
				proposal := NewValidProposal(beaconID, me)
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
				proposal := NewValidProposal(beaconID, me)
				proposal.Timeout = timestamppb.New(time.Now().Add(-10 * time.Hour))
				return proposal
			}(),
			expected: ErrTimeoutReached,
		},
		{
			name:     "non-matching beaconID returns error",
			state:    current,
			terms:    NewValidProposal("some other beacon ID", me),
			expected: ErrInvalidBeaconID,
		},
		{
			name:  "epoch 0 returns an error",
			state: current,
			terms: func() *drand.ProposalTerms {
				proposal := NewValidProposal(beaconID, me)
				proposal.Epoch = 0
				return proposal
			}(),
			expected: ErrInvalidEpoch,
		},
		{
			name:  "if epoch is 1, nodes remaining returns an error",
			state: NewFreshState(beaconID),
			terms: func() *drand.ProposalTerms {
				proposal := NewInitialProposal(beaconID, me)
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
				proposal := NewValidProposal(beaconID, me)
				proposal.Epoch = 1
				proposal.GenesisSeed = nil
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
			name:  "if epoch is 1, leader not joining returns an error",
			state: NewFreshState(beaconID),
			terms: func() *drand.ProposalTerms {
				proposal := NewInitialProposal(beaconID, me)
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
				proposal := NewValidProposal(beaconID, me)
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
			name:  "if epoch is > 1, leader joining returns an error",
			state: current,
			terms: func() *drand.ProposalTerms {
				proposal := NewValidProposal(beaconID, me)
				proposal.Epoch = 2
				proposal.Joining = []*drand.Participant{
					proposal.Leader,
				}
				return proposal
			}(),
			expected: ErrLeaderCantJoinAfterFirstEpoch,
		},
		{
			name:  "if epoch is > 1, leader leaving returns an error",
			state: current,
			terms: func() *drand.ProposalTerms {
				proposal := NewValidProposal(beaconID, me)
				proposal.Epoch = 2
				proposal.Leaving = []*drand.Participant{
					proposal.Leader,
				}
				return proposal
			}(),
			expected: ErrLeaderNotRemaining,
		},
		{
			name:  "if epoch is > 1, leader not remaining returns an error",
			state: current,
			terms: func() *drand.ProposalTerms {
				proposal := NewValidProposal(beaconID, me)
				proposal.Epoch = 2
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
				invalidProposal := NewValidProposal(beaconID, me)
				invalidProposal.Epoch = 2
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
				invalidProposal := NewValidProposal(beaconID, me)
				invalidProposal.Epoch = 2
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
				invalidProposal := NewValidProposal(beaconID, me)
				invalidProposal.Epoch = 2
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
				details := NewFullDKGEntry(beaconID, Left, me)
				details.Epoch = 2
				return details
			}(),
			terms: func() *drand.ProposalTerms {
				validProposal := NewValidProposal(beaconID, me)
				validProposal.Epoch = 5
				validProposal.Leader = current.Leader
				validProposal.Remaining = []*drand.Participant{
					current.Leader,
				}
				return validProposal
			}(),
			expected: nil,
		},
		{
			name: "if current status is not Left, a proposed epoch of 1 higher succeeds",
			state: func() *DBState {
				details := NewFullDKGEntry(beaconID, Complete, me)
				details.Epoch = 2
				return details
			}(),
			terms: func() *drand.ProposalTerms {
				validProposal := NewValidProposal(beaconID, me)
				validProposal.Epoch = 3
				return validProposal
			}(),
			expected: nil,
		},
		{
			name: "if current status is not Left, a proposed epoch of > 1 higher returns an error",
			state: func() *DBState {
				details := NewFullDKGEntry(beaconID, Complete, me)
				details.Epoch = 2
				return details
			}(),
			terms: func() *drand.ProposalTerms {
				invalidEpochProposal := NewValidProposal(beaconID, me)
				invalidEpochProposal.Epoch = 4
				return invalidEpochProposal
			}(),
			expected: ErrInvalidEpoch,
		},
		{
			name: "proposed epoch less than the current epoch returns an error",
			state: func() *DBState {
				details := NewFullDKGEntry(beaconID, Complete, me)
				details.Epoch = 3
				return details
			}(),
			terms: func() *drand.ProposalTerms {
				invalidEpochProposal := NewValidProposal(beaconID, me)
				invalidEpochProposal.Epoch = 2
				return invalidEpochProposal
			}(),
			expected: ErrInvalidEpoch,
		},
		{
			name: "proposed epoch equal to the current epoch returns an error",
			state: func() *DBState {
				details := NewFullDKGEntry(beaconID, Left, me)
				details.Epoch = 3
				return details
			}(),
			terms: func() *drand.ProposalTerms {
				invalidEpochProposal := NewValidProposal(beaconID, me)
				invalidEpochProposal.Epoch = 3
				return invalidEpochProposal
			}(),
			expected: ErrInvalidEpoch,
		},
		{
			name:     "leaving out an existing node in a proposal returns an error",
			state:    NewFullDKGEntry(beaconID, Complete, me, someoneElse),
			terms:    NewValidProposal(beaconID, me),
			expected: ErrMissingNodesInProposal,
		},
		{
			name:     "proposing a remainer who doesn't exist in the current epoch returns an error",
			state:    NewFullDKGEntry(beaconID, Complete, me),
			terms:    NewValidProposal(beaconID, me, someoneElse),
			expected: ErrRemainingAndLeavingNodesMustExistInCurrentEpoch,
		},
		{
			name:  "invalid schemes return an error",
			state: NewFullDKGEntry(beaconID, Complete, me),
			terms: func() *drand.ProposalTerms {
				p := NewValidProposal(beaconID, me, someoneElse)
				p.SchemeID = "something made up"
				return p
			}(),
			expected: ErrInvalidScheme,
		},
		{
			name:  "trying to change the genesis time after the first epoch returns an error",
			state: NewFullDKGEntry(beaconID, Complete, me),
			terms: func() *drand.ProposalTerms {
				p := NewValidProposal(beaconID, me, someoneElse)
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
				p := NewInitialProposal(beaconID, me, someoneElse)
				p.Epoch = 1
				p.GenesisSeed = nil
				p.TransitionTime = timestamppb.New(time.Now())
				return p
			}(),
			expected: ErrTransitionTimeMustBeGenesisTime,
		},
		{
			name:  "for > epoch 1, transition time must not be missing",
			state: NewFullDKGEntry(beaconID, Complete, me),
			terms: func() *drand.ProposalTerms {
				p := NewValidProposal(beaconID, me, someoneElse)
				p.Epoch = 2
				p.TransitionTime = nil
				return p
			}(),
			expected: ErrTransitionTimeMissing,
		},
		{
			name:  "for > epoch 1, transition time must not be before the genesis time",
			state: NewFullDKGEntry(beaconID, Complete, me),
			terms: func() *drand.ProposalTerms {
				p := NewValidProposal(beaconID, me, someoneElse)
				p.Epoch = 2
				p.TransitionTime = timestamppb.New(time.Unix(0, 0))
				return p
			}(),
			expected: ErrTransitionTimeBeforeGenesis,
		},
		{
			name:  "for the first epoch, genesis seed cannot be provided",
			state: NewFreshState(beaconID),
			terms: func() *drand.ProposalTerms {
				p := NewValidProposal(beaconID, me, someoneElse)
				p.Epoch = 1
				p.GenesisSeed = []byte("deadbeef")
				return p
			}(),
			expected: ErrNoGenesisSeedForFirstEpoch,
		},
		{
			name:  "for non-fresh after first epoch, genesis seed must not change",
			state: NewFullDKGEntry(beaconID, Complete, me),
			terms: func() *drand.ProposalTerms {
				p := NewValidProposal(beaconID, me, someoneElse)
				p.Epoch = 2
				p.GenesisSeed = []byte("something-random")
				return p
			}(),
			expected: ErrGenesisSeedCannotChange,
		},
		{
			name:  "for fresh joining after first epoch, genesis seed must be provided but can be anything",
			state: NewFreshState(beaconID),
			terms: func() *drand.ProposalTerms {
				p := NewValidProposal(beaconID, me, someoneElse)
				p.Epoch = 2
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
	beaconID := "default"
	me := NewParticipant()
	tests := []stateChangeTableTest{
		{
			name:          "fresh state cannot time out",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Fresh, TimedOut),
		},
		{
			name:          "complete state cannot time out",
			startingState: NewFullDKGEntry(beaconID, Complete, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Complete, TimedOut),
		},
		{
			name:          "timed out state cannot time out",
			startingState: NewFullDKGEntry(beaconID, TimedOut, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(TimedOut, TimedOut),
		},
		{
			name:          "aborted state cannot time out",
			startingState: NewFullDKGEntry(beaconID, Aborted, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Aborted, TimedOut),
		},
		{
			name:          "left state cannot time out",
			startingState: NewFullDKGEntry(beaconID, Left, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Left, TimedOut),
		},
		{
			name:          "joined state can time out and changes state",
			startingState: NewFullDKGEntry(beaconID, Joined, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedResult: NewFullDKGEntry(beaconID, TimedOut, me),
			expectedError:  nil,
		},
		{
			name:          "proposed state can time out and changes state",
			startingState: NewFullDKGEntry(beaconID, Proposed, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedResult: NewFullDKGEntry(beaconID, TimedOut, me),
			expectedError:  nil,
		},
		{
			name:          "proposing state can time out and changes state",
			startingState: NewFullDKGEntry(beaconID, Proposing, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedResult: NewFullDKGEntry(beaconID, TimedOut, me),
			expectedError:  nil,
		},
		{
			name:          "executing state cannot time out and changes state",
			startingState: NewFullDKGEntry(beaconID, Executing, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedResult: NewFullDKGEntry(beaconID, TimedOut, me),
			expectedError:  nil,
		},
		{
			name:          "accepted state can time out and changes state",
			startingState: NewFullDKGEntry(beaconID, Accepted, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedResult: NewFullDKGEntry(beaconID, TimedOut, me),
			expectedError:  nil,
		},
		{
			name:          "rejected state can time out and changes state",
			startingState: NewFullDKGEntry(beaconID, Rejected, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.TimedOut()
			},
			expectedResult: NewFullDKGEntry(beaconID, TimedOut, me),
			expectedError:  nil,
		},
	}

	RunStateChangeTest(t, tests)
}

//nolint:funlen
func TestAbortCanOnlyBeCalledFromValidState(t *testing.T) {
	beaconID := "default"
	me := NewParticipant()
	tests := []stateChangeTableTest{
		{
			name:          "fresh state cannot be aborted",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Fresh, Aborted),
		},
		{
			name:          "complete state cannot be aborted",
			startingState: NewFullDKGEntry(beaconID, Complete, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Complete, Aborted),
		},
		{
			name:          "timed out state cannot be aborted",
			startingState: NewFullDKGEntry(beaconID, TimedOut, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(TimedOut, Aborted),
		},
		{
			name:          "aborted state cannot be aborted",
			startingState: NewFullDKGEntry(beaconID, Aborted, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Aborted, Aborted),
		},
		{
			name:          "left state can be aborted and changes state",
			startingState: NewFullDKGEntry(beaconID, Left, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted()
			},
			expectedResult: NewFullDKGEntry(beaconID, Aborted, me),
			expectedError:  nil,
		},
		{
			name:          "joined state can be aborted and changes state",
			startingState: NewFullDKGEntry(beaconID, Joined, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted()
			},
			expectedResult: NewFullDKGEntry(beaconID, Aborted, me),
			expectedError:  nil,
		},
		{
			name:          "proposed state can be aborted and changes state",
			startingState: NewFullDKGEntry(beaconID, Proposed, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted()
			},
			expectedResult: NewFullDKGEntry(beaconID, Aborted, me),
			expectedError:  nil,
		},
		{
			name:          "proposing state can be aborted and changes state",
			startingState: NewFullDKGEntry(beaconID, Proposing, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted()
			},
			expectedResult: NewFullDKGEntry(beaconID, Aborted, me),
			expectedError:  nil,
		},
		{
			name:          "executing state cannot be aborted",
			startingState: NewFullDKGEntry(beaconID, Executing, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Executing, Aborted),
		},
		{
			name:          "accepted state can be aborted and changes state",
			startingState: NewFullDKGEntry(beaconID, Accepted, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted()
			},
			expectedResult: NewFullDKGEntry(beaconID, Aborted, me),
			expectedError:  nil,
		},
		{
			name:          "rejected state can be aborted and changes state",
			startingState: NewFullDKGEntry(beaconID, Rejected, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Aborted()
			},
			expectedResult: NewFullDKGEntry(beaconID, Aborted, me),
			expectedError:  nil,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestJoiningADKGFromProposal(t *testing.T) {
	beaconID := "default"
	me := NewParticipant()
	leader := drand.Participant{
		Address: "some-leader",
	}
	tests := []stateChangeTableTest{
		{
			name: "fresh state can join with a valid proposal",
			startingState: func() *DBState {
				s, _ := NewFreshState(beaconID).Proposed(&leader, me, NewInitialProposal(beaconID, &leader, me))
				return s
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Joined(me, nil)
			},
			expectedResult: func() *DBState {
				proposal := NewValidProposal(beaconID, &leader)
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
					Joining:        []*drand.Participant{&leader, me},
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
				someoneWhoIsntMe := drand.Participant{Address: "someone-unrelated.org"}
				return NewFullDKGEntry(beaconID, Proposed, NewParticipant(), &someoneWhoIsntMe)
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Joined(me, nil)
			},
			expectedError: ErrCannotJoinIfNotInJoining,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestProposingDKGFromFresh(t *testing.T) {
	beaconID := "default"
	me := NewParticipant()
	tests := []stateChangeTableTest{
		{
			name:          "Proposing a valid DKG changes state to Proposing",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(me, NewInitialProposal(beaconID, me))
			},
			expectedResult: func() *DBState {
				proposal := NewValidProposal(beaconID, me)
				return &DBState{
					BeaconID:       beaconID,
					Epoch:          1,
					State:          Proposing,
					Leader:         me,
					Threshold:      proposal.Threshold,
					SchemeID:       proposal.SchemeID,
					GenesisTime:    proposal.GenesisTime.AsTime(),
					TransitionTime: proposal.TransitionTime.AsTime(),
					CatchupPeriod:  time.Duration(proposal.CatchupPeriodSeconds) * time.Second,
					BeaconPeriod:   time.Duration(proposal.BeaconPeriodSeconds) * time.Second,
					Timeout:        proposal.Timeout.AsTime(),
					Remaining:      nil,
					Joining:        []*drand.Participant{me},
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
				invalidProposal := NewValidProposal(beaconID, me)
				invalidProposal.Leader = me
				invalidProposal.Epoch = 0

				return in.Proposing(me, invalidProposal)
			},
			expectedResult: nil,
			expectedError:  ErrInvalidEpoch,
		},
		{
			name:          "Proposing a DKG as non-leader returns an error",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				someRandomPerson := &drand.Participant{
					Address: "somebody-that-isnt-me.com",
				}

				return in.Proposing(me, NewInitialProposal(beaconID, someRandomPerson))
			},
			expectedResult: nil,
			expectedError:  ErrCannotProposeAsNonLeader,
		},
		{
			name:          "Proposing a DKG with epoch > 1 when fresh state returns an error",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				invalidEpochProposal := NewValidProposal(beaconID, me)
				invalidEpochProposal.Leader = me
				invalidEpochProposal.Epoch = 2

				return in.Proposing(me, invalidEpochProposal)
			},
			expectedResult: nil,
			expectedError:  ErrInvalidEpoch,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestProposingDKGFromNonFresh(t *testing.T) {
	beaconID := "default"
	me := NewParticipant()

	tests := []stateChangeTableTest{
		{
			name:          "Proposing a valid DKG from Complete changes state to Proposing",
			startingState: NewFullDKGEntry(beaconID, Complete, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				validProposal := NewValidProposal(beaconID, me)
				validProposal.Epoch = 2

				return in.Proposing(me, validProposal)
			},
			expectedResult: func() *DBState {
				proposal := NewValidProposal(beaconID, me)
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
					Leader:         me,
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
			startingState: NewFullDKGEntry(beaconID, Aborted, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				validProposal := NewValidProposal(beaconID, me)
				validProposal.Epoch = 2

				return in.Proposing(me, validProposal)
			},
			expectedResult: func() *DBState {
				proposal := NewValidProposal(beaconID, me)
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
					Leader:         me,
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
			startingState: NewFullDKGEntry(beaconID, TimedOut, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				validProposal := NewValidProposal(beaconID, me)
				validProposal.Epoch = 2

				return in.Proposing(me, validProposal)
			},
			expectedResult: func() *DBState {
				proposal := NewValidProposal(beaconID, me)
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
					Leader:         me,
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
			startingState: NewFullDKGEntry(beaconID, Joined, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},
			expectedError: InvalidStateChange(Joined, Proposing),
		},
		{
			name:          "proposing a DKG when leaving returns error",
			startingState: NewFullDKGEntry(beaconID, Left, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},
			expectedError: InvalidStateChange(Left, Proposing),
		},
		{
			name:          "proposing a DKG when already proposing returns an error",
			startingState: NewFullDKGEntry(beaconID, Proposing, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},
			expectedError: InvalidStateChange(Proposing, Proposing),
		},
		{
			name:          "proposing a DKG when one has already been proposed returns an error",
			startingState: NewFullDKGEntry(beaconID, Proposed, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},

			expectedError: InvalidStateChange(Proposed, Proposing),
		},
		{
			name:          "proposing a DKG during execution returns an error",
			startingState: NewFullDKGEntry(beaconID, Executing, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},
			expectedError: InvalidStateChange(Executing, Proposing),
		},
		{
			name:          "proposing a DKG after acceptance returns an error",
			startingState: NewFullDKGEntry(beaconID, Accepted, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},
			expectedError: InvalidStateChange(Accepted, Proposing),
		},
		{
			name:          "proposing a DKG after rejection returns an error",
			startingState: NewFullDKGEntry(beaconID, Rejected, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},
			expectedError: InvalidStateChange(Rejected, Proposing),
		},
	}

	RunStateChangeTest(t, tests)
}

//nolint:funlen
func TestProposedDKG(t *testing.T) {
	beaconID := "default"
	anotherNode := NewParticipant()
	me := &drand.Participant{
		Address: "me myself and I",
	}

	tests := []stateChangeTableTest{
		{
			name:          "Being proposed a valid DKG from Complete changes state to Proposed",
			startingState: NewFullDKGEntry(beaconID, Complete, anotherNode, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				validProposal := NewValidProposal(beaconID, anotherNode, me)
				validProposal.Epoch = 2

				return in.Proposed(anotherNode, me, validProposal)
			},
			expectedResult: func() *DBState {
				proposal := NewValidProposal(beaconID, anotherNode, me)
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
					Leader:         anotherNode,
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
				return in.Proposed(anotherNode, me, NewInitialProposal(beaconID, anotherNode, me))
			},
			expectedResult: func() *DBState {
				proposal := NewInitialProposal(beaconID, anotherNode, me)
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
					Leader:         anotherNode,
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
				return in.Proposed(anotherNode, me, NewInitialProposal(beaconID, anotherNode))
			},
			expectedError: ErrSelfMissingFromProposal,
		},
		{
			name:          "Being proposed a valid DKG with epoch > 1 from Fresh state changes state to Proposed",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposed(anotherNode, me, NewValidProposal(beaconID, anotherNode, me))
			},
			expectedResult: func() *DBState {
				proposal := NewValidProposal(beaconID, anotherNode, me)
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
					Leader:         anotherNode,
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
			startingState: NewFullDKGEntry(beaconID, Executing, anotherNode),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Proposed(anotherNode, me, NewValidProposal(beaconID, anotherNode))
			},
			expectedError: InvalidStateChange(Executing, Proposed),
		},
		{
			name:          "Being proposed a DKG by somebody who isn't the leader returns an error",
			startingState: NewFullDKGEntry(beaconID, Aborted, anotherNode),
			transitionFn: func(in *DBState) (*DBState, error) {
				aThirdParty := &drand.Participant{Address: "another-party-entirely"}
				return in.Proposed(aThirdParty, me, NewValidProposal(beaconID, anotherNode))
			},
			expectedError: ErrCannotProposeAsNonLeader,
		},
		{
			name:          "Being proposed an otherwise invalid DKG returns an error",
			startingState: NewFullDKGEntry(beaconID, Aborted, anotherNode),
			transitionFn: func(in *DBState) (*DBState, error) {
				validProposal := NewValidProposal(beaconID, anotherNode)
				validProposal.Epoch = 0

				return in.Proposed(anotherNode, me, validProposal)
			},
			expectedError: ErrInvalidEpoch,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestAcceptingDKG(t *testing.T) {
	beaconID := "default"
	me := NewParticipant()
	leader := drand.Participant{
		Address: "big leader",
	}

	tests := []stateChangeTableTest{
		{
			name:          "valid proposal can be accepted",
			startingState: NewFullDKGEntry(beaconID, Proposed, &leader, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Accepted(me)
			},
			expectedResult: NewFullDKGEntry(beaconID, Accepted, &leader, me),
			expectedError:  nil,
		},
		{
			name:          "cannot accept a fresh proposal",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Accepted(me)
			},
			expectedError: InvalidStateChange(Fresh, Accepted),
		},
		{
			name:          "cannot accept own proposal",
			startingState: NewFullDKGEntry(beaconID, Proposing, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Accepted(me)
			},
			expectedError: InvalidStateChange(Proposing, Accepted),
		},
		{
			name:          "cannot accept a proposal i've already rejected",
			startingState: NewFullDKGEntry(beaconID, Rejected, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Accepted(me)
			},
			expectedError: InvalidStateChange(Rejected, Accepted),
		},
		{
			name:          "cannot accept a proposal that has already timed out",
			startingState: PastTimeout(NewFullDKGEntry(beaconID, Proposed, me)),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Accepted(me)
			},
			expectedError: ErrTimeoutReached,
		},
		{
			name: "cannot accept a proposal where I am leaving",
			startingState: func() *DBState {
				details := NewFullDKGEntry(beaconID, Proposed, &leader)
				details.Leaving = []*drand.Participant{me}
				return details
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Accepted(me)
			},
			expectedError: ErrCannotAcceptProposalWhereLeaving,
		},
		{
			name: "cannot accept a proposal where I am joining",
			startingState: func() *DBState {
				details := NewFullDKGEntry(beaconID, Proposed, &leader)
				details.Joining = []*drand.Participant{me}
				return details
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Accepted(me)
			},
			expectedError: ErrCannotAcceptProposalWhereJoining,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestRejectingDKG(t *testing.T) {
	beaconID := "default"
	me := NewParticipant()
	leader := drand.Participant{
		Address: "big leader",
	}

	tests := []stateChangeTableTest{
		{
			name:          "valid proposal can be rejected",
			startingState: NewFullDKGEntry(beaconID, Proposed, &leader, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Rejected(me)
			},
			expectedResult: NewFullDKGEntry(beaconID, Rejected, &leader, me),
			expectedError:  nil,
		},
		{
			name:          "cannot reject a fresh proposal",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Rejected(me)
			},
			expectedError: InvalidStateChange(Fresh, Rejected),
		},
		{
			name:          "cannot reject own proposal",
			startingState: NewFullDKGEntry(beaconID, Proposing, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Rejected(me)
			},
			expectedError: InvalidStateChange(Proposing, Rejected),
		},
		{
			name:          "cannot rejected a proposal i've already accepted",
			startingState: NewFullDKGEntry(beaconID, Accepted, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Rejected(me)
			},
			expectedError: InvalidStateChange(Accepted, Rejected),
		},
		{
			name:          "cannot reject a proposal that has already timed out",
			startingState: PastTimeout(NewFullDKGEntry(beaconID, Proposed, me)),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Rejected(me)
			},
			expectedError: ErrTimeoutReached,
		},
		{
			name: "cannot reject a proposal where I am leaving",
			startingState: func() *DBState {
				details := NewFullDKGEntry(beaconID, Proposed, &leader)
				details.Leaving = []*drand.Participant{me}
				return details
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Rejected(me)
			},
			expectedError: ErrCannotRejectProposalWhereLeaving,
		},
		{
			name: "cannot reject a proposal where I am joining",
			startingState: func() *DBState {
				details := NewFullDKGEntry(beaconID, Proposed, &leader)
				details.Joining = []*drand.Participant{me}
				return details
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Rejected(me)
			},
			expectedError: ErrCannotRejectProposalWhereJoining,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestLeftDKG(t *testing.T) {
	beaconID := "default"
	me := NewParticipant()
	leader := drand.Participant{
		Address: "big leader",
	}

	tests := []stateChangeTableTest{
		{
			name: "can leave valid proposal that contains me as a leaver",
			startingState: func() *DBState {
				details := NewFullDKGEntry(beaconID, Proposed, &leader)
				details.Leaving = []*drand.Participant{
					me,
				}
				return details
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Left(me)
			},
			expectedResult: func() *DBState {
				details := NewFullDKGEntry(beaconID, Left, &leader)
				details.Leaving = []*drand.Participant{
					me,
				}
				return details
			}(),
			expectedError: nil,
		},
		{
			name: "can leave valid proposal immediately if I have just joined it",
			startingState: func() *DBState {
				details := NewFullDKGEntry(beaconID, Joined, &leader)
				details.Joining = []*drand.Participant{
					me,
				}
				return details
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Left(me)
			},
			expectedResult: func() *DBState {
				details := NewFullDKGEntry(beaconID, Left, &leader)
				details.Joining = []*drand.Participant{
					me,
				}
				return details
			}(),
			expectedError: nil,
		},
		{
			name:          "trying to leave if not a leaver returns an error",
			startingState: NewFullDKGEntry(beaconID, Proposed, &leader, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Left(me)
			},
			expectedError: ErrCannotLeaveIfNotALeaver,
		},
		{
			name: "attempting to leave if timeout has been reached returns an error",
			startingState: func() *DBState {
				details := PastTimeout(NewFullDKGEntry(beaconID, Proposed, &leader))
				details.Leaving = []*drand.Participant{
					me,
				}
				return details
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Left(me)
			},
			expectedError: ErrTimeoutReached,
		},
		{
			name: "cannot leave if proposal already complete",
			startingState: func() *DBState {
				details := NewFullDKGEntry(beaconID, Complete, &leader)
				details.Leaving = []*drand.Participant{
					me,
				}
				return details
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Left(me)
			},
			expectedError: InvalidStateChange(Complete, Left),
		},
	}

	RunStateChangeTest(t, tests)
}

func TestExecutingDKG(t *testing.T) {
	beaconID := "default"
	me := NewParticipant()
	leader := drand.Participant{
		Address: "big leader",
	}

	tests := []stateChangeTableTest{
		{
			name:          "executing valid proposal that I have accepted succeeds",
			startingState: NewFullDKGEntry(beaconID, Accepted, &leader, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Executing(me)
			},
			expectedResult: NewFullDKGEntry(beaconID, Executing, &leader, me),
			expectedError:  nil,
		},
		{
			name:          "executing a valid proposal that I have rejected returns an error",
			startingState: NewFullDKGEntry(beaconID, Rejected, &leader, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Executing(me)
			},
			expectedError: InvalidStateChange(Rejected, Executing),
		},
		{
			name:          "executing a proposal after time out returns an error",
			startingState: PastTimeout(NewFullDKGEntry(beaconID, Accepted, &leader, me)),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Executing(me)
			},
			expectedError: ErrTimeoutReached,
		},
		{
			name:          "executing a valid proposal that I am not joining or remaining in returns an error (but shouldn't have been possible anyway)",
			startingState: NewFullDKGEntry(beaconID, Accepted, &leader),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Executing(me)
			},
			expectedError: ErrCannotExecuteIfNotJoinerOrRemainer,
		},
		{
			name: "executing as a leaver transitions me to Left",
			startingState: func() *DBState {
				state := NewFullDKGEntry(beaconID, Proposed, &leader)
				state.Leaving = append(state.Leaving, me)
				return state
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Executing(me)
			},
			expectedResult: func() *DBState {
				state := NewFullDKGEntry(beaconID, Left, &leader)
				state.Leaving = append(state.Leaving, me)
				return state
			}(),
		},
	}

	RunStateChangeTest(t, tests)
}

func TestEviction(t *testing.T) {
	beaconID := "default"
	me := NewParticipant()
	tests := []stateChangeTableTest{
		{
			name:          "can be evicted from an executing DKG (e.g. if evicted)",
			startingState: NewFullDKGEntry(beaconID, Executing, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Evicted()
			},
			expectedError: nil,
		},
		{
			name:          "can be evicted from a timed out DKG (in case you missed the eviction)",
			startingState: NewFullDKGEntry(beaconID, TimedOut, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Evicted()
			},
			expectedError: nil,
		},
		{
			name:          "cannot be evicted from a DKG before execution",
			startingState: NewFullDKGEntry(beaconID, Proposed, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Evicted()
			},
			expectedError: InvalidStateChange(Proposed, Evicted),
		},
	}
	RunStateChangeTest(t, tests)
}

func TestCompleteDKG(t *testing.T) {
	beaconID := "default"
	me := NewParticipant()
	leader := drand.Participant{
		Address: "big leader",
	}
	finalGroup := key.Group{}
	keyShare := key.Share{}

	tests := []stateChangeTableTest{
		{
			name:          "completing a valid executing proposal succeeds",
			startingState: NewFullDKGEntry(beaconID, Executing, &leader, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Complete(&finalGroup, &keyShare)
			},
			expectedResult: func() *DBState {
				d := NewFullDKGEntry(beaconID, Complete, &leader, me)
				d.FinalGroup = &finalGroup
				d.GenesisSeed = finalGroup.GetGenesisSeed()
				d.KeyShare = &keyShare
				return d
			}(),
			expectedError: nil,
		},
		{
			name:          "completing a non-executing proposal returns an error",
			startingState: NewFullDKGEntry(beaconID, Accepted, &leader, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Complete(&finalGroup, &keyShare)
			},
			expectedError: InvalidStateChange(Accepted, Complete),
		},
		{
			name:          "completing a proposal after time out returns an error",
			startingState: PastTimeout(NewFullDKGEntry(beaconID, Executing, &leader, me)),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.Complete(&finalGroup, &keyShare)
			},
			expectedError: ErrTimeoutReached,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestReceivedAcceptance(t *testing.T) {
	beaconID := "default"
	me := NewParticipant()
	somebodyElse := drand.Participant{
		Address: "some participant",
	}
	aThirdParty := drand.Participant{
		Address: "number3",
	}

	tests := []stateChangeTableTest{
		{
			name:          "receiving a valid acceptance for a proposal I made adds it to the list of acceptors",
			startingState: NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedAcceptance(me, &somebodyElse)
			},
			expectedResult: func() *DBState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Acceptors = []*drand.Participant{&somebodyElse}
				return d
			}(),
			expectedError: nil,
		},
		{
			name:          "receiving an acceptance for a proposal I didn't make returns an error (shouldn't be possible)",
			startingState: NewFullDKGEntry(beaconID, Proposing, &somebodyElse, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedAcceptance(me, &somebodyElse)
			},
			expectedError: ErrNonLeaderCannotReceiveAcceptance,
		},
		{
			name:          "receiving an acceptance from somebody who isn't a remainer returns an error",
			startingState: NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedAcceptance(me, &drand.Participant{Address: "who is this?!?"})
			},
			expectedError: ErrUnknownAcceptor,
		},
		{
			name:          "receiving acceptance from non-proposing state returns an error",
			startingState: NewFullDKGEntry(beaconID, Proposed, me, &somebodyElse),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedAcceptance(me, &somebodyElse)
			},
			expectedError: InvalidStateChange(Proposed, Proposing),
		},
		{
			name: "acceptances are appended to acceptors",
			startingState: func() *DBState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Acceptors = []*drand.Participant{
					&aThirdParty,
				}
				return d
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedAcceptance(me, &somebodyElse)
			},
			expectedResult: func() *DBState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Acceptors = []*drand.Participant{
					&aThirdParty,
					&somebodyElse,
				}
				return d
			}(),
			expectedError: nil,
		},
		{
			name: "duplicate acceptance returns an error",
			startingState: func() *DBState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Acceptors = []*drand.Participant{
					&somebodyElse,
				}
				return d
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedAcceptance(me, &somebodyElse)
			},
			expectedError: ErrDuplicateAcceptance,
		},
		{
			name: "if a party has rejected and they send an acceptance, they are moved into acceptance",
			startingState: func() *DBState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Rejectors = []*drand.Participant{
					&somebodyElse,
				}
				return d
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedAcceptance(me, &somebodyElse)
			},
			expectedResult: func() *DBState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Acceptors = []*drand.Participant{
					&somebodyElse,
				}
				return d
			}(),
			expectedError: nil,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestReceivedRejection(t *testing.T) {
	beaconID := "default"
	me := NewParticipant()
	somebodyElse := drand.Participant{
		Address: "some participant",
	}
	aThirdParty := drand.Participant{
		Address: "number3",
	}

	tests := []stateChangeTableTest{
		{
			name:          "receiving a valid rejection for a proposal I made adds it to the list of rejectors",
			startingState: NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedRejection(me, &somebodyElse)
			},
			expectedResult: func() *DBState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Rejectors = []*drand.Participant{&somebodyElse}
				return d
			}(),
			expectedError: nil,
		},
		{
			name:          "receiving a rejection for a proposal I didn't make returns an error (shouldn't be possible)",
			startingState: NewFullDKGEntry(beaconID, Proposing, &somebodyElse, me),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedRejection(me, &somebodyElse)
			},
			expectedError: ErrNonLeaderCannotReceiveRejection,
		},
		{
			name:          "receiving a rejection from somebody who isn't a remainer returns an error",
			startingState: NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedRejection(me, &drand.Participant{Address: "who is this?!?"})
			},
			expectedError: ErrUnknownRejector,
		},
		{
			name:          "receiving rejection from non-proposing state returns an error",
			startingState: NewFullDKGEntry(beaconID, Proposed, me, &somebodyElse),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedRejection(me, &somebodyElse)
			},
			expectedError: InvalidStateChange(Proposed, Proposing),
		},
		{
			name: "rejections are appended to rejectors",
			startingState: func() *DBState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Rejectors = []*drand.Participant{
					&aThirdParty,
				}
				return d
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedRejection(me, &somebodyElse)
			},
			expectedResult: func() *DBState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Rejectors = []*drand.Participant{
					&aThirdParty,
					&somebodyElse,
				}
				return d
			}(),
			expectedError: nil,
		},
		{
			name: "duplicate rejection returns an error",
			startingState: func() *DBState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Rejectors = []*drand.Participant{
					&somebodyElse,
				}
				return d
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedRejection(me, &somebodyElse)
			},
			expectedError: ErrDuplicateRejection,
		},
		{
			name: "if a party has accepted and they send a rejection, they are moved into rejectors",
			startingState: func() *DBState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Acceptors = []*drand.Participant{
					&somebodyElse,
				}
				return d
			}(),
			transitionFn: func(in *DBState) (*DBState, error) {
				return in.ReceivedRejection(me, &somebodyElse)
			},
			expectedResult: func() *DBState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Rejectors = []*drand.Participant{
					&somebodyElse,
				}
				return d
			}(),
			expectedError: nil,
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
				require.EqualValues(t, test.expectedResult, result)
			}
		})
	}
}

func NewParticipant() *drand.Participant {
	k := key.NewKeyPair("somewhere.com")
	pk, _ := k.Public.Key.MarshalBinary()
	return &drand.Participant{
		Address: "somewhere.com",
		Tls:     false,
		PubKey:  pk,
	}
}

func NewFullDKGEntry(beaconID string, status DKGStatus, previousLeader *drand.Participant, others ...*drand.Participant) *DBState {
	state := DBState{
		BeaconID:       beaconID,
		Epoch:          1,
		State:          status,
		Threshold:      1,
		Timeout:        time.Unix(2549084715, 0).UTC(), // this will need updated in 2050 :^)
		SchemeID:       "pedersen-bls-chained",
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
		SchemeID:             "pedersen-bls-chained",
		Joining:              append([]*drand.Participant{leader}, others...),
	}
}

func NewValidProposal(beaconID string, leader *drand.Participant, others ...*drand.Participant) *drand.ProposalTerms {
	return &drand.ProposalTerms{
		BeaconID:             beaconID,
		Epoch:                2,
		Leader:               leader,
		Threshold:            1,
		Timeout:              timestamppb.New(time.Unix(2549084715, 0).UTC()), // this will need updated in 2050 :^)
		GenesisTime:          timestamppb.New(time.Unix(1669718523, 0).UTC()),
		GenesisSeed:          []byte("deadbeef"),
		TransitionTime:       timestamppb.New(time.Unix(1669718523, 0).UTC()),
		CatchupPeriodSeconds: 5,
		BeaconPeriodSeconds:  10,
		SchemeID:             "pedersen-bls-chained",
		Remaining:            append([]*drand.Participant{leader}, others...),
	}
}

func PastTimeout(d *DBState) *DBState {
	d.Timeout = time.Now().Add(-1 * time.Minute).UTC()
	return d
}
