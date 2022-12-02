package dkg

import (
	"context"
	"errors"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/util"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

func (d *DKGProcess) StartNetwork(context context.Context, options *drand.FirstProposalOptions) (*drand.GenericResponseMessage, error) {
	beaconID := options.BeaconID
	d.log.Infow("Starting initial DKG", "beaconID", beaconID)

	// fetch our keypair from the BeaconProcess and remap it into a `Participant`
	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return nil, err
	}

	// pull the latest DKG state from the database
	currentState, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return nil, err
	}

	genesisTime := options.GenesisTime
	if genesisTime == nil {
		genesisTime = timestamppb.New(time.Now())
	}

	// remap the CLI payload into one useful for applying to the DKG state
	terms := drand.ProposalTerms{
		BeaconID:    options.BeaconID,
		Threshold:   options.Threshold,
		Epoch:       1,
		Timeout:     options.Timeout,
		Leader:      me,
		SchemeID:    options.Scheme,
		GenesisTime: options.GenesisTime,
		GenesisSeed: nil, // is created after the DKG, so it cannot exist yet
		// for the initial proposal, we want the same transition time as the genesis time
		// ... or do we? are round 0 and round 1 the same time?
		TransitionTime:       options.GenesisTime,
		CatchupPeriodSeconds: options.CatchupPeriodSeconds,
		BeaconPeriodSeconds:  options.PeriodSeconds,
		Joining:              options.Joining,
	}

	// apply our enriched DKG payload onto the current DKG state to create a new state
	nextState, err := currentState.Proposing(me, &terms)
	if err != nil {
		return nil, err
	}

	sendProposalAndStoreNextState := func() error {
		err := d.network.Send(me, nextState.Joining, func(client drand.DKGClient) (*drand.GenericResponseMessage, error) {
			return client.Propose(context, &terms)
		})
		if err != nil {
			return err
		}

		if err = d.store.SaveCurrent(beaconID, nextState); err != nil {
			return err
		}

		d.log.Infow("Finished starting the network", "beaconID", beaconID)
		return nil
	}

	// if there's an error sending to a party or saving the state, attempt a rollback by issuing an abort
	rollback := func(err error) {
		d.log.Errorw("there was an error starting the network. Attempting rollback", "beaconID", beaconID, "error", err)
		_ = d.attemptAbort(context, me, nextState.Joining, beaconID, 1)
	}

	return responseOrError(rollbackOnError(sendProposalAndStoreNextState, rollback))
}

func rollbackOnError(fn func() error, attemptRollback func(error)) error {
	err := fn()
	if err != nil {
		attemptRollback(err)
		return err
	}
	return nil
}

