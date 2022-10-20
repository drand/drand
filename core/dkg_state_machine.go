package core

import (
	"github.com/drand/drand/protobuf/drand"
	"time"
)

type DKGStatus uint32

const (
	Fresh DKGStatus = iota
	Proposed
	Proposing
	Accepted
	Rejected
	Aborted
	Executing
	Complete
	TimedOut
	Joined
	Left
)

type DKGDetails struct {
	BeaconID  string
	Epoch     uint32
	State     DKGStatus
	Threshold uint32
	Timeout   time.Time
	Leader    *drand.Participant
	Remaining []*drand.Participant
	Joining   []*drand.Participant
	Leaving   []*drand.Participant

	Acceptors []*drand.Participant
	Rejectors []*drand.Participant

	FinalGroup []*drand.Participant
}

func NewFreshState(beaconID string) *DKGDetails {
	return &DKGDetails{
		BeaconID: beaconID,
		State:    Fresh,
		Timeout:  time.Unix(0, 0).UTC(),
	}
}

type ProposalTerms struct {
	BeaconID  string
	Threshold uint32
	Epoch     uint32
	Timeout   time.Time
	Leader    *drand.Participant

	Remaining []*drand.Participant
	Joining   []*drand.Participant
	Leaving   []*drand.Participant
}

func (d DKGDetails) Joined(me *drand.Participant, terms *ProposalTerms) (*DKGDetails, DKGErrorCode) {
	if !isValidStateChange(d.State, Joined) {
		return nil, InvalidStateChange
	}

	if err := ValidateProposal(d, terms); err != NoError {
		return nil, err
	}

	if !contains(terms.Joining, me) {
		return nil, SelfMissingFromProposal
	}

	return &DKGDetails{
		BeaconID:  d.BeaconID,
		Epoch:     terms.Epoch,
		State:     Joined,
		Threshold: terms.Threshold,
		Timeout:   terms.Timeout,
		Leader:    terms.Leader,
		Remaining: terms.Remaining,
		Joining:   terms.Joining,
		Leaving:   terms.Leaving,
	}, NoError
}

func (d DKGDetails) Proposing(me *drand.Participant, terms *ProposalTerms) (*DKGDetails, DKGErrorCode) {
	if !isValidStateChange(d.State, Proposing) {
		return nil, InvalidStateChange
	}

	if terms.Leader != me {
		return nil, CannotProposeAsNonLeader
	}

	if err := ValidateProposal(d, terms); err != NoError {
		return nil, err
	}

	if d.State == Fresh && terms.Epoch > 1 {
		return nil, InvalidEpoch
	}

	return &DKGDetails{
		BeaconID:  d.BeaconID,
		Epoch:     terms.Epoch,
		State:     Proposing,
		Threshold: terms.Threshold,
		Timeout:   terms.Timeout,
		Leader:    terms.Leader,
		Remaining: terms.Remaining,
		Joining:   terms.Joining,
		Leaving:   terms.Leaving,
	}, NoError
}

func (d DKGDetails) Proposed(sender *drand.Participant, me *drand.Participant, terms *ProposalTerms) (*DKGDetails, DKGErrorCode) {
	if !isValidStateChange(d.State, Proposed) {
		return nil, InvalidStateChange
	}

	if terms.Leader != sender {
		return nil, CannotProposeAsNonLeader
	}

	if err := ValidateProposal(d, terms); err != NoError {
		return nil, err
	}

	if !contains(terms.Joining, me) && !contains(terms.Remaining, me) && !contains(terms.Leaving, me) {
		return nil, SelfMissingFromProposal
	}

	return &DKGDetails{
		BeaconID:  d.BeaconID,
		Epoch:     terms.Epoch,
		State:     Proposed,
		Threshold: terms.Threshold,
		Timeout:   terms.Timeout,
		Leader:    terms.Leader,
		Remaining: terms.Remaining,
		Joining:   terms.Joining,
		Leaving:   terms.Leaving,
	}, NoError
}

func (d DKGDetails) TimedOut() (*DKGDetails, DKGErrorCode) {
	if !isValidStateChange(d.State, TimedOut) {
		return nil, InvalidStateChange
	}

	d.State = TimedOut

	return &d, NoError
}

func (d DKGDetails) Aborted() (*DKGDetails, DKGErrorCode) {
	if !isValidStateChange(d.State, Aborted) {
		return nil, InvalidStateChange
	}

	d.State = Aborted

	return &d, NoError
}

func (d DKGDetails) Accepted(me *drand.Participant) (*DKGDetails, DKGErrorCode) {
	if !isValidStateChange(d.State, Accepted) {
		return nil, InvalidStateChange
	}

	if hasTimedOut(d) {
		return nil, TimeoutReached
	}

	if contains(d.Leaving, me) {
		return nil, CannotAcceptProposalWhereLeaving
	}

	if contains(d.Joining, me) {
		return nil, CannotAcceptProposalWhereJoining
	}

	d.State = Accepted
	return &d, NoError
}

