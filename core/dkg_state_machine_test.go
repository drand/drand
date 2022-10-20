package core

import (
	"github.com/drand/drand/protobuf/drand"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestProposalValidation(t *testing.T) {
	beaconID := "default"
	me := NewParticipant()
	current := NewFullDKGDetails(beaconID, Complete, me)
	tests := []struct {
		name     string
		state    *DKGDetails
		terms    *ProposalTerms
		expected DKGErrorCode
	}{
		{
			name:  "valid proposal returns no error",
			state: current,
			terms: func() *ProposalTerms {
				proposal := NewValidProposal(beaconID, me)
				proposal.Leader = current.Leader
				proposal.Remaining = []*drand.Participant{
					current.Leader,
				}
				return proposal
			}(),
			expected: NoError,
		},
		{
			name:  "timeout in the past returns error",
			state: current,
			terms: func() *ProposalTerms {
				proposal := NewValidProposal(beaconID, me)
				proposal.Timeout = time.Now().Add(-10 * time.Hour)
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
			terms: func() *ProposalTerms {
				proposal := NewValidProposal(beaconID, me)
				proposal.Epoch = 0
				return proposal
			}(),
			expected: InvalidEpoch,
		},
		{
			name:  "if epoch is 1, nodes remaining returns an error",
			state: NewFreshState(beaconID),
			terms: func() *ProposalTerms {
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
			terms: func() *ProposalTerms {
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
			terms: func() *ProposalTerms {
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
			terms: func() *ProposalTerms {
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
			terms: func() *ProposalTerms {
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
			terms: func() *ProposalTerms {
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
			terms: func() *ProposalTerms {
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
			terms: func() *ProposalTerms {
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
			terms: func() *ProposalTerms {
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
			state: func() *DKGDetails {
				details := NewFullDKGDetails(beaconID, Left, me)
				details.Epoch = 2
				return details
			}(),
			terms: func() *ProposalTerms {
				validProposal := NewValidProposal(beaconID, me)
				validProposal.Epoch = 5
				validProposal.Leader = current.Leader
				validProposal.Remaining = []*drand.Participant{
					current.Leader,
				}
				return validProposal
			}(),
			expected: NoError,
		},
		{
			name: "if current status is not Left, a proposed epoch of 1 higher succeeds",
			state: func() *DKGDetails {
				details := NewFullDKGDetails(beaconID, Complete, me)
				details.Epoch = 2
				return details
			}(),
			terms: func() *ProposalTerms {
				validProposal := NewValidProposal(beaconID, me)
				validProposal.Epoch = 3
				return validProposal
			}(),
			expected: NoError,
		},
		{
			name: "if current status is not Left, a proposed epoch of > 1 higher returns an error",
			state: func() *DKGDetails {
				details := NewFullDKGDetails(beaconID, Complete, me)
				details.Epoch = 2
				return details
			}(),
			terms: func() *ProposalTerms {
				invalidEpochProposal := NewValidProposal(beaconID, me)
				invalidEpochProposal.Epoch = 4
				return invalidEpochProposal
			}(),
			expected: InvalidEpoch,
		},
		{
			name: "proposed epoch less than the current epoch returns an error",
			state: func() *DKGDetails {
				details := NewFullDKGDetails(beaconID, Complete, me)
				details.Epoch = 3
				return details
			}(),
			terms: func() *ProposalTerms {
				invalidEpochProposal := NewValidProposal(beaconID, me)
				invalidEpochProposal.Epoch = 2
				return invalidEpochProposal
			}(),
			expected: InvalidEpoch,
		},
		{
			name: "proposed epoch equal to the current epoch returns an error",
			state: func() *DKGDetails {
				details := NewFullDKGDetails(beaconID, Left, me)
				details.Epoch = 3
				return details
			}(),
			terms: func() *ProposalTerms {
				invalidEpochProposal := NewValidProposal(beaconID, me)
				invalidEpochProposal.Epoch = 3
				return invalidEpochProposal
			}(),
			expected: InvalidEpoch,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ValidateProposal(*test.state, test.terms)
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
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.TimedOut()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange,
		},
		{
			name:          "complete state cannot time out",
			startingState: NewFullDKGDetails(beaconID, Complete, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.TimedOut()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange,
		},
		{
			name:          "timed out state cannot time out",
			startingState: NewFullDKGDetails(beaconID, TimedOut, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.TimedOut()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange,
		},
		{
			name:          "aborted state cannot time out",
			startingState: NewFullDKGDetails(beaconID, Aborted, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.TimedOut()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange,
		},
		{
			name:          "left state cannot time out",
			startingState: NewFullDKGDetails(beaconID, Aborted, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.TimedOut()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange,
		},
		{
			name:          "joined state can time out and changes state",
			startingState: NewFullDKGDetails(beaconID, Joined, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.TimedOut()
			},
			expectedResult: NewFullDKGDetails(beaconID, TimedOut, me),
			expectedError:  NoError,
		},
		{
			name:          "proposed state can time out and changes state",
			startingState: NewFullDKGDetails(beaconID, Proposed, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.TimedOut()
			},
			expectedResult: NewFullDKGDetails(beaconID, TimedOut, me),
			expectedError:  NoError,
		},
		{
			name:          "proposing state can time out and changes state",
			startingState: NewFullDKGDetails(beaconID, Proposing, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.TimedOut()
			},
			expectedResult: NewFullDKGDetails(beaconID, TimedOut, me),
			expectedError:  NoError,
		},
		{
			name:          "executing state cannot time out and changes state",
			startingState: NewFullDKGDetails(beaconID, Executing, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.TimedOut()
			},
			expectedResult: NewFullDKGDetails(beaconID, TimedOut, me),
			expectedError:  NoError,
		},
		{
			name:          "accepted state can time out and changes state",
			startingState: NewFullDKGDetails(beaconID, Accepted, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.TimedOut()
			},
			expectedResult: NewFullDKGDetails(beaconID, TimedOut, me),
			expectedError:  NoError,
		},
		{
			name:          "rejected state can time out and changes state",
			startingState: NewFullDKGDetails(beaconID, Rejected, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.TimedOut()
			},
			expectedResult: NewFullDKGDetails(beaconID, TimedOut, me),
			expectedError:  NoError,
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
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Aborted()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange,
		},
		{
			name:          "complete state cannot be aborted",
			startingState: NewFullDKGDetails(beaconID, Complete, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Aborted()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange,
		},
		{
			name:          "timed out state cannot be aborted",
			startingState: NewFullDKGDetails(beaconID, TimedOut, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Aborted()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange,
		},
		{
			name:          "aborted state cannot be aborted",
			startingState: NewFullDKGDetails(beaconID, Aborted, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Aborted()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange,
		},
		{
			name:          "left state can be aborted and changes state",
			startingState: NewFullDKGDetails(beaconID, Left, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Aborted()
			},
			expectedResult: NewFullDKGDetails(beaconID, Aborted, me),
			expectedError:  NoError,
		},
		{
			name:          "joined state can be aborted and changes state",
			startingState: NewFullDKGDetails(beaconID, Joined, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Aborted()
			},
			expectedResult: NewFullDKGDetails(beaconID, Aborted, me),
			expectedError:  NoError,
		},
		{
			name:          "proposed state can be aborted and changes state",
			startingState: NewFullDKGDetails(beaconID, Proposed, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Aborted()
			},
			expectedResult: NewFullDKGDetails(beaconID, Aborted, me),
			expectedError:  NoError,
		},
		{
			name:          "proposing state can be aborted and changes state",
			startingState: NewFullDKGDetails(beaconID, Proposing, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Aborted()
			},
			expectedResult: NewFullDKGDetails(beaconID, Aborted, me),
			expectedError:  NoError,
		},
		{
			name:          "executing state cannot be aborted",
			startingState: NewFullDKGDetails(beaconID, Executing, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Aborted()
			},
			expectedResult: nil,
			expectedError:  InvalidStateChange,
		},
		{
			name:          "accepted state can be aborted and changes state",
			startingState: NewFullDKGDetails(beaconID, Accepted, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Aborted()
			},
			expectedResult: NewFullDKGDetails(beaconID, Aborted, me),
			expectedError:  NoError,
		},
		{
			name:          "rejected state can be aborted and changes state",
			startingState: NewFullDKGDetails(beaconID, Rejected, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Aborted()
			},
			expectedResult: NewFullDKGDetails(beaconID, Aborted, me),
			expectedError:  NoError,
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
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Joined(me, NewInitialProposal(beaconID, &leader, me))
			},
			expectedResult: func() *DKGDetails {
				proposal := NewValidProposal(beaconID, &leader)
				return &DKGDetails{
					BeaconID:   beaconID,
					Epoch:      1,
					State:      Joined,
					Threshold:  proposal.Threshold,
					Timeout:    proposal.Timeout,
					Leader:     proposal.Leader,
					Remaining:  nil,
					Joining:    []*drand.Participant{&leader, me},
					Leaving:    nil,
					FinalGroup: nil,
				}
			}(),
			expectedError: NoError,
		},
		{
			name:          "fresh state join fails if self not present in joining",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
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
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Proposing(me, NewInitialProposal(beaconID, me))
			},
			expectedResult: func() *DKGDetails {
				proposal := NewValidProposal(beaconID, me)
				return &DKGDetails{
					BeaconID:   beaconID,
					Epoch:      1,
					State:      Proposing,
					Threshold:  proposal.Threshold,
					Timeout:    proposal.Timeout,
					Leader:     me,
					Remaining:  nil,
					Joining:    []*drand.Participant{me},
					Leaving:    nil,
					FinalGroup: nil,
				}
			}(),
			expectedError: NoError,
		},
		{
			name:          "Proposing an invalid DKG returns an error",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
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
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
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
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
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
			startingState: NewFullDKGDetails(beaconID, Complete, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				validProposal := NewValidProposal(beaconID, me)
				validProposal.Epoch = 2

				return in.Proposing(me, validProposal)
			},
			expectedResult: func() *DKGDetails {
				proposal := NewValidProposal(beaconID, me)
				return &DKGDetails{
					BeaconID:   beaconID,
					Epoch:      2,
					State:      Proposing,
					Threshold:  proposal.Threshold,
					Timeout:    proposal.Timeout,
					Leader:     me,
					Remaining:  proposal.Remaining,
					Joining:    nil,
					Leaving:    nil,
					FinalGroup: nil,
				}
			}(),
			expectedError: NoError,
		},
		{
			name:          "Proposing a valid DKG from Aborted changes state to Proposing",
			startingState: NewFullDKGDetails(beaconID, Aborted, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				validProposal := NewValidProposal(beaconID, me)
				validProposal.Epoch = 2

				return in.Proposing(me, validProposal)
			},
			expectedResult: func() *DKGDetails {
				proposal := NewValidProposal(beaconID, me)
				return &DKGDetails{
					BeaconID:   beaconID,
					Epoch:      2,
					State:      Proposing,
					Threshold:  proposal.Threshold,
					Timeout:    proposal.Timeout,
					Leader:     me,
					Remaining:  proposal.Remaining,
					Joining:    nil,
					Leaving:    nil,
					FinalGroup: nil,
				}
			}(),
			expectedError: NoError,
		},
		{
			name:          "Proposing a valid DKG after Timeout changes state to Proposing",
			startingState: NewFullDKGDetails(beaconID, TimedOut, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				validProposal := NewValidProposal(beaconID, me)
				validProposal.Epoch = 2

				return in.Proposing(me, validProposal)
			},
			expectedResult: func() *DKGDetails {
				proposal := NewValidProposal(beaconID, me)
				return &DKGDetails{
					BeaconID:   beaconID,
					Epoch:      2,
					State:      Proposing,
					Threshold:  proposal.Threshold,
					Timeout:    proposal.Timeout,
					Leader:     me,
					Remaining:  proposal.Remaining,
					Joining:    nil,
					Leaving:    nil,
					FinalGroup: nil,
				}
			}(),
			expectedError: NoError,
		},
		{
			name:          "cannot propose a DKG when already joined",
			startingState: NewFullDKGDetails(beaconID, Joined, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},
			expectedError: InvalidStateChange,
		},
		{
			name:          "proposing a DKG when leaving returns error",
			startingState: NewFullDKGDetails(beaconID, Left, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},
			expectedError: InvalidStateChange,
		},
		{
			name:          "proposing a DKG when already proposing returns an error",
			startingState: NewFullDKGDetails(beaconID, Proposing, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},
			expectedError: InvalidStateChange,
		},
		{
			name:          "proposing a DKG when one has already been proposed returns an error",
			startingState: NewFullDKGDetails(beaconID, Proposed, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},

			expectedError: InvalidStateChange,
		},
		{
			name:          "proposing a DKG during execution returns an error",
			startingState: NewFullDKGDetails(beaconID, Executing, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},
			expectedError: InvalidStateChange,
		},
		{
			name:          "proposing a DKG after acceptance returns an error",
			startingState: NewFullDKGDetails(beaconID, Accepted, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},
			expectedError: InvalidStateChange,
		},
		{
			name:          "proposing a DKG after rejection returns an error",
			startingState: NewFullDKGDetails(beaconID, Rejected, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Proposing(me, NewValidProposal(beaconID, me))
			},
			expectedError: InvalidStateChange,
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
			startingState: NewFullDKGDetails(beaconID, Complete, anotherNode, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				validProposal := NewValidProposal(beaconID, anotherNode, me)
				validProposal.Epoch = 2

				return in.Proposed(anotherNode, me, validProposal)
			},
			expectedResult: func() *DKGDetails {
				proposal := NewValidProposal(beaconID, anotherNode, me)
				return &DKGDetails{
					BeaconID:   beaconID,
					Epoch:      2,
					State:      Proposed,
					Threshold:  proposal.Threshold,
					Timeout:    proposal.Timeout,
					Leader:     anotherNode,
					Remaining:  proposal.Remaining,
					Joining:    proposal.Joining,
					Leaving:    proposal.Leaving,
					FinalGroup: nil,
				}
			}(),
			expectedError: NoError,
		},
		{
			name:          "Being proposed a valid DKG with epoch 1 from Fresh state changes state to Proposed",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Proposed(anotherNode, me, NewInitialProposal(beaconID, anotherNode, me))
			},
			expectedResult: func() *DKGDetails {
				proposal := NewInitialProposal(beaconID, anotherNode, me)
				return &DKGDetails{
					BeaconID:   beaconID,
					Epoch:      1,
					State:      Proposed,
					Threshold:  proposal.Threshold,
					Timeout:    proposal.Timeout,
					Leader:     anotherNode,
					Remaining:  proposal.Remaining,
					Joining:    proposal.Joining,
					Leaving:    proposal.Leaving,
					FinalGroup: nil,
				}
			}(),
			expectedError: NoError,
		},
		{
			name:          "Being proposed a valid DKG but without me included in some way returns an error",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Proposed(anotherNode, me, NewInitialProposal(beaconID, anotherNode))
			},
			expectedError: SelfMissingFromProposal,
		},
		{
			name:          "Being proposed a valid DKG with epoch > 1 from Fresh state changes state to Proposed",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Proposed(anotherNode, me, NewValidProposal(beaconID, anotherNode, me))
			},
			expectedResult: func() *DKGDetails {
				proposal := NewValidProposal(beaconID, anotherNode, me)
				return &DKGDetails{
					BeaconID:   beaconID,
					Epoch:      proposal.Epoch,
					State:      Proposed,
					Threshold:  proposal.Threshold,
					Timeout:    proposal.Timeout,
					Leader:     anotherNode,
					Remaining:  proposal.Remaining,
					Joining:    proposal.Joining,
					Leaving:    nil,
					FinalGroup: nil,
				}
			}(),
			expectedError: NoError,
		},
		{
			name:          "Being proposed a valid DKG from state Executing returns an error",
			startingState: NewFullDKGDetails(beaconID, Executing, anotherNode),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Proposed(anotherNode, me, NewValidProposal(beaconID, anotherNode))
			},
			expectedError: InvalidStateChange,
		},
		{
			name:          "Being proposed a DKG by somebody who isn't the leader returns an error",
			startingState: NewFullDKGDetails(beaconID, Aborted, anotherNode),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				aThirdParty := &drand.Participant{Address: "another-party-entirely"}
				return in.Proposed(aThirdParty, me, NewValidProposal(beaconID, anotherNode))
			},
			expectedError: CannotProposeAsNonLeader,
		},
		{
			name:          "Being proposed an otherwise invalid DKG returns an error",
			startingState: NewFullDKGDetails(beaconID, Aborted, anotherNode),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
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
			startingState: NewFullDKGDetails(beaconID, Proposed, &leader, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Accepted(me)
			},
			expectedResult: NewFullDKGDetails(beaconID, Accepted, &leader, me),
			expectedError:  NoError,
		},
		{
			name:          "cannot accept a fresh proposal",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Accepted(me)
			},
			expectedError: InvalidStateChange,
		},
		{
			name:          "cannot accept own proposal",
			startingState: NewFullDKGDetails(beaconID, Proposing, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Accepted(me)
			},
			expectedError: InvalidStateChange,
		},
		{
			name:          "cannot accept a proposal i've already rejected",
			startingState: NewFullDKGDetails(beaconID, Rejected, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Accepted(me)
			},
			expectedError: InvalidStateChange,
		},
		{
			name:          "cannot accept a proposal that has already timed out",
			startingState: PastTimeout(NewFullDKGDetails(beaconID, Proposed, me)),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Accepted(me)
			},
			expectedError: TimeoutReached,
		},
		{
			name: "cannot accept a proposal where I am leaving",
			startingState: func() *DKGDetails {
				details := NewFullDKGDetails(beaconID, Proposed, &leader)
				details.Leaving = []*drand.Participant{me}
				return details
			}(),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Accepted(me)
			},
			expectedError: CannotAcceptProposalWhereLeaving,
		},
		{
			name: "cannot accept a proposal where I am joining",
			startingState: func() *DKGDetails {
				details := NewFullDKGDetails(beaconID, Proposed, &leader)
				details.Joining = []*drand.Participant{me}
				return details
			}(),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
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
			startingState: NewFullDKGDetails(beaconID, Proposed, &leader, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Rejected(me)
			},
			expectedResult: NewFullDKGDetails(beaconID, Rejected, &leader, me),
			expectedError:  NoError,
		},
		{
			name:          "cannot reject a fresh proposal",
			startingState: NewFreshState(beaconID),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Rejected(me)
			},
			expectedError: InvalidStateChange,
		},
		{
			name:          "cannot reject own proposal",
			startingState: NewFullDKGDetails(beaconID, Proposing, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Rejected(me)
			},
			expectedError: InvalidStateChange,
		},
		{
			name:          "cannot rejected a proposal i've already accepted",
			startingState: NewFullDKGDetails(beaconID, Accepted, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Rejected(me)
			},
			expectedError: InvalidStateChange,
		},
		{
			name:          "cannot reject a proposal that has already timed out",
			startingState: PastTimeout(NewFullDKGDetails(beaconID, Proposed, me)),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Rejected(me)
			},
			expectedError: TimeoutReached,
		},
		{
			name: "cannot reject a proposal where I am leaving",
			startingState: func() *DKGDetails {
				details := NewFullDKGDetails(beaconID, Proposed, &leader)
				details.Leaving = []*drand.Participant{me}
				return details
			}(),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Rejected(me)
			},
			expectedError: CannotRejectProposalWhereLeaving,
		},
		{
			name: "cannot reject a proposal where I am joining",
			startingState: func() *DKGDetails {
				details := NewFullDKGDetails(beaconID, Proposed, &leader)
				details.Joining = []*drand.Participant{me}
				return details
			}(),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
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
			startingState: func() *DKGDetails {
				details := NewFullDKGDetails(beaconID, Proposed, &leader)
				details.Leaving = []*drand.Participant{
					me,
				}
				return details
			}(),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Left(me)
			},
			expectedResult: func() *DKGDetails {
				details := NewFullDKGDetails(beaconID, Left, &leader)
				details.Leaving = []*drand.Participant{
					me,
				}
				return details
			}(),
			expectedError: NoError,
		},
		{
			name: "can leave valid proposal immediately if I have just joined it",
			startingState: func() *DKGDetails {
				details := NewFullDKGDetails(beaconID, Joined, &leader)
				details.Joining = []*drand.Participant{
					me,
				}
				return details
			}(),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Left(me)
			},
			expectedResult: func() *DKGDetails {
				details := NewFullDKGDetails(beaconID, Left, &leader)
				details.Joining = []*drand.Participant{
					me,
				}
				return details
			}(),
			expectedError: NoError,
		},
		{
			name:          "trying to leave if not a leaver returns an error",
			startingState: NewFullDKGDetails(beaconID, Proposed, &leader, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Left(me)
			},
			expectedError: CannotLeaveIfNotALeaver,
		},
		{
			name: "attempting to leave if timeout has been reached returns an error",
			startingState: func() *DKGDetails {
				details := PastTimeout(NewFullDKGDetails(beaconID, Proposed, &leader))
				details.Leaving = []*drand.Participant{
					me,
				}
				return details
			}(),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Left(me)
			},
			expectedError: TimeoutReached,
		},
		{
			name: "cannot leave if proposal already complete",
			startingState: func() *DKGDetails {
				details := NewFullDKGDetails(beaconID, Complete, &leader)
				details.Leaving = []*drand.Participant{
					me,
				}
				return details
			}(),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Left(me)
			},
			expectedError: InvalidStateChange,
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
			startingState: NewFullDKGDetails(beaconID, Accepted, &leader, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Executing(me)
			},
			expectedResult: NewFullDKGDetails(beaconID, Executing, &leader, me),
			expectedError:  NoError,
		},
		{
			name:          "executing a valid proposal that I have rejected returns an error",
			startingState: NewFullDKGDetails(beaconID, Rejected, &leader, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Executing(me)
			},
			expectedError: InvalidStateChange,
		},
		{
			name:          "executing a proposal after time out returns an error",
			startingState: PastTimeout(NewFullDKGDetails(beaconID, Accepted, &leader, me)),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Executing(me)
			},
			expectedError: TimeoutReached,
		},
		{
			name:          "executing a valid proposal that I am not joining or remaining in returns an error (but shouldn't have been possible anyway)",
			startingState: NewFullDKGDetails(beaconID, Accepted, &leader),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
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
			startingState: NewFullDKGDetails(beaconID, Executing, &leader, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Complete(finalGroup)
			},
			expectedResult: func() *DKGDetails {
				d := NewFullDKGDetails(beaconID, Complete, &leader, me)
				d.FinalGroup = finalGroup
				return d
			}(),
			expectedError: NoError,
		},
		{
			name:          "completing a non-executing proposal returns an error",
			startingState: NewFullDKGDetails(beaconID, Accepted, &leader, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.Complete(finalGroup)
			},
			expectedError: InvalidStateChange,
		},
		{
			name:          "completing a proposal after time out returns an error",
			startingState: PastTimeout(NewFullDKGDetails(beaconID, Executing, &leader, me)),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
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
			startingState: NewFullDKGDetails(beaconID, Proposing, me, &somebodyElse),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.ReceivedAcceptance(me, &somebodyElse)
			},
			expectedResult: func() *DKGDetails {
				d := NewFullDKGDetails(beaconID, Proposing, me, &somebodyElse)
				d.Acceptors = []*drand.Participant{&somebodyElse}
				return d
			}(),
			expectedError: NoError,
		},
		{
			name:          "receiving an acceptance for a proposal I didn't make returns an error (shouldn't be possible)",
			startingState: NewFullDKGDetails(beaconID, Proposing, &somebodyElse, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.ReceivedAcceptance(me, &somebodyElse)
			},
			expectedError: NonLeaderCannotReceiveAcceptance,
		},
		{
			name:          "receiving an acceptance from somebody who isn't a remainer returns an error",
			startingState: NewFullDKGDetails(beaconID, Proposing, me, &somebodyElse),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.ReceivedAcceptance(me, &drand.Participant{Address: "who is this?!?"})
			},
			expectedError: UnknownAcceptor,
		},
		{
			name:          "receiving acceptance from non-proposing state returns an error",
			startingState: NewFullDKGDetails(beaconID, Proposed, me, &somebodyElse),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.ReceivedAcceptance(me, &somebodyElse)
			},
			expectedError: InvalidStateChange,
		},
		{
			name: "acceptances are appended to acceptors",
			startingState: func() *DKGDetails {
				d := NewFullDKGDetails(beaconID, Proposing, me, &somebodyElse)
				d.Acceptors = []*drand.Participant{
					&aThirdParty,
				}
				return d
			}(),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.ReceivedAcceptance(me, &somebodyElse)
			},
			expectedResult: func() *DKGDetails {
				d := NewFullDKGDetails(beaconID, Proposing, me, &somebodyElse)
				d.Acceptors = []*drand.Participant{
					&aThirdParty,
					&somebodyElse,
				}
				return d
			}(),
			expectedError: NoError,
		},
		{
			name: "duplicate acceptance returns an error",
			startingState: func() *DKGDetails {
				d := NewFullDKGDetails(beaconID, Proposing, me, &somebodyElse)
				d.Acceptors = []*drand.Participant{
					&somebodyElse,
				}
				return d
			}(),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.ReceivedAcceptance(me, &somebodyElse)
			},
			expectedError: DuplicateAcceptance,
		},
		{
			name: "if a party has rejected and they send an acceptance, they are moved into acceptance",
			startingState: func() *DKGDetails {
				d := NewFullDKGDetails(beaconID, Proposing, me, &somebodyElse)
				d.Rejectors = []*drand.Participant{
					&somebodyElse,
				}
				return d
			}(),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.ReceivedAcceptance(me, &somebodyElse)
			},
			expectedResult: func() *DKGDetails {
				d := NewFullDKGDetails(beaconID, Proposing, me, &somebodyElse)
				d.Acceptors = []*drand.Participant{
					&somebodyElse,
				}
				return d
			}(),
			expectedError: NoError,
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
			startingState: NewFullDKGDetails(beaconID, Proposing, me, &somebodyElse),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.ReceivedRejection(me, &somebodyElse)
			},
			expectedResult: func() *DKGDetails {
				d := NewFullDKGDetails(beaconID, Proposing, me, &somebodyElse)
				d.Rejectors = []*drand.Participant{&somebodyElse}
				return d
			}(),
			expectedError: NoError,
		},
		{
			name:          "receiving a rejection for a proposal I didn't make returns an error (shouldn't be possible)",
			startingState: NewFullDKGDetails(beaconID, Proposing, &somebodyElse, me),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.ReceivedRejection(me, &somebodyElse)
			},
			expectedError: NonLeaderCannotReceiveRejection,
		},
		{
			name:          "receiving a rejection from somebody who isn't a remainer returns an error",
			startingState: NewFullDKGDetails(beaconID, Proposing, me, &somebodyElse),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.ReceivedRejection(me, &drand.Participant{Address: "who is this?!?"})
			},
			expectedError: UnknownRejector,
		},
		{
			name:          "receiving rejection from non-proposing state returns an error",
			startingState: NewFullDKGDetails(beaconID, Proposed, me, &somebodyElse),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.ReceivedRejection(me, &somebodyElse)
			},
			expectedError: InvalidStateChange,
		},
		{
			name: "rejections are appended to rejectors",
			startingState: func() *DKGDetails {
				d := NewFullDKGDetails(beaconID, Proposing, me, &somebodyElse)
				d.Rejectors = []*drand.Participant{
					&aThirdParty,
				}
				return d
			}(),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.ReceivedRejection(me, &somebodyElse)
			},
			expectedResult: func() *DKGDetails {
				d := NewFullDKGDetails(beaconID, Proposing, me, &somebodyElse)
				d.Rejectors = []*drand.Participant{
					&aThirdParty,
					&somebodyElse,
				}
				return d
			}(),
			expectedError: NoError,
		},
		{
			name: "duplicate rejection returns an error",
			startingState: func() *DKGDetails {
				d := NewFullDKGDetails(beaconID, Proposing, me, &somebodyElse)
				d.Rejectors = []*drand.Participant{
					&somebodyElse,
				}
				return d
			}(),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.ReceivedRejection(me, &somebodyElse)
			},
			expectedError: DuplicateRejection,
		},
		{
			name: "if a party has accepted and they send a rejection, they are moved into rejectors",
			startingState: func() *DKGDetails {
				d := NewFullDKGDetails(beaconID, Proposing, me, &somebodyElse)
				d.Acceptors = []*drand.Participant{
					&somebodyElse,
				}
				return d
			}(),
			transitionFn: func(in *DKGDetails) (*DKGDetails, DKGErrorCode) {
				return in.ReceivedRejection(me, &somebodyElse)
			},
			expectedResult: func() *DKGDetails {
				d := NewFullDKGDetails(beaconID, Proposing, me, &somebodyElse)
				d.Rejectors = []*drand.Participant{
					&somebodyElse,
				}
				return d
			}(),
			expectedError: NoError,
		},
	}

	RunStateChangeTest(t, tests)
}