func (d *DKGProcess) attemptAbort(context context.Context, me *drand.Participant, participants []*drand.Participant, beaconID string, epoch uint32) error {
	return d.network.Send(me, participants, func(client drand.DKGClient) (*drand.GenericResponseMessage, error) {
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
		return nil, err
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return nil, err
	}

	newEpoch := current.Epoch + 1
	terms := drand.ProposalTerms{
		BeaconID:             beaconID,
		Threshold:            options.Threshold,
		Epoch:                current.Epoch + 1,
		SchemeID:             current.SchemeID,
		BeaconPeriodSeconds:  uint32(current.BeaconPeriod.Seconds()),
		CatchupPeriodSeconds: options.CatchupPeriodSeconds,
		GenesisTime:          timestamppb.New(current.GenesisTime),
		GenesisSeed:          current.GenesisSeed,
		TransitionTime:       options.TransitionTime,
		Timeout:              options.Timeout,
		Leader:               me,
		Joining:              options.Joining,
		Remaining:            options.Remaining,
		Leaving:              options.Leaving,
	}

	nextState, err := current.Proposing(me, &terms)
	if err != nil {
		return nil, err
	}

	// sends the proposal to all participants of the DKG and stores the updated state in the DB
	allParticipants := concat(nextState.Joining, nextState.Remaining, nextState.Leaving)

	sendProposalToAllAndStoreState := func() error {
		err = d.network.Send(me, allParticipants, func(client drand.DKGClient) (*drand.GenericResponseMessage, error) {
			return client.Propose(context, &terms)
		})
		if err != nil {
			return err
		}

		if err = d.store.SaveCurrent(beaconID, nextState); err != nil {
			return err
		}

		d.log.Infow("Finished proposing a new DKG", "beaconID", beaconID)
		return nil
	}

	// if there's an error sending to a party or saving the state, attempt a rollback by issuing an abort
	rollback := func(err error) {
		d.log.Errorw("There was an error proposing a DKG", "err", err, "beaconID", beaconID)
		_ = d.attemptAbort(context, me, allParticipants, beaconID, newEpoch)
	}

	return responseOrError(rollbackOnError(sendProposalToAllAndStoreState, rollback))
}

func (d *DKGProcess) StartAbort(context context.Context, options *drand.AbortOptions) (*drand.GenericResponseMessage, error) {
	beaconID := options.BeaconID
	d.log.Infow("Aborting DKG", "beaconID", beaconID)

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return nil, err
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return nil, err
	}

	if util.EqualParticipant(current.Leader, me) {
		return nil, errors.New("cannot abort the DKG if you aren't the leader")
	}

	nextState, err := current.Aborted()
	if err != nil {
		return nil, err
	}

	allParticipants := concat(nextState.Joining, nextState.Remaining, nextState.Leaving)
	if err = d.attemptAbort(context, me, allParticipants, beaconID, nextState.Epoch); err != nil {
		return nil, err
	}

	if err = d.store.SaveCurrent(beaconID, nextState); err != nil {
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
		return nil, err
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return nil, err
	}

	if !util.EqualParticipant(current.Leader, me) {
		return nil, errors.New("cannot start execution if you aren't the leader")
	}

	nextState, err := current.Executing(me)
	if err != nil {
		return nil, err
	}

	allParticipants := concat(nextState.Joining, nextState.Remaining, nextState.Leaving)
	err = d.network.Send(me, allParticipants, func(client drand.DKGClient) (*drand.GenericResponseMessage, error) {
		return client.Execute(context, &drand.StartExecution{
			Metadata: &drand.DKGMetadata{
				BeaconID: beaconID,
				Epoch:    nextState.Epoch,
			}},
		)
	})
	if err != nil {
		return nil, err
	}

	if err = d.store.SaveCurrent(beaconID, nextState); err != nil {
		d.log.Errorw("error executing the DKG", "error", err, "beaconID", beaconID)
		return nil, err
	}

	d.log.Infow("DKG execution started successfully", "beaconID", beaconID)

	go func() {
		err := d.executeAndFinishDKG(beaconID)
		if err != nil {
			d.log.Errorw("there was an error during the DKG!", "beaconID", beaconID, "error", err)
		}
	}()

	return responseOrError(err)

}

func (d *DKGProcess) StartJoin(_ context.Context, options *drand.JoinOptions) (*drand.GenericResponseMessage, error) {
	beaconID := options.BeaconID
	d.log.Infow("Joining DKG", "beaconID", beaconID)

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return nil, err
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return nil, err
	}

	nextState, err := current.Joined(me)
	if err != nil {
		return nil, err
	}

	if err = d.store.SaveCurrent(beaconID, nextState); err != nil {
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
		return nil, err
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return nil, err
	}

	nextState, err := current.Accepted(me)
	if err != nil {
		return nil, err
	}

	client, err := net.NewDKGClient(nextState.Leader.Address)
	if err != nil {
		return nil, err
	}

	response, err := client.Accept(context, &drand.AcceptProposal{
		Acceptor: me,
		Metadata: &drand.DKGMetadata{
			BeaconID: options.BeaconID,
			Epoch:    current.Epoch,
		},
	})
	if err != nil {
		return nil, err
	}
	if response.IsError {
		return nil, errors.New(response.ErrorMessage)
	}

	if err = d.store.SaveCurrent(beaconID, nextState); err != nil {
		d.log.Errorw("error accepting the DKG", "error", err, "beaconID", beaconID)
	} else {
		d.log.Infow("DKG accepted successfully", "beaconID", beaconID)
	}
	return responseOrError(err)
}

