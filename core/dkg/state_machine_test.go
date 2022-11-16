package dkg

import (
	"github.com/drand/drand/protobuf/drand"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"testing"
	"time"
)

func TestProposalValidation(t *testing.T) {
	beaconID := "default"
	me := NewParticipant()
	current := NewFullDKGEntry(beaconID, Complete, me)
	tests := []struct {
		name     string
		state    *DKGState
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

			expected: TimeoutReached,
		},
		{
			name:     "non-matching beaconID returns error",
			state:    current,
			terms:    NewValidProposal("some other beacon ID", me),
			expected: InvalidBeaconID,
		},
		{
			name:  "epoch 0 returns an error",
			state: current,
			terms: func() *drand.ProposalTerms {
				proposal := NewValidProposal(beaconID, me)
				proposal.Epoch = 0
				return proposal
			}(),
			expected: InvalidEpoch,
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
			expected: OnlyJoinersAllowedForFirstEpoch,
		},
		{
			name:  "if epoch is 1, nodes leaving returns an error",
			state: NewFreshState(beaconID),
			terms: func() *drand.ProposalTerms {
				proposal := NewValidProposal(beaconID, me)
				proposal.Epoch = 1
				proposal.Leaving = []*drand.Participant{
					{
						Address: "somebody.com",
					},
				}
				return proposal
			}(),
			expected: OnlyJoinersAllowedForFirstEpoch,
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
			expected: LeaderNotJoining,
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
			expected: NoNodesRemaining,
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
			expected: LeaderCantJoinAfterFirstEpoch,
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
			expected: LeaderNotRemaining,
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
			expected: LeaderNotRemaining,
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
			expected: ThresholdHigherThanNodeCount,
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
			expected: RemainingNodesMustExistInCurrentEpoch,
		},
		{
			name: "if current status is Left, any higher epoch value is valid",
			state: func() *DKGState {
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
			state: func() *DKGState {
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
			state: func() *DKGState {
				details := NewFullDKGEntry(beaconID, Complete, me)
				details.Epoch = 2
				return details
			}(),
			terms: func() *drand.ProposalTerms {
				invalidEpochProposal := NewValidProposal(beaconID, me)
				invalidEpochProposal.Epoch = 4
				return invalidEpochProposal
			}(),
			expected: InvalidEpoch,
		},
		{
			name: "proposed epoch less than the current epoch returns an error",
			state: func() *DKGState {
				details := NewFullDKGEntry(beaconID, Complete, me)
				details.Epoch = 3
				return details
			}(),
			terms: func() *drand.ProposalTerms {
				invalidEpochProposal := NewValidProposal(beaconID, me)
				invalidEpochProposal.Epoch = 2
				return invalidEpochProposal
			}(),
			expected: InvalidEpoch,
		},
		{
			name: "proposed epoch equal to the current epoch returns an error",
			state: func() *DKGState {
				details := NewFullDKGEntry(beaconID, Left, me)
				details.Epoch = 3
				return details
			}(),
			terms: func() *drand.ProposalTerms {
				invalidEpochProposal := NewValidProposal(beaconID, me)
				invalidEpochProposal.Epoch = 3
				return invalidEpochProposal
			}(),
			expected: InvalidEpoch,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ValidateProposal(test.state, test.terms)
			require.Equal(t, test.expected, result, "expected %s, got %s", test.expected, result)
		})
	}
}

