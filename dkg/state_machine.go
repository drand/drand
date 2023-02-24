//nolint:lll,dupl
package dkg

// the error messages are very long but go fmt doesn't want them over multiple lines
// the DBState and DBStateTOML structs are quite similar so the linter reports duplicate code

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/drand/drand/crypto"
	"reflect"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/util"
)

//nolint:revive
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

// DBState !!! if you add a field, make sure you add it to DBStateTOML AND the FromTOML()/TOML() functions too !!!
type DBState struct {
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

	FinalGroup *key.Group
	KeyShare   *key.Share
}

// Equals does a deep equal comparison on all the values in the `DBState`
func (d *DBState) Equals(e *DBState) bool {
	if d == nil {
		return e == nil
	}

	if e == nil {
		return false
	}

	return d.BeaconID == e.BeaconID &&
		d.Epoch == e.Epoch &&
		d.State == e.State &&
		d.Threshold == e.Threshold &&
		d.Timeout == e.Timeout &&
		d.SchemeID == e.SchemeID &&
		d.GenesisTime == e.GenesisTime &&
		bytes.Equal(d.GenesisSeed, e.GenesisSeed) &&
		d.TransitionTime == e.TransitionTime &&
		d.CatchupPeriod == e.CatchupPeriod &&
		d.BeaconPeriod == e.BeaconPeriod &&
		reflect.DeepEqual(d.Leader, e.Leader) &&
		reflect.DeepEqual(d.Remaining, e.Remaining) &&
		reflect.DeepEqual(d.Joining, e.Joining) &&
		reflect.DeepEqual(d.Leaving, e.Leaving) &&
		reflect.DeepEqual(d.Acceptors, e.Acceptors) &&
		reflect.DeepEqual(d.Rejectors, e.Rejectors) &&
		d.FinalGroup.Equal(e.FinalGroup) &&
		reflect.DeepEqual(d.KeyShare, e.KeyShare)
}

type DBStateTOML struct {
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

	FinalGroup *key.GroupTOML
	KeyShare   *key.ShareTOML
}

func (d *DBState) TOML() DBStateTOML {
	var finalGroup *key.GroupTOML
	if d.FinalGroup != nil {
		finalGroup = d.FinalGroup.TOML().(*key.GroupTOML)
	}
	var keyShare *key.ShareTOML
	if d.KeyShare != nil {
		keyShare = d.KeyShare.TOML().(*key.ShareTOML)
	}

	return DBStateTOML{
		BeaconID:       d.BeaconID,
		Epoch:          d.Epoch,
		State:          d.State,
		Threshold:      d.Threshold,
		Timeout:        d.Timeout,
		SchemeID:       d.SchemeID,
		GenesisTime:    d.GenesisTime,
		GenesisSeed:    d.GenesisSeed,
		TransitionTime: d.TransitionTime,
		CatchupPeriod:  d.CatchupPeriod,
		BeaconPeriod:   d.BeaconPeriod,
		Leader:         d.Leader,
		Remaining:      d.Remaining,
		Joining:        d.Joining,
		Leaving:        d.Leaving,
		Acceptors:      d.Acceptors,
		Rejectors:      d.Rejectors,
		FinalGroup:     finalGroup,
		KeyShare:       keyShare,
	}
}

func (d *DBStateTOML) FromTOML() (*DBState, error) {
	var share *key.Share
	if d.KeyShare != nil {
		share = &key.Share{}
		err := share.FromTOML(d.KeyShare)
		if err != nil {
			return nil, err
		}
	}

	var finalGroup *key.Group
	if d.FinalGroup != nil {
		finalGroup = &key.Group{}
		err := finalGroup.FromTOML(d.FinalGroup)
		if err != nil {
			return nil, err
		}
	}

	return &DBState{
		BeaconID:       d.BeaconID,
		Epoch:          d.Epoch,
		State:          d.State,
		Threshold:      d.Threshold,
		Timeout:        d.Timeout,
		SchemeID:       d.SchemeID,
		GenesisTime:    d.GenesisTime,
		GenesisSeed:    d.GenesisSeed,
		TransitionTime: d.TransitionTime,
		CatchupPeriod:  d.CatchupPeriod,
		BeaconPeriod:   d.BeaconPeriod,
		Leader:         d.Leader,
		Remaining:      d.Remaining,
		Joining:        d.Joining,
		Leaving:        d.Leaving,
		Acceptors:      d.Acceptors,
		Rejectors:      d.Rejectors,
		FinalGroup:     finalGroup,
		KeyShare:       share,
	}, nil
}

