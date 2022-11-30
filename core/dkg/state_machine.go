package dkg

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/util"
	"github.com/drand/kyber/share/dkg"
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
	Evicted
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
	case Evicted:
		return "Evicted"
	default:
		panic("impossible DKG state received")
	}
}

type DKGState struct {
	BeaconID       string
	Epoch          uint32
	State          DKGStatus
	Threshold      uint32
	Timeout        time.Time
	SchemeID       string
	GenesisTime    time.Time
	GenesisSeed    []byte
	TransitionTime time.Time
	CatchupPeriod  time.Duration
	BeaconPeriod   time.Duration

	Leader    *drand.Participant
	Remaining []*drand.Participant
	Joining   []*drand.Participant
	Leaving   []*drand.Participant

	Acceptors []*drand.Participant
	Rejectors []*drand.Participant

	FinalGroup []*drand.Participant
	KeyShare   *dkg.DistKeyShare
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

	if !util.Contains(d.Joining, me) {
		return nil, CannotJoinIfNotInJoining
	}

	d.State = Joined
	return d, nil
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
		BeaconID:       d.BeaconID,
		Epoch:          terms.Epoch,
		State:          Proposing,
		Threshold:      terms.Threshold,
		Timeout:        terms.Timeout.AsTime(),
		SchemeID:       terms.SchemeID,
		CatchupPeriod:  time.Duration(terms.CatchupPeriodSeconds) * time.Second,
		BeaconPeriod:   time.Duration(terms.BeaconPeriodSeconds) * time.Second,
		GenesisSeed:    terms.GenesisSeed,
		GenesisTime:    terms.GenesisTime.AsTime(),
		TransitionTime: terms.TransitionTime.AsTime(),
		Leader:         terms.Leader,
		Remaining:      terms.Remaining,
		Joining:        terms.Joining,
		Leaving:        terms.Leaving,
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

	if !util.Contains(terms.Joining, me) && !util.Contains(terms.Remaining, me) && !util.Contains(terms.Leaving, me) {
		return nil, SelfMissingFromProposal
	}

	return &DKGState{
		BeaconID:       d.BeaconID,
		Epoch:          terms.Epoch,
		State:          Proposed,
		Threshold:      terms.Threshold,
		Timeout:        terms.Timeout.AsTime(),
		SchemeID:       terms.SchemeID,
		CatchupPeriod:  time.Duration(terms.CatchupPeriodSeconds) * time.Second,
		BeaconPeriod:   time.Duration(terms.BeaconPeriodSeconds) * time.Second,
		GenesisSeed:    terms.GenesisSeed,
		GenesisTime:    terms.GenesisTime.AsTime(),
		TransitionTime: terms.TransitionTime.AsTime(),
		Leader:         terms.Leader,
		Remaining:      terms.Remaining,
		Joining:        terms.Joining,
		Leaving:        terms.Leaving,
	}, nil
}

func (d *DKGState) TimedOut() (*DKGState, error) {
	if !isValidStateChange(d.State, TimedOut) {
		return nil, InvalidStateChange(d.State, TimedOut)
	}

	d.State = TimedOut
	return d, nil
}

func (d *DKGState) Aborted() (*DKGState, error) {
	if !isValidStateChange(d.State, Aborted) {
		return nil, InvalidStateChange(d.State, Aborted)
	}

	d.State = Aborted
	return d, nil
}

func (d *DKGState) Accepted(me *drand.Participant) (*DKGState, error) {
	if !isValidStateChange(d.State, Accepted) {
		return nil, InvalidStateChange(d.State, Accepted)
	}

	if hasTimedOut(d) {
		return nil, TimeoutReached
	}

	if util.Contains(d.Leaving, me) {
		return nil, CannotAcceptProposalWhereLeaving
	}

	if util.Contains(d.Joining, me) {
		return nil, CannotAcceptProposalWhereJoining
	}

	d.State = Accepted
	return d, nil
}

