package dkg

import (
	"context"
	"errors"
	"time"

	"github.com/drand/drand/internal/metrics"
	"github.com/drand/drand/protobuf/drand"
)

// actions_passive contains all internal messaging between nodes triggered by the protocol - things it does automatically
// upon receiving messages from other nodes: storing proposals, aborting when the leader aborts, etc

func (d *Process) Propose(ctx context.Context, proposal *drand.ProposalTerms) (*drand.EmptyResponse, error) {
	_, span := metrics.NewSpan(ctx, "dkg.Propose")
	defer span.End()

	if proposal.Epoch == 1 {
		err := d.verifyMessage("StartNetwork", proposal.Metadata, proposal)
		if err != nil {
			return nil, err
		}
	} else {
		err := d.verifyMessage("StartProposal", proposal.Metadata, proposal)
		if err != nil {
			return nil, err
		}
	}

	err := d.executeAction("DKG proposal", proposal.BeaconID, func(me *drand.Participant, current *DBState) (*DBState, error) {
		// strictly speaking, we don't actually _know_ this proposal came from the leader here
		// it will have to be verified by signing later
		return current.Proposed(proposal.Leader, me, proposal)
	})

	return responseOrError(err)
}

//nolint:dupl // it's similar to Reject, but not the same
func (d *Process) Accept(ctx context.Context, acceptance *drand.AcceptProposal) (*drand.EmptyResponse, error) {
	_, span := metrics.NewSpan(ctx, "dkg.Accept")
	defer span.End()

	err := d.executeAction("DKG acceptance", acceptance.Metadata.BeaconID, func(me *drand.Participant, current *DBState) (*DBState, error) {
		err := d.verifyMessage("StartAccept", acceptance.Metadata, termsFromState(current))
		if err != nil {
			return nil, err
		}
		return current.ReceivedAcceptance(me, acceptance.Acceptor)
	})

	return responseOrError(err)
}

//nolint:dupl // it's similar to Accept, but not the same
func (d *Process) Reject(ctx context.Context, rejection *drand.RejectProposal) (*drand.EmptyResponse, error) {
	_, span := metrics.NewSpan(ctx, "dkg.Reject")
	defer span.End()
	err := d.executeAction("DKG rejection", rejection.Metadata.BeaconID, func(me *drand.Participant, current *DBState) (*DBState, error) {
		err := d.verifyMessage("StartReject", rejection.Metadata, termsFromState(current))
		if err != nil {
			return nil, err
		}
		return current.ReceivedRejection(me, rejection.Rejector)
	})

	return responseOrError(err)
}

func (d *Process) Abort(ctx context.Context, abort *drand.AbortDKG) (*drand.EmptyResponse, error) {
	_, span := metrics.NewSpan(ctx, "dkg.Abort")
	defer span.End()

	err := d.executeAction("abort DKG", abort.Metadata.BeaconID, func(_ *drand.Participant, current *DBState) (*DBState, error) {
		err := d.verifyMessage("StartAbort", abort.Metadata, termsFromState(current))
		if err != nil {
			return nil, err
		}
		return current.Aborted()
	})

	return responseOrError(err)
}

func (d *Process) Execute(ctx context.Context, kickoff *drand.StartExecution) (*drand.EmptyResponse, error) {
	ctx, span := metrics.NewSpan(ctx, "dkg.Execute")
	defer span.End()
	beaconID := kickoff.Metadata.BeaconID

	err := d.executeAction("DKG execution", beaconID, func(me *drand.Participant, current *DBState) (*DBState, error) {
		err := d.verifyMessage("StartExecute", kickoff.Metadata, termsFromState(current))
		if err != nil {
			return nil, err
		}
		return current.Executing(me)
	})

	if err != nil {
		d.log.Errorw("There was an error starting the DKG", "beaconID", beaconID, "error", err)
		return responseOrError(err)
	}

	d.log.Infow("DKG execution started successfully", "beaconID", beaconID)
	dkgConfig, err := d.setupDKG(ctx, beaconID)
	if err != nil {
		return nil, err
	}

	d.log.Infow("DKG execution setup successful", "beaconID", beaconID)

	go func() {
		time.Sleep(d.config.KickoffGracePeriod)
		// copy this to avoid any data races with kyber
		dkgConfigCopy := *dkgConfig
		err := d.executeAndFinishDKG(ctx, beaconID, dkgConfigCopy)
		if err != nil {
			d.log.Errorw("there was an error during the DKG execution!", "beaconID", beaconID, "error", err)
		}
	}()

	return responseOrError(err)
}

// BroadcastDKG gossips internal DKG protocol messages to other nodes (i.e. any messages encapsulated in the Kyber DKG)
func (d *Process) BroadcastDKG(ctx context.Context, packet *drand.DKGPacket) (*drand.EmptyResponse, error) {
	_, span := metrics.NewSpan(ctx, "dkg.BroadcastDKG")
	defer span.End()
	beaconID := packet.Dkg.Metadata.BeaconID
	d.lock.Lock()
	broadcaster := d.Executions[beaconID]
	d.lock.Unlock()
	if broadcaster == nil {
		return nil, errors.New("could not broadcast a DKG message - there may not be a DKG in progress and in the execution phase")
	}

	err := broadcaster.BroadcastDKG(ctx, packet)
	if err != nil {
		return nil, err
	}
	return &drand.EmptyResponse{}, nil
}
