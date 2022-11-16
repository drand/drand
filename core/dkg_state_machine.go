package core

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/drand/drand/protobuf/drand"
	"google.golang.org/protobuf/types/known/timestamppb"
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

// IntoEntry turns a DKGDetails object into the protobuf entry for
// easy marshalling and unmarshalling, and to maintain a
// consistent wire format
func (d *DKGDetails) IntoEntry() *drand.DKGEntry {
	if d == nil {
		return nil
	}
	return &drand.DKGEntry{
		State:      uint32(d.State),
		Epoch:      d.Epoch,
		Threshold:  d.Threshold,
		Timeout:    timestamppb.New(d.Timeout),
		Leader:     d.Leader,
		Remaining:  d.Remaining,
		Joining:    d.Joining,
		Leaving:    d.Leaving,
		Acceptors:  d.Acceptors,
		Rejectors:  d.Rejectors,
		FinalGroup: d.FinalGroup,
	}
}

func NewFreshState(beaconID string) *DKGDetails {
	return &DKGDetails{
		BeaconID: beaconID,
		State:    Fresh,
		Timeout:  time.Unix(0, 0).UTC(),
	}
}

func (d *DKGDetails) Joined(me *drand.Participant, terms *drand.ProposalTerms) (*DKGDetails, error) {
	if !isValidStateChange(d.State, Joined) {
		return nil, InvalidStateChange(d.State, Joined)
	}

	if err := ValidateProposal(d, terms); err != nil {
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
		Timeout:   terms.Timeout.AsTime(),
		Leader:    terms.Leader,
		Remaining: terms.Remaining,
		Joining:   terms.Joining,
		Leaving:   terms.Leaving,
	}, nil
}

func (d *DKGDetails) Proposing(me *drand.Participant, terms *drand.ProposalTerms) (*DKGDetails, error) {
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

	return &DKGDetails{
		BeaconID:  d.BeaconID,
		Epoch:     terms.Epoch,
		State:     Proposing,
		Threshold: terms.Threshold,
		Timeout:   terms.Timeout.AsTime(),
		Leader:    terms.Leader,
		Remaining: terms.Remaining,
		Joining:   terms.Joining,
		Leaving:   terms.Leaving,
	}, nil
}

func (d *DKGDetails) Proposed(sender *drand.Participant, me *drand.Participant, terms *drand.ProposalTerms) (*DKGDetails, error) {
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

	return &DKGDetails{
		BeaconID:  d.BeaconID,
		Epoch:     terms.Epoch,
		State:     Proposed,
		Threshold: terms.Threshold,
		Timeout:   terms.Timeout.AsTime(),
		Leader:    terms.Leader,
		Remaining: terms.Remaining,
		Joining:   terms.Joining,
		Leaving:   terms.Leaving,
	}, nil
}

func (d *DKGDetails) TimedOut() (*DKGDetails, error) {
	if !isValidStateChange(d.State, TimedOut) {
		return nil, InvalidStateChange(d.State, TimedOut)
	}

	d.State = TimedOut

	return d, nil
}

func (d *DKGDetails) Aborted() (*DKGDetails, error) {
	if !isValidStateChange(d.State, Aborted) {
		return nil, InvalidStateChange(d.State, Aborted)
	}

	d.State = Aborted

	return d, nil
}

func (d *DKGDetails) Accepted(me *drand.Participant) (*DKGDetails, error) {
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

	d.State = Accepted
	return d, nil
}

func (d *DKGDetails) Rejected(me *drand.Participant) (*DKGDetails, error) {
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

	d.State = Rejected
	return d, nil
}

func (d *DKGDetails) Left(me *drand.Participant) (*DKGDetails, error) {
	if !isValidStateChange(d.State, Left) {
		return nil, InvalidStateChange(d.State, Left)
	}

	if hasTimedOut(d) {
		return nil, TimeoutReached
	}

	if !contains(d.Leaving, me) && !contains(d.Joining, me) {
		return nil, CannotLeaveIfNotALeaver
	}

	d.State = Left

	return d, nil
}

func (d *DKGDetails) Executing(me *drand.Participant) (*DKGDetails, error) {
	if !isValidStateChange(d.State, Executing) {
		return nil, InvalidStateChange(d.State, Executing)
	}

	if hasTimedOut(d) {
		return nil, TimeoutReached
	}

	if !contains(d.Remaining, me) && !contains(d.Joining, me) {
		return nil, CannotExecuteIfNotJoinerOrRemainer
	}

	d.State = Executing

	return d, nil
}

func (d *DKGDetails) Complete(finalGroup []*drand.Participant) (*DKGDetails, error) {
	if !isValidStateChange(d.State, Complete) {
		return nil, InvalidStateChange(d.State, Complete)
	}

	if hasTimedOut(d) {
		return nil, TimeoutReached
	}

	d.FinalGroup = finalGroup
	d.State = Complete

	return d, nil
}

func (d *DKGDetails) ReceivedAcceptance(me *drand.Participant, them *drand.Participant) (*DKGDetails, error) {
	if d.State != Proposing {
		return nil, InvalidStateChange(d.State, Proposing)

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

	return d, nil
}

func (d *DKGDetails) ReceivedRejection(me *drand.Participant, them *drand.Participant) (*DKGDetails, error) {
	if d.State != Proposing {
		return nil, InvalidStateChange(d.State, Proposing)
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

	return d, nil
}

func InvalidStateChange(from DKGStatus, to DKGStatus) error {
	return fmt.Errorf("invalid transition attempt from %s to %s", from.String(), to.String())
}

var TimeoutReached = errors.New("timeout has been reached")
var InvalidBeaconID = errors.New("BeaconID was invalid")
var SelfMissingFromProposal = errors.New("you must include yourself in a proposal")
var InvalidEpoch = errors.New("the epoch provided was invalid")
var LeaderCantJoinAfterFirstEpoch = errors.New("you cannot lead a DKG and join at the same time (unless it is epoch 1)")
var LeaderNotRemaining = errors.New("you cannot lead a DKG and leave at the same time")
var LeaderNotJoining = errors.New("the leader must join in the first epoch")
var OnlyJoinersAllowedForFirstEpoch = errors.New("participants can only be joiners for the first epoch")
var NoNodesRemaining = errors.New("cannot propose a network without nodes remaining")
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
var InvalidPacket = errors.New("the packet received was invalid (i.e. not well formed)")
var UnexpectedError = errors.New("there was an unexpected error")

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

func hasTimedOut(details *DKGDetails) bool {
	now := time.Now()
	return details.Timeout.Before(now) || details.Timeout == now
}

func ValidateProposal(currentState *DKGDetails, terms *drand.ProposalTerms) error {
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
		for _, node := range terms.Remaining {
			if !contains(currentState.FinalGroup, node) {
				return RemainingNodesMustExistInCurrentEpoch
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
		if participantsEqual(v, needle) {
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

// participantsEqual performs a deep comparison of two drand.Participant objects
func participantsEqual(p1 *drand.Participant, p2 *drand.Participant) bool {
	return p1.Address == p2.Address &&
		p1.Tls == p2.Tls &&
		bytes.Compare(p1.PubKey, p2.PubKey) == 0 &&
		bytes.Compare(p1.Signature, p2.Signature) == 0
}