func (d *DKGState) Rejected(me *drand.Participant) (*DKGState, error) {
	if !isValidStateChange(d.State, Rejected) {
		return nil, InvalidStateChange(d.State, Rejected)
	}

	if hasTimedOut(d) {
		return nil, TimeoutReached
	}

	if util.Contains(d.Joining, me) {
		return nil, CannotRejectProposalWhereJoining
	}

	if util.Contains(d.Leaving, me) {
		return nil, CannotRejectProposalWhereLeaving
	}

	d.State = Rejected
	return d, nil
}

func (d *DKGState) Left(me *drand.Participant) (*DKGState, error) {
	if !isValidStateChange(d.State, Left) {
		return nil, InvalidStateChange(d.State, Left)
	}

	if hasTimedOut(d) {
		return nil, TimeoutReached
	}

	if !util.Contains(d.Leaving, me) && !util.Contains(d.Joining, me) {
		return nil, CannotLeaveIfNotALeaver
	}

	d.State = Left
	return d, nil
}

func (d *DKGState) Evicted() (*DKGState, error) {
	if !isValidStateChange(d.State, Evicted) {
		return nil, InvalidStateChange(d.State, Evicted)
	}

	if hasTimedOut(d) {
		return nil, TimeoutReached
	}

	d.State = Evicted
	return d, nil
}

func (d *DKGState) Executing(me *drand.Participant) (*DKGState, error) {
	if hasTimedOut(d) {
		return nil, TimeoutReached
	}

	if util.Contains(d.Leaving, me) {
		return d.Left(me)
	}

	if !isValidStateChange(d.State, Executing) {
		return nil, InvalidStateChange(d.State, Executing)
	}

	if !util.Contains(d.Remaining, me) && !util.Contains(d.Joining, me) {
		return nil, CannotExecuteIfNotJoinerOrRemainer
	}

	d.State = Executing
	return d, nil
}

func (d *DKGState) Complete(finalGroup []*drand.Participant, share *dkg.DistKeyShare) (*DKGState, error) {
	if !isValidStateChange(d.State, Complete) {
		return nil, InvalidStateChange(d.State, Complete)
	}

	if hasTimedOut(d) {
		return nil, TimeoutReached
	}

	d.State = Complete
	d.FinalGroup = finalGroup
	d.KeyShare = share

	return d, nil
}

func (d *DKGState) ReceivedAcceptance(me *drand.Participant, them *drand.Participant) (*DKGState, error) {
	if d.State != Proposing {
		return nil, InvalidStateChange(d.State, Proposing)

	}

	if !util.EqualParticipant(d.Leader, me) {
		return nil, NonLeaderCannotReceiveAcceptance
	}

	if !util.Contains(d.Remaining, them) {
		return nil, UnknownAcceptor
	}

	if util.Contains(d.Acceptors, them) {
		return nil, DuplicateAcceptance
	}

	d.Acceptors = append(d.Acceptors, them)
	d.Rejectors = util.Without(d.Rejectors, them)

	return d, nil
}

func (d *DKGState) ReceivedRejection(me *drand.Participant, them *drand.Participant) (*DKGState, error) {
	if d.State != Proposing {
		return nil, InvalidStateChange(d.State, Proposing)
	}

	if !util.EqualParticipant(d.Leader, me) {
		return nil, NonLeaderCannotReceiveRejection
	}

	if !util.Contains(d.Remaining, them) {
		return nil, UnknownRejector
	}

	if util.Contains(d.Rejectors, them) {
		return nil, DuplicateRejection
	}

	d.Acceptors = util.Without(d.Acceptors, them)
	d.Rejectors = append(d.Rejectors, them)

	return d, nil
}

func InvalidStateChange(from DKGStatus, to DKGStatus) error {
	return fmt.Errorf("invalid transition attempt from %s to %s", from.String(), to.String())
}

