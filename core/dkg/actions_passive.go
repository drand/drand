package dkg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/drand/drand/protobuf/drand"
)

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
		err := d.executeAndFinishDKG(beaconID, dkgConfig)
		if err != nil {
			d.log.Errorw("there was an error during the DKG execution!", "beaconID", beaconID, "error", err)
		}
	}()

	return responseOrError(err)
}

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

// executeAction fetches the latest DKG state, applies the action to it and stores it back in the database
func (d *DKGProcess) executeAction(
	name string,
	beaconID string,
	action func(me *drand.Participant, current *DBState) (*DBState, error),
) error {
	return d.executeActionWithCallback(name, beaconID, action, nil)
}

// executeActionWithCallback fetches the latest DKG state, applies the action to it, passes that new state
// to a callback then stores the new state in the database if the callback was successful
func (d *DKGProcess) executeActionWithCallback(
	name string,
	beaconID string,
	createNewState func(me *drand.Participant, current *DBState) (*DBState, error),
	callback func(me *drand.Participant, newState *DBState) error,
) error {
	var err error
	d.log.Infow(fmt.Sprintf("Processing %s", name), "beaconID", beaconID)

	defer func() {
		if err != nil {
			d.log.Errorw(fmt.Sprintf("Error processing %s", name), "beaconID", beaconID, "error", err)
		} else {
			d.log.Infow(fmt.Sprintf("%s successful", name), "beaconID", beaconID)
		}
	}()

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return err
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return err
	}

	nextState, err := createNewState(me, current)
	if err != nil {
		return err
	}

	if callback != nil {
		err = callback(me, nextState)
		if err != nil {
			return err
		}
	}
	err = d.store.SaveCurrent(beaconID, nextState)
	return err
}