func TestTimeoutCanOnlyBeCalledFromValidState(t *testing.T) {
	beaconID := "default"
	me := NewParticipant()
	tests := []stateChangeTableTest{
		{
			name:          "fresh state cannot time out",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.TimedOut()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Fresh, TimedOut),
		},
		{
			name:          "complete state cannot time out",
			startingState: NewFullDKGEntry(beaconID, Complete, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.TimedOut()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Complete, TimedOut),
		},
		{
			name:          "timed out state cannot time out",
			startingState: NewFullDKGEntry(beaconID, TimedOut, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.TimedOut()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(TimedOut, TimedOut),
		},
		{
			name:          "aborted state cannot time out",
			startingState: NewFullDKGEntry(beaconID, Aborted, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.TimedOut()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Aborted, TimedOut),
		},
		{
			name:          "left state cannot time out",
			startingState: NewFullDKGEntry(beaconID, Left, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.TimedOut()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Left, TimedOut),
		},
		{
			name:          "joined state can time out and changes state",
			startingState: NewFullDKGEntry(beaconID, Joined, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.TimedOut()
			},
			expectedResult: NewFullDKGEntry(beaconID, TimedOut, me),
			expectedError:  nil,
		},
		{
			name:          "proposed state can time out and changes state",
			startingState: NewFullDKGEntry(beaconID, Proposed, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.TimedOut()
			},
			expectedResult: NewFullDKGEntry(beaconID, TimedOut, me),
			expectedError:  nil,
		},
		{
			name:          "proposing state can time out and changes state",
			startingState: NewFullDKGEntry(beaconID, Proposing, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.TimedOut()
			},
			expectedResult: NewFullDKGEntry(beaconID, TimedOut, me),
			expectedError:  nil,
		},
		{
			name:          "executing state cannot time out and changes state",
			startingState: NewFullDKGEntry(beaconID, Executing, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.TimedOut()
			},
			expectedResult: NewFullDKGEntry(beaconID, TimedOut, me),
			expectedError:  nil,
		},
		{
			name:          "accepted state can time out and changes state",
			startingState: NewFullDKGEntry(beaconID, Accepted, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.TimedOut()
			},
			expectedResult: NewFullDKGEntry(beaconID, TimedOut, me),
			expectedError:  nil,
		},
		{
			name:          "rejected state can time out and changes state",
			startingState: NewFullDKGEntry(beaconID, Rejected, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.TimedOut()
			},
			expectedResult: NewFullDKGEntry(beaconID, TimedOut, me),
			expectedError:  nil,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestAbortCanOnlyBeCalledFromValidState(t *testing.T) {
	beaconID := "default"
	me := NewParticipant()
	tests := []stateChangeTableTest{
		{
			name:          "fresh state cannot be aborted",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Aborted()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Fresh, Aborted),
		},
		{
			name:          "complete state cannot be aborted",
			startingState: NewFullDKGEntry(beaconID, Complete, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Aborted()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Complete, Aborted),
		},
		{
			name:          "timed out state cannot be aborted",
			startingState: NewFullDKGEntry(beaconID, TimedOut, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Aborted()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(TimedOut, Aborted),
		},
		{
			name:          "aborted state cannot be aborted",
			startingState: NewFullDKGEntry(beaconID, Aborted, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Aborted()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Aborted, Aborted),
		},
		{
			name:          "left state can be aborted and changes state",
			startingState: NewFullDKGEntry(beaconID, Left, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Aborted()
			},
			expectedResult: NewFullDKGEntry(beaconID, Aborted, me),
			expectedError:  nil,
		},
		{
			name:          "joined state can be aborted and changes state",
			startingState: NewFullDKGEntry(beaconID, Joined, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Aborted()
			},
			expectedResult: NewFullDKGEntry(beaconID, Aborted, me),
			expectedError:  nil,
		},
		{
			name:          "proposed state can be aborted and changes state",
			startingState: NewFullDKGEntry(beaconID, Proposed, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Aborted()
			},
			expectedResult: NewFullDKGEntry(beaconID, Aborted, me),
			expectedError:  nil,
		},
		{
			name:          "proposing state can be aborted and changes state",
			startingState: NewFullDKGEntry(beaconID, Proposing, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Aborted()
			},
			expectedResult: NewFullDKGEntry(beaconID, Aborted, me),
			expectedError:  nil,
		},
		{
			name:          "executing state cannot be aborted",
			startingState: NewFullDKGEntry(beaconID, Executing, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Aborted()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange(Executing, Aborted),
		},
		{
			name:          "accepted state can be aborted and changes state",
			startingState: NewFullDKGEntry(beaconID, Accepted, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Aborted()
			},
			expectedResult: NewFullDKGEntry(beaconID, Aborted, me),
			expectedError:  nil,
		},
		{
			name:          "rejected state can be aborted and changes state",
			startingState: NewFullDKGEntry(beaconID, Rejected, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Aborted()
			},
			expectedResult: NewFullDKGEntry(beaconID, Aborted, me),
			expectedError:  nil,
		},
	}

	RunStateChangeTest(t, tests)
}

func TestJoiningADKGFromFresh(t *testing.T) {
	beaconID := "default"
	me := NewParticipant()
	leader := drand.Participant{
		Address: "some-leader",
	}
	tests := []stateChangeTableTest{
		{
			name:          "fresh state can join with a valid proposal",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Joined(me, NewInitialProposal(beaconID, &leader, me))
			},
			expectedResult: func() *DKGState {
				proposal := NewValidProposal(beaconID, &leader)
				return &DKGState{
					BeaconID:   beaconID,
					Epoch:      1,
					State:      Joined,
					Threshold:  proposal.Threshold,
					Timeout:    proposal.Timeout.AsTime(),
					Leader:     proposal.Leader,
					Remaining:  nil,
					Joining:    []*drand.Participant{&leader, me},
					Leaving:    nil,
					FinalGroup: nil,
				}
			}(),
			expectedError: nil,
		},
		{
			name:          "fresh state join fails if self not present in joining",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				someoneWhoIsntMe := drand.Participant{Address: "someone-unrelated.org"}
				return in.Joined(me, NewInitialProposal(beaconID, &leader, &someoneWhoIsntMe))
			},
			expectedError: SelfMissingFromProposal,
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
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Proposing(me, NewInitialProposal(beaconID, me))
			},
			expectedResult: func() *DKGState {
				proposal := NewValidProposal(beaconID, me)
				return &DKGState{
					BeaconID:   beaconID,
					Epoch:      1,
					State:      Proposing,
					Threshold:  proposal.Threshold,
					Timeout:    proposal.Timeout.AsTime(),
					Leader:     me,
					Remaining:  nil,
					Joining:    []*drand.Participant{me},
					Leaving:    nil,
					FinalGroup: nil,
				}
			}(),
			expectedError: nil,
		},
		{
			name:          "Proposing an invalid DKG returns an error",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				invalidProposal := NewValidProposal(beaconID, me)
				invalidProposal.Leader = me
				invalidProposal.Epoch = 0

				return in.Proposing(me, invalidProposal)
			},
			expectedResult: nil,
			expectedError:  InvalidEpoch,
		},
		{
			name:          "Proposing a DKG as non-leader returns an error",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				someRandomPerson := &drand.Participant{
					Address: "somebody-that-isnt-me.com",
				}

				return in.Proposing(me, NewInitialProposal(beaconID, someRandomPerson))
			},
			expectedResult: nil,
			expectedError:  CannotProposeAsNonLeader,
		},
		{
			name:          "Proposing a DKG with epoch > 1 when fresh state returns an error",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				invalidEpochProposal := NewValidProposal(beaconID, me)
				invalidEpochProposal.Leader = me
				invalidEpochProposal.Epoch = 2

				return in.Proposing(me, invalidEpochProposal)
			},
			expectedResult: nil,
			expectedError:  InvalidEpoch,
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
			transitionFn: func(in *DKGState) (*DKGState, error) {
				validProposal := NewValidProposal(beaconID, me)
				validProposal.Epoch = 2

				return in.Proposing(me, validProposal)
			},
			expectedResult: func() *DKGState {
				proposal := NewValidProposal(beaconID, me)
				return &DKGState{
					BeaconID:   beaconID,
					Epoch:      2,
					State:      Proposing,
					Threshold:  proposal.Threshold,
					Timeout:    proposal.Timeout.AsTime(),
					Leader:     me,
					Remaining:  proposal.Remaining,
					Joining:    nil,
					Leaving:    nil,
					FinalGroup: nil,
				}
			}(),
			expectedError: nil,
		},
		{
			name:          "Proposing a valid DKG from Aborted changes state to Proposing",
			startingState: NewFullDKGEntry(beaconID, Aborted, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				validProposal := NewValidProposal(beaconID, me)
				validProposal.Epoch = 2

				return in.Proposing(me, validProposal)
			},
			expectedResult: func() *DKGState {
				proposal := NewValidProposal(beaconID, me)
				return &DKGState{
					BeaconID:   beaconID,
					Epoch:      2,
					State:      Proposing,
					Threshold:  proposal.Threshold,
					Timeout:    proposal.Timeout.AsTime(),
					Leader:     me,
					Remaining:  proposal.Remaining,
					Joining:    nil,
					Leaving:    nil,
					FinalGroup: nil,
				}
			}(),
			expectedError: nil,
		},
		{
			name:          "Proposing a valid DKG after Timeout changes state to Proposing",
			startingState: NewFullDKGEntry(beaconID, TimedOut, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				validProposal := NewValidProposal(beaconID, me)
				validProposal.Epoch = 2

				return in.Proposing(me, validProposal)
			},
			expectedResult: func() *DKGState {
				proposal := NewValidProposal(beaconID, me)
				return &DKGState{
					BeaconID:   beaconID,
					Epoch:      2,
					State:      Proposing,
					Threshold:  proposal.Threshold,
					Timeout:    proposal.Timeout.AsTime(),
					Leader:     me,
					Remaining:  proposal.Remaining,
					Joining:    nil,
					Leaving:    nil,
					FinalGroup: nil,
				}
			}(),
			expectedError: nil,
		},
		{
			name:          "cannot propose a DKG when already joined",
			startingState: NewFullDKGEntry(beaconID, Joined, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},
			expectedError: InvalidStateChange(Joined, Proposing),
		},
		{
			name:          "proposing a DKG when leaving returns error",
			startingState: NewFullDKGEntry(beaconID, Left, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},
			expectedError: InvalidStateChange(Left, Proposing),
		},
		{
			name:          "proposing a DKG when already proposing returns an error",
			startingState: NewFullDKGEntry(beaconID, Proposing, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},
			expectedError: InvalidStateChange(Proposing, Proposing),
		},
		{
			name:          "proposing a DKG when one has already been proposed returns an error",
			startingState: NewFullDKGEntry(beaconID, Proposed, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},

			expectedError: InvalidStateChange(Proposed, Proposing),
		},
		{
			name:          "proposing a DKG during execution returns an error",
			startingState: NewFullDKGEntry(beaconID, Executing, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},
			expectedError: InvalidStateChange(Executing, Proposing),
		},
		{
			name:          "proposing a DKG after acceptance returns an error",
			startingState: NewFullDKGEntry(beaconID, Accepted, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},
			expectedError: InvalidStateChange(Accepted, Proposing),
		},
		{
			name:          "proposing a DKG after rejection returns an error",
			startingState: NewFullDKGEntry(beaconID, Rejected, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},
			expectedError: InvalidStateChange(Rejected, Proposing),
		},
	}

	RunStateChangeTest(t, tests)
}

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
			transitionFn: func(in *DKGState) (*DKGState, error) {
				validProposal := NewValidProposal(beaconID, anotherNode, me)
				validProposal.Epoch = 2

				return in.Proposed(anotherNode, me, validProposal)
			},
			expectedResult: func() *DKGState {
				proposal := NewValidProposal(beaconID, anotherNode, me)
				return &DKGState{
					BeaconID:   beaconID,
					Epoch:      2,
					State:      Proposed,
					Threshold:  proposal.Threshold,
					Timeout:    proposal.Timeout.AsTime(),
					Leader:     anotherNode,
					Remaining:  proposal.Remaining,
					Joining:    proposal.Joining,
					Leaving:    proposal.Leaving,
					FinalGroup: nil,
				}
			}(),
			expectedError: nil,
		},
		{
			name:          "Being proposed a valid DKG with epoch 1 from Fresh state changes state to Proposed",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Proposed(anotherNode, me, NewInitialProposal(beaconID, anotherNode, me))
			},
			expectedResult: func() *DKGState {
				proposal := NewInitialProposal(beaconID, anotherNode, me)
				return &DKGState{
					BeaconID:   beaconID,
					Epoch:      1,
					State:      Proposed,
					Threshold:  proposal.Threshold,
					Timeout:    proposal.Timeout.AsTime(),
					Leader:     anotherNode,
					Remaining:  proposal.Remaining,
					Joining:    proposal.Joining,
					Leaving:    proposal.Leaving,
					FinalGroup: nil,
				}
			}(),
			expectedError: nil,
		},
		{
			name:          "Being proposed a valid DKG but without me included in some way returns an error",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Proposed(anotherNode, me, NewInitialProposal(beaconID, anotherNode))
			},
			expectedError: SelfMissingFromProposal,
		},
		{
			name:          "Being proposed a valid DKG with epoch > 1 from Fresh state changes state to Proposed",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Proposed(anotherNode, me, NewValidProposal(beaconID, anotherNode, me))
			},
			expectedResult: func() *DKGState {
				proposal := NewValidProposal(beaconID, anotherNode, me)
				return &DKGState{
					BeaconID:   beaconID,
					Epoch:      proposal.Epoch,
					State:      Proposed,
					Threshold:  proposal.Threshold,
					Timeout:    proposal.Timeout.AsTime(),
					Leader:     anotherNode,
					Remaining:  proposal.Remaining,
					Joining:    proposal.Joining,
					Leaving:    nil,
					FinalGroup: nil,
				}
			}(),
			expectedError: nil,
		},
		{
			name:          "Being proposed a valid DKG from state Executing returns an error",
			startingState: NewFullDKGEntry(beaconID, Executing, anotherNode),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Proposed(anotherNode, me, NewValidProposal(beaconID, anotherNode))
			},
			expectedError: InvalidStateChange(Executing, Proposed),
		},
		{
			name:          "Being proposed a DKG by somebody who isn't the leader returns an error",
			startingState: NewFullDKGEntry(beaconID, Aborted, anotherNode),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				aThirdParty := &drand.Participant{Address: "another-party-entirely"}
				return in.Proposed(aThirdParty, me, NewValidProposal(beaconID, anotherNode))
			},
			expectedError: CannotProposeAsNonLeader,
		},
		{
			name:          "Being proposed an otherwise invalid DKG returns an error",
			startingState: NewFullDKGEntry(beaconID, Aborted, anotherNode),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				validProposal := NewValidProposal(beaconID, anotherNode)
				validProposal.Epoch = 0

				return in.Proposed(anotherNode, me, validProposal)
			},
			expectedError: InvalidEpoch,
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
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Accepted(me)
			},
			expectedResult: NewFullDKGEntry(beaconID, Accepted, &leader, me),
			expectedError:  nil,
		},
		{
			name:          "cannot accept a fresh proposal",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Accepted(me)
			},
			expectedError: InvalidStateChange(Fresh, Accepted),
		},
		{
			name:          "cannot accept own proposal",
			startingState: NewFullDKGEntry(beaconID, Proposing, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Accepted(me)
			},
			expectedError: InvalidStateChange(Proposing, Accepted),
		},
		{
			name:          "cannot accept a proposal i've already rejected",
			startingState: NewFullDKGEntry(beaconID, Rejected, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Accepted(me)
			},
			expectedError: InvalidStateChange(Rejected, Accepted),
		},
		{
			name:          "cannot accept a proposal that has already timed out",
			startingState: PastTimeout(NewFullDKGEntry(beaconID, Proposed, me)),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Accepted(me)
			},
			expectedError: TimeoutReached,
		},
		{
			name: "cannot accept a proposal where I am leaving",
			startingState: func() *DKGState {
				details := NewFullDKGEntry(beaconID, Proposed, &leader)
				details.Leaving = []*drand.Participant{me}
				return details
			}(),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Accepted(me)
			},
			expectedError: CannotAcceptProposalWhereLeaving,
		},
		{
			name: "cannot accept a proposal where I am joining",
			startingState: func() *DKGState {
				details := NewFullDKGEntry(beaconID, Proposed, &leader)
				details.Joining = []*drand.Participant{me}
				return details
			}(),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Accepted(me)
			},
			expectedError: CannotAcceptProposalWhereJoining,
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
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Rejected(me)
			},
			expectedResult: NewFullDKGEntry(beaconID, Rejected, &leader, me),
			expectedError:  nil,
		},
		{
			name:          "cannot reject a fresh proposal",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Rejected(me)
			},
			expectedError: InvalidStateChange(Fresh, Rejected),
		},
		{
			name:          "cannot reject own proposal",
			startingState: NewFullDKGEntry(beaconID, Proposing, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Rejected(me)
			},
			expectedError: InvalidStateChange(Proposing, Rejected),
		},
		{
			name:          "cannot rejected a proposal i've already accepted",
			startingState: NewFullDKGEntry(beaconID, Accepted, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Rejected(me)
			},
			expectedError: InvalidStateChange(Accepted, Rejected),
		},
		{
			name:          "cannot reject a proposal that has already timed out",
			startingState: PastTimeout(NewFullDKGEntry(beaconID, Proposed, me)),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Rejected(me)
			},
			expectedError: TimeoutReached,
		},
		{
			name: "cannot reject a proposal where I am leaving",
			startingState: func() *DKGState {
				details := NewFullDKGEntry(beaconID, Proposed, &leader)
				details.Leaving = []*drand.Participant{me}
				return details
			}(),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Rejected(me)
			},
			expectedError: CannotRejectProposalWhereLeaving,
		},
		{
			name: "cannot reject a proposal where I am joining",
			startingState: func() *DKGState {
				details := NewFullDKGEntry(beaconID, Proposed, &leader)
				details.Joining = []*drand.Participant{me}
				return details
			}(),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Rejected(me)
			},
			expectedError: CannotRejectProposalWhereJoining,
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
			startingState: func() *DKGState {
				details := NewFullDKGEntry(beaconID, Proposed, &leader)
				details.Leaving = []*drand.Participant{
					me,
				}
				return details
			}(),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Left(me)
			},
			expectedResult: func() *DKGState {
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
			startingState: func() *DKGState {
				details := NewFullDKGEntry(beaconID, Joined, &leader)
				details.Joining = []*drand.Participant{
					me,
				}
				return details
			}(),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Left(me)
			},
			expectedResult: func() *DKGState {
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
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Left(me)
			},
			expectedError: CannotLeaveIfNotALeaver,
		},
		{
			name: "attempting to leave if timeout has been reached returns an error",
			startingState: func() *DKGState {
				details := PastTimeout(NewFullDKGEntry(beaconID, Proposed, &leader))
				details.Leaving = []*drand.Participant{
					me,
				}
				return details
			}(),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Left(me)
			},
			expectedError: TimeoutReached,
		},
		{
			name: "cannot leave if proposal already complete",
			startingState: func() *DKGState {
				details := NewFullDKGEntry(beaconID, Complete, &leader)
				details.Leaving = []*drand.Participant{
					me,
				}
				return details
			}(),
			transitionFn: func(in *DKGState) (*DKGState, error) {
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
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Executing(me)
			},
			expectedResult: NewFullDKGEntry(beaconID, Executing, &leader, me),
			expectedError:  nil,
		},
		{
			name:          "executing a valid proposal that I have rejected returns an error",
			startingState: NewFullDKGEntry(beaconID, Rejected, &leader, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Executing(me)
			},
			expectedError: InvalidStateChange(Rejected, Executing),
		},
		{
			name:          "executing a proposal after time out returns an error",
			startingState: PastTimeout(NewFullDKGEntry(beaconID, Accepted, &leader, me)),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Executing(me)
			},
			expectedError: TimeoutReached,
		},
		{
			name:          "executing a valid proposal that I am not joining or remaining in returns an error (but shouldn't have been possible anyway)",
			startingState: NewFullDKGEntry(beaconID, Accepted, &leader),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Executing(me)
			},
			expectedError: CannotExecuteIfNotJoinerOrRemainer,
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

	finalGroup := []*drand.Participant{
		me, &leader,
	}

	tests := []stateChangeTableTest{
		{
			name:          "completing a valid executing proposal succeeds",
			startingState: NewFullDKGEntry(beaconID, Executing, &leader, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Complete(finalGroup)
			},
			expectedResult: func() *DKGState {
				d := NewFullDKGEntry(beaconID, Complete, &leader, me)
				d.FinalGroup = finalGroup
				return d
			}(),
			expectedError: nil,
		},
		{
			name:          "completing a non-executing proposal returns an error",
			startingState: NewFullDKGEntry(beaconID, Accepted, &leader, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Complete(finalGroup)
			},
			expectedError: InvalidStateChange(Accepted, Complete),
		},
		{
			name:          "completing a proposal after time out returns an error",
			startingState: PastTimeout(NewFullDKGEntry(beaconID, Executing, &leader, me)),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.Complete(finalGroup)
			},
			expectedError: TimeoutReached,
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
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.ReceivedAcceptance(me, &somebodyElse)
			},
			expectedResult: func() *DKGState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Acceptors = []*drand.Participant{&somebodyElse}
				return d
			}(),
			expectedError: nil,
		},
		{
			name:          "receiving an acceptance for a proposal I didn't make returns an error (shouldn't be possible)",
			startingState: NewFullDKGEntry(beaconID, Proposing, &somebodyElse, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.ReceivedAcceptance(me, &somebodyElse)
			},
			expectedError: NonLeaderCannotReceiveAcceptance,
		},
		{
			name:          "receiving an acceptance from somebody who isn't a remainer returns an error",
			startingState: NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.ReceivedAcceptance(me, &drand.Participant{Address: "who is this?!?"})
			},
			expectedError: UnknownAcceptor,
		},
		{
			name:          "receiving acceptance from non-proposing state returns an error",
			startingState: NewFullDKGEntry(beaconID, Proposed, me, &somebodyElse),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.ReceivedAcceptance(me, &somebodyElse)
			},
			expectedError: InvalidStateChange(Proposed, Proposing),
		},
		{
			name: "acceptances are appended to acceptors",
			startingState: func() *DKGState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Acceptors = []*drand.Participant{
					&aThirdParty,
				}
				return d
			}(),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.ReceivedAcceptance(me, &somebodyElse)
			},
			expectedResult: func() *DKGState {
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
			startingState: func() *DKGState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Acceptors = []*drand.Participant{
					&somebodyElse,
				}
				return d
			}(),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.ReceivedAcceptance(me, &somebodyElse)
			},
			expectedError: DuplicateAcceptance,
		},
		{
			name: "if a party has rejected and they send an acceptance, they are moved into acceptance",
			startingState: func() *DKGState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Rejectors = []*drand.Participant{
					&somebodyElse,
				}
				return d
			}(),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.ReceivedAcceptance(me, &somebodyElse)
			},
			expectedResult: func() *DKGState {
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
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.ReceivedRejection(me, &somebodyElse)
			},
			expectedResult: func() *DKGState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Rejectors = []*drand.Participant{&somebodyElse}
				return d
			}(),
			expectedError: nil,
		},
		{
			name:          "receiving a rejection for a proposal I didn't make returns an error (shouldn't be possible)",
			startingState: NewFullDKGEntry(beaconID, Proposing, &somebodyElse, me),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.ReceivedRejection(me, &somebodyElse)
			},
			expectedError: NonLeaderCannotReceiveRejection,
		},
		{
			name:          "receiving a rejection from somebody who isn't a remainer returns an error",
			startingState: NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.ReceivedRejection(me, &drand.Participant{Address: "who is this?!?"})
			},
			expectedError: UnknownRejector,
		},
		{
			name:          "receiving rejection from non-proposing state returns an error",
			startingState: NewFullDKGEntry(beaconID, Proposed, me, &somebodyElse),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.ReceivedRejection(me, &somebodyElse)
			},
			expectedError: InvalidStateChange(Proposed, Proposing),
		},
		{
			name: "rejections are appended to rejectors",
			startingState: func() *DKGState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Rejectors = []*drand.Participant{
					&aThirdParty,
				}
				return d
			}(),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.ReceivedRejection(me, &somebodyElse)
			},
			expectedResult: func() *DKGState {
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
			startingState: func() *DKGState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Rejectors = []*drand.Participant{
					&somebodyElse,
				}
				return d
			}(),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.ReceivedRejection(me, &somebodyElse)
			},
			expectedError: DuplicateRejection,
		},
		{
			name: "if a party has accepted and they send a rejection, they are moved into rejectors",
			startingState: func() *DKGState {
				d := NewFullDKGEntry(beaconID, Proposing, me, &somebodyElse)
				d.Acceptors = []*drand.Participant{
					&somebodyElse,
				}
				return d
			}(),
			transitionFn: func(in *DKGState) (*DKGState, error) {
				return in.ReceivedRejection(me, &somebodyElse)
			},
			expectedResult: func() *DKGState {
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
	startingState  *DKGState
	expectedResult *DKGState
	expectedError  error
	transitionFn   func(starting *DKGState) (*DKGState, error)
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
	return &drand.Participant{
		Address: "somewhere.com",
		Tls:     false,
		PubKey:  []byte("deadbeef"),
	}
}

func NewFullDKGEntry(beaconID string, status DKGStatus, previousLeader *drand.Participant, others ...*drand.Participant) *DKGState {
	return &DKGState{
		BeaconID:   beaconID,
		Epoch:      1,
		State:      status,
		Threshold:  1,
		Timeout:    time.Unix(2549084715, 0), // this will need updated in 2050 :^)
		Leader:     previousLeader,
		Remaining:  append([]*drand.Participant{previousLeader}, others...),
		Joining:    nil,
		Leaving:    nil,
		FinalGroup: append([]*drand.Participant{previousLeader}, others...),
		Acceptors:  nil,
		Rejectors:  nil,
	}
}

func NewInitialProposal(beaconID string, leader *drand.Participant, others ...*drand.Participant) *drand.ProposalTerms {
	return &drand.ProposalTerms{
		BeaconID:  beaconID,
		Threshold: 1,
		Epoch:     1,
		Timeout:   timestamppb.New(time.Unix(2549084715, 0)), // this will need updated in 2050 :^)
		Leader:    leader,
		Joining:   append([]*drand.Participant{leader}, others...),
	}
}

func NewValidProposal(beaconID string, leader *drand.Participant, others ...*drand.Participant) *drand.ProposalTerms {
	return &drand.ProposalTerms{
		BeaconID:  beaconID,
		Threshold: 1,
		Epoch:     2,
		Timeout:   timestamppb.New(time.Unix(2549084715, 0)), // this will need updated in 2050 :^)
		Leader:    leader,
		Remaining: append([]*drand.Participant{leader}, others...),
	}
}

func PastTimeout(d *DKGState) *DKGState {
	d.Timeout = time.Now().Add(-1 * time.Minute)
	return d
}
