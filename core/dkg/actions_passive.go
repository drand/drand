package dkg

import (
	"context"
	"github.com/drand/drand/protobuf/drand"
)

func (d *DKGProcess) Propose(_ context.Context, proposal *drand.ProposalTerms) (*drand.GenericResponseMessage, error) {
	beaconID := proposal.BeaconID
	d.log.Infow("Processing DKG proposal", "beaconID", beaconID, "leader", proposal.Leader.Address)

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	currentDKGState, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	nextDKGState, err := currentDKGState.Proposed(proposal.Leader, me, proposal)
	if err != nil {
		return responseOrError(err)
	}

	err = d.store.SaveCurrent(beaconID, nextDKGState)

	if err != nil {
		d.log.Errorw("Error processing DKG proposal", "beaconID", beaconID, "error", err)
	} else {
		d.log.Infow("DKG proposal processed successfully", "beaconID", beaconID)
	}
	return responseOrError(err)
}

func (d *DKGProcess) Accept(_ context.Context, acceptance *drand.AcceptProposal) (*drand.GenericResponseMessage, error) {
	beaconID := acceptance.Metadata.BeaconID
	d.log.Infow("Processing acceptance", "beaconID", beaconID, "acceptor", acceptance.Acceptor.Address)

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

	if err != nil {
		d.log.Errorw("Error processing DKG acceptance", "beaconID", beaconID, "error", err)
	} else {
		d.log.Infow("DKG acceptance successful", "beaconID", beaconID)
	}

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

	if err != nil {
		d.log.Errorw("Error processing DKG rejection", "beaconID", beaconID, "error", err)
	} else {
		d.log.Infow("DKG rejection successful", "beaconID", beaconID)
	}

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

	if err != nil {
		d.log.Errorw("Error processing DKG abort", "beaconID", beaconID, "error", err)
	} else {
		d.log.Infow("DKG aborted successfully", "beaconID", beaconID)
	}

	return responseOrError(err)
}

func (d *DKGProcess) Execute(_ context.Context, kickoff *drand.StartExecution) (*drand.GenericResponseMessage, error) {
	beaconID := kickoff.Metadata.BeaconID
	d.log.Infow("Starting DKG execution", "beaconID", beaconID)

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

	if err != nil {
		d.log.Errorw("Error starting execution of DKG", "beaconID", beaconID, "error", err)
	} else {
		d.log.Infow("DKG execution started successfully", "beaconID", beaconID)
	}

	go d.executeAndFinishDKG(beaconID)

	return responseOrError(err)
}