var TimeoutReached = errors.New("timeout has been reached")
var InvalidBeaconID = errors.New("BeaconID was invalid")
var InvalidScheme = errors.New("the scheme proposed does not exist")
var GenesisTimeNotEqual = errors.New("genesis time cannot be changed after the initial DKG")
var GenesisSeedMissing = errors.New("you must provide a genesis seed when starting a network")
var GenesisSeedNotEqual = errors.New("genesis seed cannot be changed after the initial DKG")
var TransitionTimeMustBeGenesisTime = errors.New("transition time must be the same as the genesis time for the first epoch")
var TransitionTimeMissing = errors.New("transition time must be provided in a proposal")
var TransitionTimeBeforeGenesis = errors.New("transition time cannot be before the genesis time")
var SelfMissingFromProposal = errors.New("you must include yourself in a proposal")
var CannotJoinIfNotInJoining = errors.New("you cannot join a proposal in which you are not a joiner")
var InvalidEpoch = errors.New("the epoch provided was invalid")
var LeaderCantJoinAfterFirstEpoch = errors.New("you cannot lead a DKG and join at the same time (unless it is epoch 1)")
var LeaderNotRemaining = errors.New("you cannot lead a DKG and leave at the same time")
var LeaderNotJoining = errors.New("the leader must join in the first epoch")
var OnlyJoinersAllowedForFirstEpoch = errors.New("participants can only be joiners for the first epoch")
var NoNodesRemaining = errors.New("cannot propose a network common.Without nodes remaining")
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
		return next == Proposing || next == Proposed || next == Evicted
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
		return next == Complete || next == TimedOut || next == Evicted
	case Evicted:
		return next == Joined
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

	_, found := scheme.GetSchemeByID(terms.SchemeID)
	if !found {
		return InvalidScheme
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

	if len(terms.GenesisSeed) == 0 {
		return GenesisSeedMissing
	}

	if terms.TransitionTime == nil {
		return TransitionTimeMissing
	}

	if terms.Epoch == 1 {
		if terms.Remaining != nil || terms.Leaving != nil {
			return OnlyJoinersAllowedForFirstEpoch
		}
		if !util.Contains(terms.Joining, terms.Leader) {
			return LeaderNotJoining
		}
		if !terms.TransitionTime.AsTime().Equal(terms.GenesisTime.AsTime()) {
			return TransitionTimeMustBeGenesisTime
		}

		return nil
	}

	if terms.GenesisTime.AsTime() != currentState.GenesisTime {
		return GenesisTimeNotEqual
	}

	if bytes.Equal(terms.GenesisSeed, currentState.GenesisSeed) {
		return GenesisSeedNotEqual
	}

	// perhaps this should be stricter?
	// should there be at least one round? should it be after `time.Now()`?
	if !currentState.GenesisTime.Before(terms.TransitionTime.AsTime()) {
		return TransitionTimeBeforeGenesis
	}

	if util.Contains(terms.Joining, terms.Leader) {
		return LeaderCantJoinAfterFirstEpoch
	}

	if len(terms.Remaining) == 0 {
		return NoNodesRemaining
	}

	if util.Contains(terms.Leaving, terms.Leader) || !util.Contains(terms.Remaining, terms.Leader) {
		return LeaderNotRemaining
	}

	if currentState.State != Fresh {
		// make sure all proposed `remaining` nodes exist in the current epoch
		for _, node := range terms.Remaining {
			if !util.Contains(currentState.FinalGroup, node) {
				return RemainingNodesMustExistInCurrentEpoch
			}
		}

		// make sure all the nodes from the current epoch exist in the proposal
		shouldBeCurrentParticipants := append(terms.Remaining, terms.Leaving...)
		for _, node := range currentState.FinalGroup {
			if !util.Contains(shouldBeCurrentParticipants, node) {
				return MissingNodesInProposal
			}
		}
	}

	return nil
}