func NewFreshState(beaconID string) *DBState {
	return &DBState{
		BeaconID: beaconID,
		State:    Fresh,
		Timeout:  time.Unix(0, 0).UTC(),
	}
}

func (d *DBState) Joined(me *drand.Participant, previousGroup *key.Group) (*DBState, error) {
	if !isValidStateChange(d.State, Joined) {
		return nil, InvalidStateChange(d.State, Joined)
	}

	if hasTimedOut(d) {
		return nil, ErrTimeoutReached
	}

	if d.Epoch != 1 && previousGroup == nil {
		return nil, ErrJoiningAfterFirstEpochNeedsGroupFile
	}

	if !util.Contains(d.Joining, me) {
		return nil, ErrCannotJoinIfNotInJoining
	}

	d.State = Joined
	d.FinalGroup = previousGroup
	return d, nil
}

func (d *DBState) Proposing(me *drand.Participant, terms *drand.ProposalTerms) (*DBState, error) {
	if !isValidStateChange(d.State, Proposing) {
		return nil, InvalidStateChange(d.State, Proposing)
	}

	if terms.Leader != me {
		return nil, ErrCannotProposeAsNonLeader
	}

	if err := ValidateProposal(d, terms); err != nil {
		return nil, err
	}

	if d.State == Fresh && terms.Epoch > 1 {
		return nil, ErrInvalidEpoch
	}

	return &DBState{
		BeaconID:       d.BeaconID,
		Epoch:          terms.Epoch,
		State:          Proposing,
		Threshold:      terms.Threshold,
		Timeout:        terms.Timeout.AsTime(),
		SchemeID:       terms.SchemeID,
		CatchupPeriod:  time.Duration(terms.CatchupPeriodSeconds) * time.Second,
		BeaconPeriod:   time.Duration(terms.BeaconPeriodSeconds) * time.Second,
		GenesisTime:    terms.GenesisTime.AsTime(),
		GenesisSeed:    d.GenesisSeed, // does not exist until the first DKG has completed
		TransitionTime: terms.TransitionTime.AsTime(),
		Leader:         terms.Leader,
		Remaining:      terms.Remaining,
		Joining:        terms.Joining,
		Leaving:        terms.Leaving,
	}, nil
}

func (d *DBState) Proposed(sender, me *drand.Participant, terms *drand.ProposalTerms) (*DBState, error) {
	if !isValidStateChange(d.State, Proposed) {
		return nil, InvalidStateChange(d.State, Proposed)
	}

	if terms.Leader != sender {
		return nil, ErrCannotProposeAsNonLeader
	}

	if err := ValidateProposal(d, terms); err != nil {
		return nil, err
	}

	if !util.Contains(terms.Joining, me) && !util.Contains(terms.Remaining, me) && !util.Contains(terms.Leaving, me) {
		return nil, ErrSelfMissingFromProposal
	}

	return &DBState{
		BeaconID:       d.BeaconID,
		Epoch:          terms.Epoch,
		State:          Proposed,
		Threshold:      terms.Threshold,
		Timeout:        terms.Timeout.AsTime(),
		SchemeID:       terms.SchemeID,
		CatchupPeriod:  time.Duration(terms.CatchupPeriodSeconds) * time.Second,
		BeaconPeriod:   time.Duration(terms.BeaconPeriodSeconds) * time.Second,
		GenesisTime:    terms.GenesisTime.AsTime(),
		GenesisSeed:    terms.GenesisSeed,
		TransitionTime: terms.TransitionTime.AsTime(),
		Leader:         terms.Leader,
		Remaining:      terms.Remaining,
		Joining:        terms.Joining,
		Leaving:        terms.Leaving,
	}, nil
}

func (d *DBState) TimedOut() (*DBState, error) {
	if !isValidStateChange(d.State, TimedOut) {
		return nil, InvalidStateChange(d.State, TimedOut)
	}

	d.State = TimedOut
	return d, nil
}