func (d DKGDetails) Rejected(me *drand.Participant) (*DKGDetails, DKGErrorCode) {
	if !isValidStateChange(d.State, Rejected) {
		return nil, InvalidStateChange
	}

	if hasTimedOut(d) {
		return nil, TimeoutReached
	}

	if contains(d.Joining, me) {
		return nil, CannotRejectProposalWhereJoining
	}

	if contains(d.Leaving, me) {
		return nil, CannotRejectProposalWhereLeaving
	}

	d.State = Rejected
	return &d, NoError
}

func (d DKGDetails) Left(me *drand.Participant) (*DKGDetails, DKGErrorCode) {
	if !isValidStateChange(d.State, Left) {
		return nil, InvalidStateChange
	}

	if hasTimedOut(d) {
		return nil, TimeoutReached
	}

	if !contains(d.Leaving, me) && !contains(d.Joining, me) {
		return nil, CannotLeaveIfNotALeaver
	}

	d.State = Left

	return &d, NoError
}

func (d DKGDetails) Executing(me *drand.Participant) (*DKGDetails, DKGErrorCode) {
	if !isValidStateChange(d.State, Executing) {
		return nil, InvalidStateChange
	}

	if hasTimedOut(d) {
		return nil, TimeoutReached
	}

	if !contains(d.Remaining, me) && !contains(d.Joining, me) {
		return nil, CannotExecuteIfNotJoinerOrRemainer
	}

	d.State = Executing

	return &d, NoError
}

func (d DKGDetails) Complete(finalGroup []*drand.Participant) (*DKGDetails, DKGErrorCode) {
	if !isValidStateChange(d.State, Complete) {
		return nil, InvalidStateChange
	}

	if hasTimedOut(d) {
		return nil, TimeoutReached
	}

	d.FinalGroup = finalGroup
	d.State = Complete

	return &d, NoError
}

func (d DKGDetails) ReceivedAcceptance(me *drand.Participant, them *drand.Participant) (*DKGDetails, DKGErrorCode) {
	if d.State != Proposing {
		return nil, InvalidStateChange

	}

	if d.Leader != me {
		return nil, NonLeaderCannotReceiveAcceptance
	}

	if !contains(d.Remaining, them) {
		return nil, UnknownAcceptor
	}

	if contains(d.Acceptors, them) {
		return nil, DuplicateAcceptance
	}

	d.Acceptors = append(d.Acceptors, them)
	d.Rejectors = without(d.Rejectors, them)

	return &d, NoError
}

func (d DKGDetails) ReceivedRejection(me *drand.Participant, them *drand.Participant) (*DKGDetails, DKGErrorCode) {
	if d.State != Proposing {
		return nil, InvalidStateChange
	}

	if d.Leader != me {
		return nil, NonLeaderCannotReceiveRejection
	}

	if !contains(d.Remaining, them) {
		return nil, UnknownRejector
	}

	if contains(d.Rejectors, them) {
		return nil, DuplicateRejection
	}

	d.Rejectors = append(d.Rejectors, them)
	d.Acceptors = without(d.Acceptors, them)

	return &d, NoError
}

type DKGErrorCode uint32

const (
	NoError DKGErrorCode = iota
	InvalidStateChange
	TimeoutReached
	InvalidBeaconID
	SelfMissingFromProposal
	InvalidEpoch
	LeaderCantJoinAfterFirstEpoch
	LeaderNotRemaining
	LeaderNotJoining
	OnlyJoinersAllowedForFirstEpoch
	NoNodesRemaining
	CannotProposeAsNonLeader
	ThresholdHigherThanNodeCount
	RemainingNodesMustExistInCurrentEpoch
	CannotAcceptProposalWhereLeaving
	CannotAcceptProposalWhereJoining
	CannotRejectProposalWhereLeaving
	CannotRejectProposalWhereJoining
	CannotLeaveIfNotALeaver
	CannotExecuteIfNotJoinerOrRemainer
	UnknownAcceptor
	DuplicateAcceptance
	UnknownRejector
	DuplicateRejection
	NonLeaderCannotReceiveAcceptance
	NonLeaderCannotReceiveRejection
)