type stateChangeTableTest struct {
	name           string
	startingState  *DKGDetails
	expectedResult *DKGDetails
	expectedError  DKGErrorCode
	transitionFn   func(starting *DKGDetails) (*DKGDetails, DKGErrorCode)
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
		Address:   "somewhere.com",
		Tls:       false,
		PubKey:    []byte("deadbeef"),
		Signature: []byte("deadbeef"),
	}
}

func NewFullDKGDetails(beaconID string, status DKGStatus, previousLeader *drand.Participant, others ...*drand.Participant) *DKGDetails {
	return &DKGDetails{
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

func NewInitialProposal(beaconID string, leader *drand.Participant, others ...*drand.Participant) *ProposalTerms {
	return &ProposalTerms{
		BeaconID:  beaconID,
		Threshold: 1,
		Epoch:     1,
		Timeout:   time.Unix(2549084715, 0), // this will need updated in 2050 :^)
		Leader:    leader,
		Joining:   append([]*drand.Participant{leader}, others...),
	}
}

func NewValidProposal(beaconID string, leader *drand.Participant, others ...*drand.Participant) *ProposalTerms {
	return &ProposalTerms{
		BeaconID:  beaconID,
		Threshold: 1,
		Epoch:     2,
		Timeout:   time.Unix(2549084715, 0), // this will need updated in 2050 :^)
		Leader:    leader,
		Remaining: append([]*drand.Participant{leader}, others...),
	}
}

func PastTimeout(d *DKGDetails) *DKGDetails {
	d.Timeout = time.Now().Add(-1 * time.Minute)
	return d
}