func (d *DBState) Aborted() (*DBState, error) {
	if !isValidStateChange(d.State, Aborted) {
		return nil, InvalidStateChange(d.State, Aborted)
	}

	d.State = Aborted
	return d, nil
}

func (d *DBState) Accepted(me *drand.Participant) (*DBState, error) {
	if !isValidStateChange(d.State, Accepted) {
		return nil, InvalidStateChange(d.State, Accepted)
	}

	if hasTimedOut(d) {
		return nil, ErrTimeoutReached
	}

	if util.Contains(d.Leaving, me) {
		return nil, ErrCannotAcceptProposalWhereLeaving
	}

	if util.Contains(d.Joining, me) {
		return nil, ErrCannotAcceptProposalWhereJoining
	}

	d.State = Accepted
	return d, nil
}

func (d *DBState) Rejected(me *drand.Participant) (*DBState, error) {
	if !isValidStateChange(d.State, Rejected) {
		return nil, InvalidStateChange(d.State, Rejected)
	}

	if hasTimedOut(d) {
		return nil, ErrTimeoutReached
	}

	if util.Contains(d.Joining, me) {
		return nil, ErrCannotRejectProposalWhereJoining
	}

	if util.Contains(d.Leaving, me) {
		return nil, ErrCannotRejectProposalWhereLeaving
	}

	d.State = Rejected
	return d, nil
}

func (d *DBState) Left(me *drand.Participant) (*DBState, error) {
	if !isValidStateChange(d.State, Left) {
		return nil, InvalidStateChange(d.State, Left)
	}

	if hasTimedOut(d) {
		return nil, ErrTimeoutReached
	}

	if !util.Contains(d.Leaving, me) && !util.Contains(d.Joining, me) {
		return nil, ErrCannotLeaveIfNotALeaver
	}

	d.State = Left
	return d, nil
}

func (d *DBState) Evicted() (*DBState, error) {
	if !isValidStateChange(d.State, Evicted) {
		return nil, InvalidStateChange(d.State, Evicted)
	}

	if hasTimedOut(d) {
		return nil, ErrTimeoutReached
	}

	d.State = Evicted
	return d, nil
}

func (d *DBState) Executing(me *drand.Participant) (*DBState, error) {
	if hasTimedOut(d) {
		return nil, ErrTimeoutReached
	}

	if util.Contains(d.Leaving, me) {
		return d.Left(me)
	}

	if !isValidStateChange(d.State, Executing) {
		return nil, InvalidStateChange(d.State, Executing)
	}

	if !util.Contains(d.Remaining, me) && !util.Contains(d.Joining, me) {
		return nil, ErrCannotExecuteIfNotJoinerOrRemainer
	}

	d.State = Executing
	return d, nil
}

func (d *DBState) Complete(finalGroup *key.Group, share *key.Share) (*DBState, error) {
	if !isValidStateChange(d.State, Complete) {
		return nil, InvalidStateChange(d.State, Complete)
	}

	if hasTimedOut(d) {
		return nil, ErrTimeoutReached
	}

	if finalGroup == nil {
		return nil, ErrFinalGroupCannotBeEmpty
	}
	if share == nil {
		return nil, ErrKeyShareCannotBeEmpty
	}

	d.State = Complete
	d.FinalGroup = finalGroup
	d.KeyShare = share
	d.GenesisSeed = finalGroup.GetGenesisSeed()
	return d, nil
}

func (d *DBState) ReceivedAcceptance(me, them *drand.Participant) (*DBState, error) {
	if d.State != Proposing {
		return nil, InvalidStateChange(d.State, Proposing)
	}

	if !util.EqualParticipant(d.Leader, me) {
		return nil, ErrNonLeaderCannotReceiveAcceptance
	}

	if !util.Contains(d.Remaining, them) {
		return nil, ErrUnknownAcceptor
	}

	if util.Contains(d.Acceptors, them) {
		return nil, ErrDuplicateAcceptance
	}

	d.Acceptors = append(d.Acceptors, them)
	d.Rejectors = util.Without(d.Rejectors, them)

	return d, nil
}