func (d DKGErrorCode) String() string {
	switch d {
	case NoError:
		return "NoError"
	case InvalidStateChange:
		return "InvalidStateChange"
	case TimeoutReached:
		return "TimeoutReached"
	case InvalidBeaconID:
		return "InvalidBeaconID"
	case SelfMissingFromProposal:
		return "SelfMissingFromProposal"
	case InvalidEpoch:
		return "InvalidEpoch"
	case LeaderCantJoinAfterFirstEpoch:
		return "LeaderCantJoinAfterFirstEpoch"
	case LeaderNotRemaining:
		return "LeaderNotRemaining"
	case LeaderNotJoining:
		return "LeaderNotJoining"
	case OnlyJoinersAllowedForFirstEpoch:
		return "OnlyJoinersAllowedForFirstEpoch"
	case NoNodesRemaining:
		return "NoNodesRemaining"
	case CannotProposeAsNonLeader:
		return "CannotProposeAsNonLeader"
	case ThresholdHigherThanNodeCount:
		return "ThresholdHigherThanNodeCount"
	case RemainingNodesMustExistInCurrentEpoch:
		return "RemainingNodesMustExistInCurrentEpoch"
	case CannotAcceptProposalWhereLeaving:
		return "CannotAcceptProposalWhereLeaving"
	case CannotAcceptProposalWhereJoining:
		return "CannotAcceptProposalWhereJoining"
	case CannotRejectProposalWhereLeaving:
		return "CannotRejectProposalWhereLeaving"
	case CannotRejectProposalWhereJoining:
		return "CannotRejectProposalWhereJoining"
	case CannotLeaveIfNotALeaver:
		return "CannotLeaveIfNotALeaver"
	case CannotExecuteIfNotJoinerOrRemainer:
		return "CannotExecuteIfNotJoinerOrRemainer"
	case UnknownAcceptor:
		return "UnknownAcceptor"
	case DuplicateAcceptance:
		return "DuplicateAcceptance"
	case DuplicateRejection:
		return "DuplicateRejection"
	case UnknownRejector:
		return "UnknownRejector"
	case NonLeaderCannotReceiveAcceptance:
		return "NonLeaderCannotReceiveAcceptance"
	case NonLeaderCannotReceiveRejection:
		return "NonLeaderCannotReceiveRejection"
	default:
		return "invalid DKG error code!"
	}
}

func isValidStateChange(current DKGStatus, next DKGStatus) bool {
	switch current {
	case Complete:
		return next == Proposing || next == Proposed
	case Aborted:
		return next == Proposing || next == Proposed
	case TimedOut:
		return next == Proposing || next == Proposed
	case Fresh:
		return next == Joined || next == Proposing || next == Proposed
	case Joined:
		return next == Left || next == Executing || next == Aborted || next == TimedOut
	case Left:
		return next == Joined || next == Aborted
	case Proposing:
		return next == Executing || next == Aborted || next == TimedOut
	case Proposed:
		return next == Accepted || next == Rejected || next == Aborted || next == TimedOut || next == Left
	case Accepted:
		return next == Executing || next == Aborted || next == TimedOut
	case Rejected:
		return next == Aborted || next == TimedOut
	case Executing:
		return next == Complete || next == TimedOut
	}
	return false
}

func hasTimedOut(details DKGDetails) bool {
	now := time.Now()
	return details.Timeout.Before(now) || details.Timeout == now
}

func ValidateProposal(currentState DKGDetails, terms *ProposalTerms) DKGErrorCode {
	if currentState.BeaconID != terms.BeaconID {
		return InvalidBeaconID
	}

	if terms.Timeout.Before(time.Now()) {
		return TimeoutReached
	}

	if int(terms.Threshold) > len(terms.Joining)+len(terms.Remaining) {
		return ThresholdHigherThanNodeCount
	}

	if terms.Epoch <= currentState.Epoch {
		return InvalidEpoch
	}

	if terms.Epoch > currentState.Epoch+1 && (currentState.State != Left && currentState.State != Fresh) {
		return InvalidEpoch
	}

	if terms.Epoch == 1 {
		if terms.Remaining != nil || terms.Leaving != nil {
			return OnlyJoinersAllowedForFirstEpoch
		}
		if !contains(terms.Joining, terms.Leader) {
			return LeaderNotJoining
		}

		return NoError
	}

	if contains(terms.Joining, terms.Leader) {
		return LeaderCantJoinAfterFirstEpoch
	}

	if len(terms.Remaining) == 0 {
		return NoNodesRemaining
	}

	if contains(terms.Leaving, terms.Leader) || !contains(terms.Remaining, terms.Leader) {
		return LeaderNotRemaining
	}

	if currentState.State != Fresh {
		for _, node := range terms.Remaining {
			if !contains(currentState.FinalGroup, node) {
				return RemainingNodesMustExistInCurrentEpoch
			}
		}
	}

	return NoError
}

func contains[T comparable](haystack []T, needle T) bool {
	if haystack == nil {
		return false
	}
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

func without[T comparable](haystack []T, needle T) []T {
	if haystack == nil {
		return nil
	}

	indexToRemove := -1
	for i, v := range haystack {
		if v == needle {
			indexToRemove = i
		}
	}

	if indexToRemove == -1 {
		return haystack
	}

	if len(haystack) == 1 {
		return nil
	}

	ret := make([]T, 0)
	ret = append(ret, haystack[:indexToRemove]...)
	return append(ret, haystack[indexToRemove+1:]...)
}
