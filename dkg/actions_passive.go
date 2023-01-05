package dkg

import (
	"context"
	"errors"
	"time"

	"github.com/drand/drand/protobuf/drand"
)

// actions_passive contains all internal messaging between nodes triggered by the protocol - things it does automatically
// upon receiving messages from other nodes: storing proposals, aborting when the leader aborts, etc

func (d *DKGProcess) Propose(_ context.Context, proposal *drand.ProposalTerms) (*drand.EmptyResponse, error) {
	err := d.executeAction("DKG proposal", proposal.BeaconID, func(me *drand.Participant, current *DBState) (*DBState, error) {
		// strictly speaking, we don't actually _know_ this proposal came from the leader here
		// it will have to be verified by signing later
		return current.Proposed(proposal.Leader, me, proposal)
	})

	return responseOrError(err)
}

func (d *DKGProcess) Accept(_ context.Context, acceptance *drand.AcceptProposal) (*drand.EmptyResponse, error) {
	err := d.executeAction("DKG acceptance", acceptance.Metadata.BeaconID, func(me *drand.Participant, current *DBState) (*DBState, error) {
		return current.ReceivedAcceptance(me, acceptance.Acceptor)
	})

	return responseOrError(err)
}

func (d *DKGProcess) Reject(_ context.Context, rejection *drand.RejectProposal) (*drand.EmptyResponse, error) {
	err := d.executeAction("DKG rejection", rejection.Metadata.BeaconID, func(me *drand.Participant, current *DBState) (*DBState, error) {
		return current.ReceivedRejection(me, rejection.Rejector)
	})

	return responseOrError(err)
}

func (d *DKGProcess) Abort(_ context.Context, abort *drand.AbortDKG) (*drand.EmptyResponse, error) {
	err := d.executeAction("abort DKG", abort.Metadata.BeaconID, func(_ *drand.Participant, current *DBState) (*DBState, error) {
		return current.Aborted()
	})

	return responseOrError(err)
}

func (d *DKGProcess) Execute(_ context.Context, kickoff *drand.StartExecution) (*drand.EmptyResponse, error) {
	beaconID := kickoff.Metadata.BeaconID

	err := d.executeAction("DKG execution", beaconID, func(me *drand.Participant, current *DBState) (*DBState, error) {
		return current.Executing(me)
	})

	if err != nil {
		d.log.Errorw("There was an error starting the DKG", "beaconID", beaconID, "error", err)
		return responseOrError(err)
	}

	d.log.Infow("DKG execution started successfully", "beaconID", beaconID)
	dkgConfig, err := d.setupDKG(beaconID)
	if err != nil {
		return nil, err
	}

	go func() {
		time.Sleep(d.config.KickoffGracePeriod)
		// copy this to avoid any data races with kyber
		dkgConfigCopy := *dkgConfig
		err := d.executeAndFinishDKG(beaconID, dkgConfigCopy)
		if err != nil {
			d.log.Errorw("there was an error during the DKG execution!", "beaconID", beaconID, "error", err)
		}
	}()

	return responseOrError(err)
}

// BroadcastDKG gossips internal DKG protocol messages to other nodes (i.e. any messages encapsulated in the Kyber DKG)
func (d *DKGProcess) BroadcastDKG(ctx context.Context, packet *drand.DKGPacket) (*drand.EmptyResponse, error) {
	beaconID := packet.Dkg.Metadata.BeaconID
	broadcaster := d.Executions[beaconID]
	if broadcaster == nil {
		return nil, errors.New("could not broadcast a DKG message - there may not be a DKG in progress and in the execution phase")
	}

	err := broadcaster.BroadcastDKG(ctx, packet)
	if err != nil {
		return nil, err
	}
	return &drand.EmptyResponse{}, nil
}