func (d *DBState) ReceivedRejection(me, them *drand.Participant) (*DBState, error) {
	if d.State != Proposing {
		return nil, InvalidStateChange(d.State, Proposing)
	}

	if !util.EqualParticipant(d.Leader, me) {
		return nil, ErrNonLeaderCannotReceiveRejection
	}

	if !util.Contains(d.Remaining, them) {
		return nil, ErrUnknownRejector
	}

	if util.Contains(d.Rejectors, them) {
		return nil, ErrDuplicateRejection
	}

	d.Acceptors = util.Without(d.Acceptors, them)
	d.Rejectors = append(d.Rejectors, them)

	return d, nil
}

func InvalidStateChange(from, to DKGStatus) error {
	return fmt.Errorf("invalid transition attempt from %s to %s", from.String(), to.String())
}

var ErrTimeoutReached = errors.New("timeout has been reached")
var ErrInvalidBeaconID = errors.New("BeaconID was invalid")
var ErrInvalidScheme = errors.New("the scheme proposed does not exist")
var ErrGenesisTimeNotEqual = errors.New("genesis time cannot be changed after the initial DKG")
var ErrNoGenesisSeedForFirstEpoch = errors.New("the genesis seed is created during the first epoch, so you can't provide it in the proposal")
var ErrGenesisSeedCannotChange = errors.New("genesis seed cannot change after the first epoch")
var ErrTransitionTimeMustBeGenesisTime = errors.New("transition time must be the same as the genesis time for the first epoch")
var ErrTransitionTimeMissing = errors.New("transition time must be provided in a proposal")
var ErrTransitionTimeBeforeGenesis = errors.New("transition time cannot be before the genesis time")
var ErrSelfMissingFromProposal = errors.New("you must include yourself in a proposal")
var ErrCannotJoinIfNotInJoining = errors.New("you cannot join a proposal in which you are not a joiner")
var ErrJoiningAfterFirstEpochNeedsGroupFile = errors.New("joining after the first epoch requires a previous group file")
var ErrInvalidEpoch = errors.New("the epoch provided was invalid")
var ErrLeaderCantJoinAfterFirstEpoch = errors.New("you cannot lead a DKG and join at the same time (unless it is epoch 1)")
var ErrLeaderNotRemaining = errors.New("you cannot lead a DKG and leave at the same time")
var ErrLeaderNotJoining = errors.New("the leader must join in the first epoch")
var ErrOnlyJoinersAllowedForFirstEpoch = errors.New("participants can only be joiners for the first epoch")
var ErrNoNodesRemaining = errors.New("cannot propose a network common.Without nodes remaining")
var ErrMissingNodesInProposal = errors.New("some node(s) in the current epoch are missing from the proposal - they should be remaining or leaving")
var ErrCannotProposeAsNonLeader = errors.New("cannot make a proposal where you are not the leader")
var ErrThresholdHigherThanNodeCount = errors.New("the threshold cannot be higher than the count of remaining + joining nodes")
var ErrRemainingAndLeavingNodesMustExistInCurrentEpoch = errors.New("remaining and leaving nodes contained a not that does not exist in the current epoch - they must be added as joiners")
var ErrCannotAcceptProposalWhereLeaving = errors.New("you cannot accept a proposal where your node is leaving")
var ErrCannotAcceptProposalWhereJoining = errors.New("you cannot accept a proposal where your node is joining - run the join command instead")
var ErrCannotRejectProposalWhereLeaving = errors.New("you cannot reject a proposal where your node is leaving")
var ErrCannotRejectProposalWhereJoining = errors.New("you cannot reject a proposal where your node is joining (just turn your node off)")
var ErrCannotLeaveIfNotALeaver = errors.New("you cannot execute leave if you were not included as a leaver in the proposal")
var ErrCannotExecuteIfNotJoinerOrRemainer = errors.New("you cannot start execution if you are not a remainer or joiner to the DKG")
var ErrUnknownAcceptor = errors.New("somebody unknown tried to accept the proposal")
var ErrDuplicateAcceptance = errors.New("this participant already accepted the proposal")
var ErrUnknownRejector = errors.New("somebody unknown tried to reject the proposal")
var ErrDuplicateRejection = errors.New("this participant already rejected the proposal")
var ErrNonLeaderCannotReceiveAcceptance = errors.New("you received an acceptance but are not the leader of this DKG - cannot do anything")
var ErrNonLeaderCannotReceiveRejection = errors.New("you received a rejection but are not the leader of this DKG - cannot do anything")
var ErrFinalGroupCannotBeEmpty = errors.New("you cannot complete a DKG with a nil final group")
var ErrKeyShareCannotBeEmpty = errors.New("you cannot complete a DKG with a nil key share")

