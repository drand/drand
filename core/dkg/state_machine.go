package dkg

import (
	"errors"
	"fmt"
	"github.com/drand/drand/protobuf/drand"
	"reflect"
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

func (s DKGStatus) String() string {
	switch s {
	case Fresh:
		return "Fresh"
	case Proposed:
		return "Proposed"
	case Proposing:
		return "Proposing"
	case Accepted:
		return "Accepted"
	case Rejected:
		return "Rejected"
	case Aborted:
		return "Aborted"
	case Executing:
		return "Executing"
	case Complete:
		return "Complete"
	case TimedOut:
		return "TimedOut"
	case Joined:
		return "Joined"
	case Left:
		return "Left"
	default:
		panic("impossible DKG state received")
	}
}

type DKGState struct {
	BeaconID      string
	Epoch         uint32
	State         DKGStatus
	Threshold     uint32
	Timeout       time.Time
	SchemeID      string
	CatchupPeriod time.Duration
	BeaconPeriod  time.Duration

	Leader    *drand.Participant
	Remaining []*drand.Participant
	Joining   []*drand.Participant
	Leaving   []*drand.Participant

	Acceptors []*drand.Participant
	Rejectors []*drand.Participant

	FinalGroup []*drand.Participant
}

func NewFreshState(beaconID string) *DKGState {
	return &DKGState{
		BeaconID: beaconID,
		State:    Fresh,
		Timeout:  time.Unix(0, 0).UTC(),
	}
}

func (d *DKGState) Joined(me *drand.Participant) (*DKGState, error) {
	if !isValidStateChange(d.State, Joined) {
		return nil, InvalidStateChange(d.State, Joined)
	}

	if hasTimedOut(d) {
		return nil, TimeoutReached
	}

	if !contains(d.Joining, me) {
		return nil, CannotJoinIfNotInJoining
	}

	return &DKGState{
		BeaconID:  d.BeaconID,
		Epoch:     d.Epoch,
		State:     Joined,
		Threshold: d.Threshold,
		Timeout:   d.Timeout,
		Leader:    d.Leader,
		Remaining: d.Remaining,
		Joining:   d.Joining,
		Leaving:   d.Leaving,
	}, nil
}

func (d *DKGState) Proposing(me *drand.Participant, terms *drand.ProposalTerms) (*DKGState, error) {
	if !isValidStateChange(d.State, Proposing) {
		return nil, InvalidStateChange(d.State, Proposing)
	}

	if terms.Leader != me {
		return nil, CannotProposeAsNonLeader
	}

	if err := ValidateProposal(d, terms); err != nil {
		return nil, err
	}

	if d.State == Fresh && terms.Epoch > 1 {
		return nil, InvalidEpoch
	}

	return &DKGState{
		BeaconID:      d.BeaconID,
		Epoch:         terms.Epoch,
		State:         Proposing,
		Threshold:     terms.Threshold,
		Timeout:       terms.Timeout.AsTime(),
		SchemeID:      terms.SchemeID,
		CatchupPeriod: time.Duration(terms.CatchupPeriodSeconds) * time.Second,
		BeaconPeriod:  time.Duration(terms.BeaconPeriodSeconds) * time.Second,
		Leader:        terms.Leader,
		Remaining:     terms.Remaining,
		Joining:       terms.Joining,
		Leaving:       terms.Leaving,
	}, nil
}

func (d *DKGState) Proposed(sender *drand.Participant, me *drand.Participant, terms *drand.ProposalTerms) (*DKGState, error) {
	if !isValidStateChange(d.State, Proposed) {
		return nil, InvalidStateChange(d.State, Proposed)
	}

	if terms.Leader != sender {
		return nil, CannotProposeAsNonLeader
	}

	if err := ValidateProposal(d, terms); err != nil {
		return nil, err
	}

	if !contains(terms.Joining, me) && !contains(terms.Remaining, me) && !contains(terms.Leaving, me) {
		return nil, SelfMissingFromProposal
	}

	return &DKGState{
		BeaconID:      d.BeaconID,
		Epoch:         terms.Epoch,
		State:         Proposed,
		Threshold:     terms.Threshold,
		Timeout:       terms.Timeout.AsTime(),
		SchemeID:      terms.SchemeID,
		CatchupPeriod: time.Duration(terms.CatchupPeriodSeconds) * time.Second,
		BeaconPeriod:  time.Duration(terms.BeaconPeriodSeconds) * time.Second,
		Leader:        terms.Leader,
		Remaining:     terms.Remaining,
		Joining:       terms.Joining,
		Leaving:       terms.Leaving,
	}, nil
}

func (d *DKGState) TimedOut() (*DKGState, error) {
	if !isValidStateChange(d.State, TimedOut) {
		return nil, InvalidStateChange(d.State, TimedOut)
	}

	return &DKGState{
		BeaconID:   d.BeaconID,
		Epoch:      d.Epoch,
		State:      TimedOut,
		Threshold:  d.Threshold,
		Timeout:    d.Timeout,
		Leader:     d.Leader,
		Remaining:  d.Remaining,
		Joining:    d.Joining,
		Leaving:    d.Leaving,
		FinalGroup: d.FinalGroup,
	}, nil
}

func (d *DKGState) Aborted() (*DKGState, error) {
	if !isValidStateChange(d.State, Aborted) {
		return nil, InvalidStateChange(d.State, Aborted)
	}

	return &DKGState{
		BeaconID:      d.BeaconID,
		Epoch:         d.Epoch,
		State:         Aborted,
		Threshold:     d.Threshold,
		Timeout:       d.Timeout,
		SchemeID:      d.SchemeID,
		CatchupPeriod: d.CatchupPeriod,
		BeaconPeriod:  d.BeaconPeriod,
		Leader:        d.Leader,
		Remaining:     d.Remaining,
		Joining:       d.Joining,
		Leaving:       d.Leaving,
		FinalGroup:    d.FinalGroup,
	}, nil
}

func (d *DKGState) Accepted(me *drand.Participant) (*DKGState, error) {
	if !isValidStateChange(d.State, Accepted) {
		return nil, InvalidStateChange(d.State, Accepted)
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

	return &DKGState{
		BeaconID:      d.BeaconID,
		Epoch:         d.Epoch,
		State:         Accepted,
		Threshold:     d.Threshold,
		Timeout:       d.Timeout,
		SchemeID:      d.SchemeID,
		CatchupPeriod: d.CatchupPeriod,
		BeaconPeriod:  d.BeaconPeriod,
		Leader:        d.Leader,
		Remaining:     d.Remaining,
		Joining:       d.Joining,
		Leaving:       d.Leaving,
		FinalGroup:    d.FinalGroup,
	}, nil
}

func (d *DKGState) Rejected(me *drand.Participant) (*DKGState, error) {
	if !isValidStateChange(d.State, Rejected) {
		return nil, InvalidStateChange(d.State, Rejected)
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

	return &DKGState{
		BeaconID:      d.BeaconID,
		Epoch:         d.Epoch,
		State:         Rejected,
		Threshold:     d.Threshold,
		Timeout:       d.Timeout,
		SchemeID:      d.SchemeID,
		CatchupPeriod: d.CatchupPeriod,
		BeaconPeriod:  d.BeaconPeriod,
		Leader:        d.Leader,
		Remaining:     d.Remaining,
		Joining:       d.Joining,
		Leaving:       d.Leaving,
		FinalGroup:    d.FinalGroup,
	}, nil
}

func (d *DKGState) Left(me *drand.Participant) (*DKGState, error) {
	if !isValidStateChange(d.State, Left) {
		return nil, InvalidStateChange(d.State, Left)
	}

	if hasTimedOut(d) {
		return nil, TimeoutReached
	}

	if !contains(d.Leaving, me) && !contains(d.Joining, me) {
		return nil, CannotLeaveIfNotALeaver
	}

	return &DKGState{
		BeaconID:      d.BeaconID,
		Epoch:         d.Epoch,
		State:         Left,
		Threshold:     d.Threshold,
		Timeout:       d.Timeout,
		SchemeID:      d.SchemeID,
		CatchupPeriod: d.CatchupPeriod,
		BeaconPeriod:  d.BeaconPeriod,
		Leader:        d.Leader,
		Remaining:     d.Remaining,
		Joining:       d.Joining,
		Leaving:       d.Leaving,
		FinalGroup:    d.FinalGroup,
	}, nil
}

func (d *DKGState) Executing(me *drand.Participant) (*DKGState, error) {
	if hasTimedOut(d) {
		return nil, TimeoutReached
	}

	if contains(d.Leaving, me) {
		return d.Left(me)
	}

	if !isValidStateChange(d.State, Executing) {
		return nil, InvalidStateChange(d.State, Executing)
	}

	if !contains(d.Remaining, me) && !contains(d.Joining, me) {
		return nil, CannotExecuteIfNotJoinerOrRemainer
	}
	return &DKGState{
		BeaconID:      d.BeaconID,
		Epoch:         d.Epoch,
		State:         Executing,
		Threshold:     d.Threshold,
		Timeout:       d.Timeout,
		SchemeID:      d.SchemeID,
		CatchupPeriod: d.CatchupPeriod,
		BeaconPeriod:  d.BeaconPeriod,
		Leader:        d.Leader,
		Remaining:     d.Remaining,
		Joining:       d.Joining,
		Leaving:       d.Leaving,
		Acceptors:     d.Acceptors,
		Rejectors:     d.Rejectors,
		FinalGroup:    d.FinalGroup,
	}, nil
}

func (d *DKGState) Complete(finalGroup []*drand.Participant) (*DKGState, error) {
	if !isValidStateChange(d.State, Complete) {
		return nil, InvalidStateChange(d.State, Complete)
	}

	if hasTimedOut(d) {
		return nil, TimeoutReached
	}

	return &DKGState{
		BeaconID:      d.BeaconID,
		Epoch:         d.Epoch,
		State:         Complete,
		Threshold:     d.Threshold,
		Timeout:       d.Timeout,
		SchemeID:      d.SchemeID,
		CatchupPeriod: d.CatchupPeriod,
		BeaconPeriod:  d.BeaconPeriod,
		Leader:        d.Leader,
		Remaining:     d.Remaining,
		Joining:       d.Joining,
		Leaving:       d.Leaving,
		Acceptors:     d.Acceptors,
		Rejectors:     d.Rejectors,
		FinalGroup:    finalGroup,
	}, nil
}

func (d *DKGState) ReceivedAcceptance(me *drand.Participant, them *drand.Participant) (*DKGState, error) {
	if d.State != Proposing {
		return nil, InvalidStateChange(d.State, Proposing)

	}

	if !equalParticipant(d.Leader, me) {
		return nil, NonLeaderCannotReceiveAcceptance
	}

	if !contains(d.Remaining, them) {
		return nil, UnknownAcceptor
	}

	if contains(d.Acceptors, them) {
		return nil, DuplicateAcceptance
	}

	return &DKGState{
		BeaconID:      d.BeaconID,
		Epoch:         d.Epoch,
		State:         Proposing,
		Threshold:     d.Threshold,
		Timeout:       d.Timeout,
		SchemeID:      d.SchemeID,
		CatchupPeriod: d.CatchupPeriod,
		BeaconPeriod:  d.BeaconPeriod,
		Leader:        d.Leader,
		Remaining:     d.Remaining,
		Joining:       d.Joining,
		Leaving:       d.Leaving,
		Acceptors:     append(d.Acceptors, them),
		Rejectors:     without(d.Rejectors, them),
		FinalGroup:    d.FinalGroup,
	}, nil
}

func (d *DKGState) ReceivedRejection(me *drand.Participant, them *drand.Participant) (*DKGState, error) {
	if d.State != Proposing {
		return nil, InvalidStateChange(d.State, Proposing)
	}

	if !equalParticipant(d.Leader, me) {
		return nil, NonLeaderCannotReceiveRejection
	}

	if !contains(d.Remaining, them) {
		return nil, UnknownRejector
	}

	if contains(d.Rejectors, them) {
		return nil, DuplicateRejection
	}

	return &DKGState{
		BeaconID:      d.BeaconID,
		Epoch:         d.Epoch,
		State:         Proposing,
		Threshold:     d.Threshold,
		Timeout:       d.Timeout,
		SchemeID:      d.SchemeID,
		CatchupPeriod: d.CatchupPeriod,
		BeaconPeriod:  d.BeaconPeriod,
		Leader:        d.Leader,
		Remaining:     d.Remaining,
		Joining:       d.Joining,
		Leaving:       d.Leaving,
		Acceptors:     without(d.Acceptors, them),
		Rejectors:     append(d.Rejectors, them),
		FinalGroup:    d.FinalGroup,
	}, nil
}

func InvalidStateChange(from DKGStatus, to DKGStatus) error {
	return fmt.Errorf("invalid transition attempt from %s to %s", from.String(), to.String())
}

var TimeoutReached = errors.New("timeout has been reached")
var InvalidBeaconID = errors.New("BeaconID was invalid")
var SelfMissingFromProposal = errors.New("you must include yourself in a proposal")
var CannotJoinIfNotInJoining = errors.New("you cannot join a proposal in which you are not a joiner")
var InvalidEpoch = errors.New("the epoch provided was invalid")
var LeaderCantJoinAfterFirstEpoch = errors.New("you cannot lead a DKG and join at the same time (unless it is epoch 1)")
var LeaderNotRemaining = errors.New("you cannot lead a DKG and leave at the same time")
var LeaderNotJoining = errors.New("the leader must join in the first epoch")
var OnlyJoinersAllowedForFirstEpoch = errors.New("participants can only be joiners for the first epoch")
var NoNodesRemaining = errors.New("cannot propose a network without nodes remaining")
var MissingNodesInProposal = errors.New("some node(s) in the current epoch are missing from the proposal - they should be remaining or leaving")
var CannotProposeAsNonLeader = errors.New("cannot make a proposal where you are not the leader")
var ThresholdHigherThanNodeCount = errors.New("the threshold cannot be higher than the count of remaining + joining nodes")
var RemainingNodesMustExistInCurrentEpoch = errors.New("remaining nodes contained a not that does not exist in the current epoch - they must be added as joiners")
var CannotAcceptProposalWhereLeaving = errors.New("you cannot accept a proposal where your node is leaving")
var CannotAcceptProposalWhereJoining = errors.New("you cannot accept a proposal where your node is joining - run the join command instead")
var CannotRejectProposalWhereLeaving = errors.New("you cannot reject a proposal where your node is leaving")
var CannotRejectProposalWhereJoining = errors.New("you cannot reject a proposal where your node is joining (just turn your node off)")
var CannotLeaveIfNotALeaver = errors.New("you cannot execute leave if you were not included as a leaver in the proposal")
var CannotExecuteIfNotJoinerOrRemainer = errors.New("you cannot start execution if you are not a remainer or joiner to the DKG")
var UnknownAcceptor = errors.New("somebody unknown tried to accept the proposal")
var DuplicateAcceptance = errors.New("this participant already accepted the proposal")
var UnknownRejector = errors.New("somebody unknown tried to reject the proposal")
var DuplicateRejection = errors.New("this participant already rejected the proposal")
var NonLeaderCannotReceiveAcceptance = errors.New("you received an acceptance but are not the leader of this DKG - cannot do anything")
var NonLeaderCannotReceiveRejection = errors.New("you received a rejection but are not the leader of this DKG - cannot do anything")

func isValidStateChange(current DKGStatus, next DKGStatus) bool {
	switch current {
	case Complete:
		return next == Proposing || next == Proposed
	case Aborted:
		return next == Proposing || next == Proposed
	case TimedOut:
		return next == Proposing || next == Proposed
	case Fresh:
		return next == Proposing || next == Proposed
	case Joined:
		return next == Left || next == Executing || next == Aborted || next == TimedOut
	case Left:
		return next == Joined || next == Aborted
	case Proposing:
		return next == Executing || next == Aborted || next == TimedOut
	case Proposed:
		return next == Accepted || next == Rejected || next == Aborted || next == TimedOut || next == Left || next == Joined
	case Accepted:
		return next == Executing || next == Aborted || next == TimedOut
	case Rejected:
		return next == Aborted || next == TimedOut
	case Executing:
		return next == Complete || next == TimedOut
	}
	return false
}

func hasTimedOut(details *DKGState) bool {
	now := time.Now()
	return details.Timeout.Before(now) || details.Timeout == now
}

func ValidateProposal(currentState *DKGState, terms *drand.ProposalTerms) error {
	if currentState.BeaconID != terms.BeaconID {
		return InvalidBeaconID
	}

	if terms.Timeout.AsTime().Before(time.Now()) {
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

		return nil
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
		// make sure all proposed `remaining` nodes exist in the current epoch
		for _, node := range terms.Remaining {
			if !contains(currentState.FinalGroup, node) {
				return RemainingNodesMustExistInCurrentEpoch
			}
		}

		// make sure all the nodes from the current epoch exist in the proposal
		shouldBeCurrentParticipants := append(terms.Remaining, terms.Leaving...)
		for _, node := range currentState.FinalGroup {
			if !contains(shouldBeCurrentParticipants, node) {
				return MissingNodesInProposal
			}
		}
	}

	return nil
}

func contains(haystack []*drand.Participant, needle *drand.Participant) bool {
	if haystack == nil {
		return false
	}
	for _, v := range haystack {
		if equalParticipant(v, needle) {
			return true
		}
	}
	return false
}

func without(haystack []*drand.Participant, needle *drand.Participant) []*drand.Participant {
	if haystack == nil {
		return nil
	}

	indexToRemove := -1
	for i, v := range haystack {
		if equalParticipant(v, needle) {
			indexToRemove = i
		}
	}

	if indexToRemove == -1 {
		return haystack
	}

	if len(haystack) == 1 {
		return nil
	}

	ret := make([]*drand.Participant, 0)
	ret = append(ret, haystack[:indexToRemove]...)
	return append(ret, haystack[indexToRemove+1:]...)
}

func equalParticipant(p1 *drand.Participant, p2 *drand.Participant) bool {
	return p1.Tls == p2.Tls && p1.Address == p2.Address && reflect.DeepEqual(p1.PubKey, p2.PubKey)
}
