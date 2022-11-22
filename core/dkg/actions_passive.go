package dkg

import (
	"context"
	"github.com/drand/drand/protobuf/drand"
)

func (d *DKGProcess) Propose(_ context.Context, proposal *drand.ProposalTerms) (*drand.GenericResponseMessage, error) {
	beaconID := proposal.BeaconID
	d.log.Debugw("Processing DKG proposal", "beaconID", beaconID, "leader", proposal.Leader.Address)

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	currentDKGState, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	terms := &drand.ProposalTerms{
		BeaconID:  proposal.BeaconID,
		Threshold: proposal.Threshold,
		Epoch:     proposal.Epoch,
		Timeout:   proposal.Timeout,
		Leader:    proposal.Leader,
		Joining:   proposal.Joining,
		Remaining: proposal.Remaining,
		Leaving:   proposal.Leaving,
	}

	nextDKGState, err := currentDKGState.Proposed(proposal.Leader, me, terms)
	if err != nil {
		return responseOrError(err)
	}

	err = d.store.SaveCurrent(beaconID, nextDKGState)
	return responseOrError(err)
}

func (d *DKGProcess) Accept(_ context.Context, acceptance *drand.AcceptProposal) (*drand.GenericResponseMessage, error) {
	beaconID := acceptance.Metadata.BeaconID
	d.log.Debugw("Processing acceptance", "beaconID", beaconID, "acceptor", acceptance.Acceptor.Address)

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	nextState, err := current.ReceivedAcceptance(me, acceptance.Acceptor)
	if err != nil {
		return responseOrError(err)
	}
	err = d.store.SaveCurrent(beaconID, nextState)

	return responseOrError(err)
}

func (d *DKGProcess) Reject(_ context.Context, rejection *drand.RejectProposal) (*drand.GenericResponseMessage, error) {
	beaconID := rejection.Metadata.BeaconID
	d.log.Debugw("Processing rejection", "beaconID", beaconID, "rejector", rejection.Rejector.Address)

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	nextState, err := current.ReceivedRejection(me, rejection.Rejector)
	if err != nil {
		return responseOrError(err)
	}

	err = d.store.SaveCurrent(beaconID, nextState)
	return responseOrError(err)
}

func (d *DKGProcess) Abort(_ context.Context, abort *drand.AbortDKG) (*drand.GenericResponseMessage, error) {
	beaconID := abort.Metadata.BeaconID
	d.log.Debugw("Processing abort", "beaconID", beaconID)

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	nextState, err := current.Aborted()
	if err != nil {
		return responseOrError(err)
	}

	err = d.store.SaveCurrent(beaconID, nextState)
	return responseOrError(err)
}

func (d *DKGProcess) Execute(_ context.Context, kickoff *drand.StartExecution) (*drand.GenericResponseMessage, error) {
	beaconID := kickoff.Metadata.BeaconID
	d.log.Debugw("Starting DKG execution", "beaconID", beaconID)

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	current, err := d.store.GetCurrent(kickoff.Metadata.BeaconID)
	if err != nil {
		return responseOrError(err)
	}

	nextState, err := current.Executing(me)
	if err != nil {
		return responseOrError(err)
	}
	err = d.store.SaveCurrent(beaconID, nextState)

	go d.executeAndFinishDKG(beaconID)

	return responseOrError(err)
}