//nolint:gocyclo
func isValidStateChange(current, next DKGStatus) bool {
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

func hasTimedOut(details *DBState) bool {
	now := time.Now()
	return details.Timeout.Before(now) || details.Timeout.Equal(now)
}

func ValidateProposal(currentState *DBState, terms *drand.ProposalTerms) error {
	if currentState.BeaconID != terms.BeaconID {
		return ErrInvalidBeaconID
	}

	_, found := crypto.GetSchemeByID(terms.SchemeID)
	if !found {
		return ErrInvalidScheme
	}

	if terms.Timeout.AsTime().Before(time.Now()) {
		return ErrTimeoutReached
	}

	if int(terms.Threshold) > len(terms.Joining)+len(terms.Remaining) {
		return ErrThresholdHigherThanNodeCount
	}

	if err := validateEpoch(currentState, terms); err != nil {
		return err
	}

	if terms.TransitionTime == nil {
		return ErrTransitionTimeMissing
	}

	if terms.Epoch == 1 {
		return validateFirstEpoch(terms)
	}

	// perhaps this should be stricter?
	// should there be at least one round? should it be after `time.Now()`?
	if terms.TransitionTime.AsTime().Before(currentState.GenesisTime) {
		return ErrTransitionTimeBeforeGenesis
	}

	if util.Contains(terms.Joining, terms.Leader) {
		return ErrLeaderCantJoinAfterFirstEpoch
	}

	if len(terms.Remaining) == 0 {
		return ErrNoNodesRemaining
	}

	if util.Contains(terms.Leaving, terms.Leader) || !util.Contains(terms.Remaining, terms.Leader) {
		return ErrLeaderNotRemaining
	}

	if currentState.State != Fresh {
		return validateReshare(currentState, terms)
	}

	return nil
}

func validateEpoch(currentState *DBState, terms *drand.ProposalTerms) error {
	if terms.Epoch < currentState.Epoch {
		return ErrInvalidEpoch
	}

	// aborted or timed out DKGs can be reattempted at the same epoch
	if terms.Epoch == currentState.Epoch && currentState.State != Aborted && currentState.State != TimedOut {
		return ErrInvalidEpoch
	}

	if terms.Epoch > currentState.Epoch+1 && (currentState.State != Left && currentState.State != Fresh && currentState.State != Evicted) {
		return ErrInvalidEpoch
	}
	return nil
}

func validateFirstEpoch(terms *drand.ProposalTerms) error {
	if len(terms.GenesisSeed) != 0 {
		return ErrNoGenesisSeedForFirstEpoch
	}
	if terms.Remaining != nil || terms.Leaving != nil {
		return ErrOnlyJoinersAllowedForFirstEpoch
	}
	if !util.Contains(terms.Joining, terms.Leader) {
		return ErrLeaderNotJoining
	}
	if !terms.TransitionTime.AsTime().Equal(terms.GenesisTime.AsTime()) {
		return ErrTransitionTimeMustBeGenesisTime
	}

	return nil
}

func validateReshare(currentState *DBState, terms *drand.ProposalTerms) error {
	if !terms.GenesisTime.AsTime().Equal(currentState.GenesisTime) {
		return ErrGenesisTimeNotEqual
	}

	if !bytes.Equal(terms.GenesisSeed, currentState.GenesisSeed) {
		return ErrGenesisSeedCannotChange
	}

	if !util.ContainsAll(append(currentState.Remaining, currentState.Joining...), append(terms.Remaining, terms.Leaving...)) {
		return ErrRemainingAndLeavingNodesMustExistInCurrentEpoch
	}

	if !util.ContainsAll(append(terms.Remaining, terms.Leaving...), append(currentState.Remaining, currentState.Joining...)) {
		return ErrMissingNodesInProposal
	}

	return nil
}
