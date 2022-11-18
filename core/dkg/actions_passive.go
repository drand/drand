package dkg

import (
	"context"
	"github.com/drand/drand/protobuf/drand"
)

func (d *DKGProcess) Propose(_ context.Context, proposal *drand.Proposal) (*drand.GenericResponseMessage, error) {
	beaconID := proposal.BeaconID
	d.log.Debugw("Processing proposal", "beaconID", beaconID)

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return errorResponse(err), err
	}

	currentDKGState, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return errorResponse(err), err
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
	d.log.Debugw("Processing acceptance", "beaconID", beaconID)

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	current, err := d.store.GetCurrent(acceptance.Metadata.BeaconID)
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
	return errResponse(), nil
}

func (d *DKGProcess) Abort(_ context.Context, abort *drand.AbortDKG) (*drand.GenericResponseMessage, error) {
	return errResponse(), nil
}

func (d *DKGProcess) Execute(_ context.Context, kickoff *drand.StartExecution) (*drand.GenericResponseMessage, error) {
	return errResponse(), nil
}

func errResponse() *drand.GenericResponseMessage {
	return &drand.GenericResponseMessage{
		IsError:      true,
		ErrorMessage: "this call has not yet been implemented for this service",
	}
}
