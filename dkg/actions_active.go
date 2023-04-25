package dkg

import (
	"context"
	"errors"
	"time"

	"github.com/drand/drand/key"

	"github.com/drand/drand/net"

	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/util"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// actions_active contains all the DKG actions that require user interaction: creating a network,
// accepting or rejecting a DKG, getting the status, etc. Both leader and follower interactions are contained herein.

func (d *DKGProcess) StartNetwork(ctx context.Context, options *drand.FirstProposalOptions) (*drand.EmptyResponse, error) {
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
		GenesisTime: genesisTime,
		// GenesisSeed is created after the DKG, so it cannot exist yet
		GenesisSeed: nil,
		// for the initial proposal, we want the transition time should be the same as the genesis time
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
		err := d.network.Send(me, nextState.Joining, func(client net.DKGClient, peer net.Peer) (*drand.EmptyResponse, error) {
			return client.Propose(ctx, peer, &terms)
		})
		if err != nil {
			return err
		}

		if err := d.store.SaveCurrent(beaconID, nextState); err != nil {
			return err
		}

		d.log.Infow("Finished starting the network", "beaconID", beaconID)
		return nil
	}

	// if there's an error sending to a party or saving the state, attempt a rollback by issuing an abort
	rollback := func(err error) {
		d.log.Errorw("there was an error starting the network. Attempting rollback", "beaconID", beaconID, "error", err)
		_ = d.attemptAbort(ctx, me, nextState.Joining, beaconID)
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

func (d *DKGProcess) attemptAbort(
	ctx context.Context,
	me *drand.Participant,
	participants []*drand.Participant,
	beaconID string,
) error {
	return d.network.Send(me, participants, func(client net.DKGClient, peer net.Peer) (*drand.EmptyResponse, error) {
		return client.Abort(
			ctx,
			peer,
			&drand.AbortDKG{Metadata: &drand.DKGMetadata{
				BeaconID: beaconID,
			}})
	})
}

func (d *DKGProcess) StartProposal(ctx context.Context, options *drand.ProposalOptions) (*drand.EmptyResponse, error) {
	beaconID := options.BeaconID
	d.log.Infow("Proposing DKG", "beaconID", beaconID)

	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return nil, err
	}

	lastDKG, err := d.store.GetFinished(beaconID)
	if err != nil {
		return nil, err
	}

	newEpoch := lastDKG.Epoch + 1
	terms := drand.ProposalTerms{
		BeaconID:             beaconID,
		Threshold:            options.Threshold,
		Epoch:                newEpoch,
		SchemeID:             lastDKG.SchemeID,
		BeaconPeriodSeconds:  uint32(lastDKG.BeaconPeriod.Seconds()),
		CatchupPeriodSeconds: options.CatchupPeriodSeconds,
		GenesisTime:          timestamppb.New(lastDKG.GenesisTime),
		GenesisSeed:          lastDKG.GenesisSeed,
		TransitionTime:       options.TransitionTime,
		Timeout:              options.Timeout,
		Leader:               me,
		Joining:              options.Joining,
		Remaining:            options.Remaining,
		Leaving:              options.Leaving,
	}

	nextState, err := lastDKG.Proposing(me, &terms)
	if err != nil {
		return nil, err
	}

	sendProposalToAllAndStoreState := func() error {
		// we send the proposal to joiners and remainers and error if they don't respond
		err = d.network.Send(
			me,
			util.Concat(nextState.Joining, nextState.Remaining),
			func(client net.DKGClient, peer net.Peer) (*drand.EmptyResponse, error) {
				return client.Propose(ctx, peer, &terms)
			},
		)
		if err != nil {
			return err
		}

		// we make a best effort attempt to send the proposal to the leaver, but if their node is e.g. turned off then
		// we ignore the error
		if len(nextState.Leaving) > 0 {
			err = d.network.Send(me, nextState.Leaving, func(client net.DKGClient, peer net.Peer) (*drand.EmptyResponse, error) {
				return client.Propose(ctx, peer, &terms)
			})

			if err != nil {
				d.log.Warnw("could not send proposal to a leaving participant", "err", err)
			}
		}

		if err := d.store.SaveCurrent(beaconID, nextState); err != nil {
			return err
		}

		d.log.Infow("Finished proposing a new DKG", "beaconID", beaconID)
		return nil
	}

	// if there's an error sending to a party or saving the state, attempt a rollback by issuing an abort
	rollback := func(err error) {
		allParticipants := util.Concat(nextState.Joining, nextState.Remaining, nextState.Leaving)
		d.log.Errorw("There was an error proposing a DKG", "err", err, "beaconID", beaconID)
		_ = d.attemptAbort(ctx, me, allParticipants, beaconID)
	}

	return responseOrError(rollbackOnError(sendProposalToAllAndStoreState, rollback))
}

func (d *DKGProcess) StartAbort(ctx context.Context, options *drand.AbortOptions) (*drand.EmptyResponse, error) {
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

	if !util.EqualParticipant(current.Leader, me) {
		return nil, errors.New("cannot abort the DKG if you aren't the leader")
	}

	nextState, err := current.Aborted()
	if err != nil {
		return nil, err
	}

	allParticipants := util.Concat(nextState.Joining, nextState.Remaining, nextState.Leaving)
	if err := d.attemptAbort(ctx, me, allParticipants, beaconID); err != nil {
		return nil, err
	}

	if err := d.store.SaveCurrent(beaconID, nextState); err != nil {
		d.log.Errorw("error aborting the DKG", "error", err, "beaconID", beaconID)
	} else {
		d.log.Infow("DKG aborted successfully", "beaconID", beaconID)
	}
	return responseOrError(err)
}

func (d *DKGProcess) StartExecute(ctx context.Context, options *drand.ExecutionOptions) (*drand.EmptyResponse, error) {
	beaconID := options.BeaconID

	stateTransition := func(me *drand.Participant, current *DBState) (*DBState, error) {
		if !util.EqualParticipant(current.Leader, me) {
			return nil, errors.New("cannot start execution if you aren't the leader")
		}
		return current.Executing(me)
	}

	callback := func(me *drand.Participant, nextState *DBState) error {
		allParticipants := util.Concat(nextState.Joining, nextState.Remaining, nextState.Leaving)
		return d.network.SendIgnoringConnectionError(
			me,
			allParticipants,
			func(client net.DKGClient, peer net.Peer) (*drand.EmptyResponse, error) {
				return client.Execute(ctx, peer, &drand.StartExecution{
					Metadata: &drand.DKGMetadata{
						BeaconID: beaconID,
					}},
				)
			},
		)
	}

	err := d.executeActionWithCallback("DKG execution", beaconID, stateTransition, callback)

	if err != nil {
		return nil, err
	}

	// set up the DKG broadcaster for first so we're ready to broadcast DKG messages
	dkgConfig, err := d.setupDKG(beaconID)
	if err != nil {
		return nil, err
	}

	d.log.Infow("DKG execution leader setup successful", "beaconID", beaconID)

	go func() {
		// wait for `KickOffGracePeriod` to allow other nodes to set up their broadcasters
		time.Sleep(d.config.KickoffGracePeriod)
		// copy this to avoid any data races with kyber
		dkgConfigCopy := *dkgConfig
		err := d.executeAndFinishDKG(beaconID, dkgConfigCopy)
		if err != nil {
			d.log.Errorw("there was an error during the DKG!", "beaconID", beaconID, "error", err)
		}
	}()

	return responseOrError(err)
}

func (d *DKGProcess) StartJoin(_ context.Context, options *drand.JoinOptions) (*drand.EmptyResponse, error) {
	beaconID := options.BeaconID

	err := d.executeAction("Joining DKG", beaconID, func(me *drand.Participant, current *DBState) (*DBState, error) {
		var previousGroupFile *key.Group
		if current.Epoch > 1 {
			p, err := util.ParseGroupFileBytes(options.GroupFile)
			if err != nil {
				return nil, err
			}
			previousGroupFile = p
		}

		return current.Joined(me, previousGroupFile)
	})

	return responseOrError(err)
}

// StartAccept
// don't believe the lying linter
//
//nolint:dupl
func (d *DKGProcess) StartAccept(ctx context.Context, options *drand.AcceptOptions) (*drand.EmptyResponse, error) {
	beaconID := options.BeaconID

	stateTransition := func(me *drand.Participant, current *DBState) (*DBState, error) {
		return current.Accepted(me)
	}

	callback := func(me *drand.Participant, nextState *DBState) error {
		_, err := d.internalClient.Accept(
			ctx,
			util.ToPeer(nextState.Leader),
			&drand.AcceptProposal{
				Acceptor: me,
				Metadata: &drand.DKGMetadata{
					BeaconID: beaconID,
				},
			})
		return err
	}

	err := d.executeActionWithCallback("DKG acceptance", beaconID, stateTransition, callback)
	return responseOrError(err)
}

// StartReject
// don't believe the lying linter
//
//nolint:dupl
func (d *DKGProcess) StartReject(ctx context.Context, options *drand.RejectOptions) (*drand.EmptyResponse, error) {
	beaconID := options.BeaconID

	stateTransition := func(me *drand.Participant, current *DBState) (*DBState, error) {
		return current.Rejected(me)
	}

	callback := func(me *drand.Participant, nextState *DBState) error {
		_, err := d.internalClient.Reject(ctx,
			util.ToPeer(nextState.Leader),
			&drand.RejectProposal{
				Rejector: me,
				Metadata: &drand.DKGMetadata{
					BeaconID: beaconID,
				},
			})
		return err
	}

	err := d.executeActionWithCallback("DKG rejection", beaconID, stateTransition, callback)
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
	for i, v := range finished.FinalGroup.Nodes {
		finishedFinalGroup[i] = v.Addr
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

// responseOrError takes a DKGErrorCode and maps it to an error object if an error
// or a generic success if it's not an error
func responseOrError(err error) (*drand.EmptyResponse, error) {
	if err != nil {
		return nil, err
	}

	return &drand.EmptyResponse{}, nil
}
