package dkg

import (
	"context"
	"errors"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (d *DKGProcess) StartNetwork(context context.Context, options *drand.FirstProposalOptions) (*drand.GenericResponseMessage, error) {
	beaconID := options.BeaconID
	d.log.Infow("Starting initial DKG", "beaconID", beaconID)

	// fetch our keypair from the BeaconProcess and remap it into a `Participant`
	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	// pull the latest DKG state from the database
	currentState, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	// remap the CLI payload into one useful for applying to the DKG state
	terms := drand.ProposalTerms{
		BeaconID:             options.BeaconID,
		Threshold:            options.Threshold,
		Epoch:                1,
		Timeout:              options.Timeout,
		Leader:               me,
		Joining:              options.Joining,
		SchemeID:             options.Scheme,
		CatchupPeriodSeconds: options.CatchupPeriodSeconds,
		BeaconPeriodSeconds:  options.PeriodSeconds,
		Remaining:            nil,
		Leaving:              nil,
	}

	// apply our enriched DKG payload onto the current DKG state to create a new state
	nextState, err := currentState.Proposing(me, &terms)
	if err != nil {
		return responseOrError(err)
	}

	sendProposalAndStoreNextState := func() error {
		err := d.network.Send(me, nextState.Joining, func(client drand.DKGClient) (*drand.GenericResponseMessage, error) {
			return client.Propose(context, &terms)
		})
		if err != nil {
			return err
		}

		err = d.store.SaveCurrent(beaconID, nextState)
		if err != nil {
			return err
		}

		d.log.Infow("Finished starting the network", "beaconID", beaconID)
		return nil
	}

	// if there's an error sending to a party or saving the state, attempt a rollback by issuing an abort
	rollback := func(err error) {
		d.log.Errorw("there was an error starting the network. Attempting rollback", "beaconID", beaconID, "error", err)
		d.attemptAbort(context, me, nextState.Joining, beaconID, 1)
	}

	err = rollbackOnError(sendProposalAndStoreNextState, rollback)
	return responseOrError(err)
}

func rollbackOnError(fn func() error, attemptRollback func(error)) error {
	err := fn()
	if err != nil {
		attemptRollback(err)
		return err
	}
	return nil
}

func (d *DKGProcess) attemptAbort(context context.Context, me *drand.Participant, participants []*drand.Participant, beaconID string, epoch uint32) {
	_ = d.network.Send(me, participants, func(client drand.DKGClient) (*drand.GenericResponseMessage, error) {
		return client.Abort(context, &drand.AbortDKG{Metadata: &drand.DKGMetadata{
			BeaconID: beaconID,
			Epoch:    epoch,
		}})
	})
}

func (d *DKGProcess) StartProposal(context context.Context, options *drand.ProposalOptions) (*drand.GenericResponseMessage, error) {
	beaconID := options.BeaconID
	d.log.Infow("Proposing DKG", "beaconID", beaconID)

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return nil, err
	}

	terms := drand.ProposalTerms{
		BeaconID:             beaconID,
		Threshold:            options.Threshold,
		Epoch:                current.Epoch + 1,
		SchemeID:             current.SchemeID,
		BeaconPeriodSeconds:  uint32(current.BeaconPeriod.Seconds()),
		CatchupPeriodSeconds: options.CatchupPeriodSeconds,
		Timeout:              options.Timeout,
		Leader:               me,
		Joining:              options.Joining,
		Remaining:            options.Remaining,
		Leaving:              options.Leaving,
	}

	nextState, err := current.Proposing(me, &terms)
	if err != nil {
		return responseOrError(err)
	}

	allParticipants := concat(nextState.Joining, nextState.Remaining, nextState.Leaving)
	err = d.network.Send(me, allParticipants, func(client drand.DKGClient) (*drand.GenericResponseMessage, error) {
		return client.Propose(context, &terms)
	})
	if err != nil {
		return responseOrError(err)
	}

	err = d.store.SaveCurrent(beaconID, nextState)
	if err != nil {
		d.log.Errorw("There was an error proposing a DKG", "err", err, "beaconID", beaconID)
	} else {
		d.log.Infow("Finished proposing a new DKG", "beaconID", beaconID)
	}

	return responseOrError(err)
}