func (d *DKGProcess) StartReject(context context.Context, options *drand.RejectOptions) (*drand.GenericResponseMessage, error) {
	beaconID := options.BeaconID
	d.log.Infow("Rejecting DKG terms", "beaconID", beaconID)

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return nil, err
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return nil, err
	}

	nextState, err := current.Rejected(me)
	if err != nil {
		return nil, err
	}

	client, err := net.NewDKGClient(nextState.Leader.Address)
	if err != nil {
		return nil, err
	}

	response, err := client.Reject(context, &drand.RejectProposal{
		Rejector: me,
		Metadata: &drand.DKGMetadata{
			BeaconID: options.BeaconID,
			Epoch:    current.Epoch,
		},
	})
	if err != nil {
		return nil, err
	}
	if response.IsError {
		return nil, errors.New(response.ErrorMessage)
	}

	if err = d.store.SaveCurrent(beaconID, nextState); err != nil {
		d.log.Errorw("error rejecting the DKG", "error", err, "beaconID", beaconID)
	} else {
		d.log.Infow("DKG rejected successfully", "beaconID", beaconID)
	}
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
	var finalGroup []string
	if current.FinalGroup != nil {
		finalGroup := make([]string, len(current.FinalGroup.Nodes))
		for i, v := range current.FinalGroup.Nodes {
			finalGroup[i] = v.Addr
		}
	}
	currentEntry := drand.DKGEntry{
		BeaconID:       current.BeaconID,
		State:          uint32(current.State),
		Epoch:          current.Epoch,
		Threshold:      current.Threshold,
		Timeout:        timestamppb.New(current.Timeout),
		GenesisTime:    timestamppb.New(current.GenesisTime),
		GenesisSeed:    current.GenesisSeed,
		TransitionTime: timestamppb.New(current.TransitionTime),
		Leader:         current.Leader,
		Remaining:      current.Remaining,
		Joining:        current.Joining,
		Leaving:        current.Leaving,
		Acceptors:      current.Acceptors,
		Rejectors:      current.Rejectors,
		FinalGroup:     finalGroup,
	}

	if finished == nil {
		return &drand.DKGStatusResponse{
			Current: &currentEntry,
		}, nil
	}
	finishedFinalGroup := make([]string, len(finished.FinalGroup.Nodes))
	for i, v := range current.FinalGroup.Nodes {
		finalGroup[i] = v.Addr
	}

	return &drand.DKGStatusResponse{
		Complete: &drand.DKGEntry{
			BeaconID:       finished.BeaconID,
			State:          uint32(finished.State),
			Epoch:          finished.Epoch,
			Threshold:      finished.Threshold,
			Timeout:        timestamppb.New(finished.Timeout),
			GenesisTime:    timestamppb.New(finished.GenesisTime),
			GenesisSeed:    finished.GenesisSeed,
			TransitionTime: timestamppb.New(finished.TransitionTime),
			Leader:         finished.Leader,
			Remaining:      finished.Remaining,
			Joining:        finished.Joining,
			Leaving:        finished.Leaving,
			Acceptors:      finished.Acceptors,
			Rejectors:      finished.Rejectors,
			FinalGroup:     finishedFinalGroup,
		},
		Current: &currentEntry,
	}, nil
}

// identityForBeacon grabs the key.Pair from a BeaconProcess and marshals it to a drand.Participant
func (d *DKGProcess) identityForBeacon(beaconID string) (*drand.Participant, error) {
	identity, err := d.beaconIdentifier.KeypairFor(beaconID)
	if err != nil {
		return nil, err
	}

	return util.PublicKeyAsParticipant(identity.Public)
}

func (d *DKGProcess) executeAndFinishDKG(beaconID string) error {
	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return err
	}

	lastCompleted, err := d.store.GetFinished(beaconID)
	if err != nil {
		return err
	}

	executeAndStoreDKG := func() error {
		output, err := d.executeDKG(beaconID, lastCompleted, current)
		if err != nil {
			return err
		}

		finalState, err := current.Complete(output.FinalGroup, output.KeyShare)
		if err != nil {
			return err
		}

		err = d.store.SaveFinished(beaconID, finalState)
		if err != nil {
			return err
		}

		d.completedDKGs <- DKGOutput{
			BeaconID: beaconID,
			Old:      lastCompleted,
			New:      *finalState,
		}

		d.log.Infow("DKG completed successfully!", "beaconID", beaconID)
		return nil
	}

	leaveNetwork := func(err error) {
		d.log.Errorw("There was an error during the DKG - we were likely evicted. Will attempt to store failed state", "error", err)
		// could this also be a timeout? is that semantically the same as eviction after DKG execution was triggered?
		evictedState, err := current.Evicted()
		if err != nil {
			d.log.Errorw("Failed to store failed state", "error", err)
			return
		}
		err = d.store.SaveCurrent(beaconID, evictedState)
		if err != nil {
			d.log.Errorw("Failed to store failed state", "error", err)
			return
		}
	}

	return rollbackOnError(executeAndStoreDKG, leaveNetwork)
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