func (d *DKGProcess) StartAbort(context context.Context, options *drand.AbortOptions) (*drand.GenericResponseMessage, error) {
	beaconID := options.BeaconID
	d.log.Infow("Aborting DKG", "beaconID", beaconID)

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	if equalParticipant(current.Leader, me) {
		return responseOrError(errors.New("cannot abort the DKG if you aren't the leader"))
	}

	nextState, err := current.Aborted()
	if err != nil {
		return responseOrError(err)
	}

	allParticipants := concat(nextState.Joining, nextState.Remaining, nextState.Leaving)
	abort := drand.AbortDKG{Metadata: &drand.DKGMetadata{BeaconID: beaconID, Epoch: nextState.Epoch}}
	err = d.network.Send(me, allParticipants, func(client drand.DKGClient) (*drand.GenericResponseMessage, error) {
		return client.Abort(context, &abort)
	})
	if err != nil {
		return responseOrError(err)
	}

	err = d.store.SaveCurrent(beaconID, nextState)
	if err != nil {
		d.log.Errorw("error aborting the DKG", "error", err, "beaconID", beaconID)
	} else {
		d.log.Infow("DKG aborted successfully", "beaconID", beaconID)
	}
	return responseOrError(err)

}

func (d *DKGProcess) StartExecute(context context.Context, options *drand.ExecutionOptions) (*drand.GenericResponseMessage, error) {
	beaconID := options.BeaconID
	d.log.Infow("Starting execution of DKG", "beaconID", beaconID)

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	if !equalParticipant(current.Leader, me) {
		return responseOrError(errors.New("cannot start execution if you aren't the leader"))
	}

	nextState, err := current.Executing(me)
	if err != nil {
		return responseOrError(err)
	}

	allParticipants := concat(nextState.Joining, nextState.Remaining, nextState.Leaving)
	execution := drand.StartExecution{Metadata: &drand.DKGMetadata{BeaconID: beaconID, Epoch: nextState.Epoch}}
	err = d.network.Send(me, allParticipants, func(client drand.DKGClient) (*drand.GenericResponseMessage, error) {
		return client.Execute(context, &execution)
	})
	if err != nil {
		return responseOrError(err)
	}

	err = d.store.SaveCurrent(beaconID, nextState)
	if err != nil {
		d.log.Errorw("error executing the DKG", "error", err, "beaconID", beaconID)
		return responseOrError(err)
	} else {
		d.log.Infow("DKG execution started successfully", "beaconID", beaconID)
	}

	go d.executeAndFinishDKG(beaconID)
	return responseOrError(err)

}

func (d *DKGProcess) StartJoin(_ context.Context, options *drand.JoinOptions) (*drand.GenericResponseMessage, error) {
	beaconID := options.BeaconID
	d.log.Infow("Joining DKG", "beaconID", beaconID)

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	nextState, err := current.Joined(me)
	if err != nil {
		return responseOrError(err)
	}

	err = d.store.SaveCurrent(beaconID, nextState)
	if err != nil {
		d.log.Errorw("error joining the DKG", "error", err, "beaconID", beaconID)
	} else {
		d.log.Infow("DKG joined successfully", "beaconID", beaconID)
	}

	return responseOrError(err)
}

func (d *DKGProcess) StartAccept(context context.Context, options *drand.AcceptOptions) (*drand.GenericResponseMessage, error) {
	beaconID := options.BeaconID
	d.log.Infow("Accepting DKG terms", "beaconID", beaconID)

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	nextState, err := current.Accepted(me)
	if err != nil {
		return responseOrError(err)
	}

	client, err := net.NewDKGClient(nextState.Leader.Address)
	if err != nil {
		return responseOrError(err)
	}

	acceptance := drand.AcceptProposal{
		Acceptor: me,
		Metadata: &drand.DKGMetadata{
			BeaconID: options.BeaconID,
			Epoch:    current.Epoch,
		},
	}
	response, err := client.Accept(context, &acceptance)
	if err != nil {
		return responseOrError(err)
	}
	if response.IsError {
		return responseOrError(errors.New(response.ErrorMessage))
	}

	err = d.store.SaveCurrent(beaconID, nextState)
	return responseOrError(err)
}

func (d *DKGProcess) StartReject(context context.Context, options *drand.RejectOptions) (*drand.GenericResponseMessage, error) {
	beaconID := options.BeaconID
	d.log.Infow("Rejecting DKG terms", "beaconID", beaconID)

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	nextState, err := current.Rejected(me)
	if err != nil {
		return responseOrError(err)
	}

	client, err := net.NewDKGClient(nextState.Leader.Address)
	if err != nil {
		return responseOrError(err)
	}

	rejection := drand.RejectProposal{
		Rejector: me,
		Metadata: &drand.DKGMetadata{
			BeaconID: options.BeaconID,
			Epoch:    current.Epoch,
		},
	}
	response, err := client.Reject(context, &rejection)
	if err != nil {
		return responseOrError(err)
	}
	if response.IsError {
		return responseOrError(errors.New(response.ErrorMessage))
	}

	err = d.store.SaveCurrent(beaconID, nextState)
	return responseOrError(err)
}

func (d *DKGProcess) DKGStatus(_ context.Context, request *drand.DKGStatusRequest) (*drand.DKGStatusResponse, error) {
	finished, err := d.store.GetFinished(request.BeaconID)
	if err != nil {
		return nil, err
	}
	current, err := d.store.GetCurrent(request.BeaconID)
	if err != nil {
		return nil, err
	}
	currentEntry := drand.DKGEntry{
		BeaconID:   current.BeaconID,
		State:      uint32(current.State),
		Epoch:      current.Epoch,
		Threshold:  current.Threshold,
		Timeout:    timestamppb.New(current.Timeout),
		Leader:     current.Leader,
		Remaining:  current.Remaining,
		Joining:    current.Joining,
		Leaving:    current.Leaving,
		Acceptors:  current.Acceptors,
		Rejectors:  current.Rejectors,
		FinalGroup: current.FinalGroup,
	}

	if finished == nil {
		return &drand.DKGStatusResponse{
			Current: &currentEntry,
		}, nil
	}

	return &drand.DKGStatusResponse{
		Complete: &drand.DKGEntry{
			BeaconID:   finished.BeaconID,
			State:      uint32(finished.State),
			Epoch:      finished.Epoch,
			Threshold:  finished.Threshold,
			Timeout:    timestamppb.New(finished.Timeout),
			Leader:     finished.Leader,
			Remaining:  finished.Remaining,
			Joining:    finished.Joining,
			Leaving:    finished.Leaving,
			Acceptors:  finished.Acceptors,
			Rejectors:  finished.Rejectors,
			FinalGroup: finished.FinalGroup,
		},
		Current: &currentEntry,
	}, nil
}

// identityForBeacon grabs the key.Identity from a BeaconProcess and marshals it to a drand.Participant
func (d *DKGProcess) identityForBeacon(beaconID string) (*drand.Participant, error) {
	identity, err := d.beaconIdentifier.IdentityFor(beaconID)
	if err != nil {
		return nil, err
	}

	pubKey, err := identity.Key.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return &drand.Participant{
		Address: identity.Address(),
		Tls:     identity.TLS,
		PubKey:  pubKey,
	}, nil
}

func (d *DKGProcess) executeAndFinishDKG(beaconID string) {
	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		d.log.Errorw("there was an error completing the DKG!")
		return
	}

	finalGroup := append(current.Remaining, current.Joining...)
	finalState, err := current.Complete(finalGroup)
	err = d.store.SaveFinished(beaconID, finalState)

	if err != nil {
		d.log.Errorw("there was an error completing the DKG!")
		return
	}
	d.log.Infow("DKG completed successfully!", "beaconID", beaconID)
}

// responseOrError takes a DKGErrorCode and maps it to an error object if an error
// or a generic success if it's not an error
func responseOrError(err error) (*drand.GenericResponseMessage, error) {
	if err != nil {
		return &drand.GenericResponseMessage{
			IsError:      true,
			ErrorMessage: err.Error(),
		}, err
	}

	return &drand.GenericResponseMessage{}, nil
}

// concat takes a variable number of Participant arrays and combines them into a single array
func concat(arrs ...[]*drand.Participant) []*drand.Participant {
	var output []*drand.Participant

	for _, v := range arrs {
		output = append(output, v...)
	}

	return output
}
