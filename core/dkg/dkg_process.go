package dkg

import (
	"context"
	"errors"
	"fmt"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"google.golang.org/protobuf/types/known/timestamppb"
	"reflect"
)

type DKGProcess struct {
	fetchIdentityForBeacon func(string) (*key.Identity, error)
	store                  DKGStore
	log                    log.Logger
}

type DKGStore interface {
	// GetCurrent returns the current DKG information, finished DKG information or fresh DKG information,
	// depending on the state of the world
	GetCurrent(beaconID string) (*DKGState, error)

	// GetFinished returns the last completed DKG state (i.e. completed or aborted), or nil if one has not been finished
	GetFinished(beaconID string) (*DKGState, error)

	// SaveCurrent stores a DKG packet for an ongoing DKG
	SaveCurrent(beaconID string, state *DKGState) error

	// SaveFinished stores a completed, successful DKG and overwrites the current packet
	SaveFinished(beaconID string, state *DKGState) error

	// Close closes and cleans up any database handles
	Close() error
}

func NewDKGProcess(store *DKGStore, fetchIdentityForBeacon func(string) (*key.Identity, error)) *DKGProcess {
	return &DKGProcess{
		store:                  *store,
		fetchIdentityForBeacon: fetchIdentityForBeacon,
		log:                    log.NewLogger(nil, log.LogDebug),
	}
}

func (d *DKGProcess) StartNetwork(context context.Context, options *drand.FirstProposalOptions) (*drand.GenericResponseMessage, error) {
	d.log.Debugw("Starting initial DKG")

	beaconID := options.BeaconID

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

	// remap the CLI payload into one useful for DKG state
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
	nextDKGState, err := currentState.Proposing(me, &terms)

	// for each of the joiners (other than ourselves), send the proposal
	// sequential... perhaps make this async
	for _, joiner := range nextDKGState.Joining {
		if joiner.Address == me.Address {
			continue
		}
		client, err := net.NewDKGClient(joiner.Address)
		if err != nil {
			return responseOrError(err)
		}
		response, err := client.Propose(context, &terms)
		if err != nil {
			return responseOrError(err)
		}

		if response.IsError {
			return responseOrError(errors.New(response.ErrorMessage))
		}
	}

	// save the new state
	err = d.store.SaveCurrent(beaconID, nextDKGState)
	if err != nil {
		d.log.Debugw("Error starting the network", "error", err, "beaconID", beaconID)
	} else {
		d.log.Infow("Finished starting the network", "beaconID", beaconID)
	}
	return responseOrError(err)
}

func (d *DKGProcess) StartProposal(context context.Context, options *drand.ProposalOptions) (*drand.GenericResponseMessage, error) {
	beaconID := options.BeaconID
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

	participants := append(append(nextState.Joining, nextState.Remaining...), nextState.Leaving...)
	for _, participant := range participants {
		if participant.Address == me.Address {
			continue
		}
		client, err := net.NewDKGClient(participant.Address)
		if err != nil {
			return responseOrError(err)
		}
		response, err := client.Propose(context, &terms)
		if err != nil {
			return responseOrError(err)
		}

		if response.IsError {
			return responseOrError(errors.New(response.ErrorMessage))
		}
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
	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	if !reflect.DeepEqual(current.Leader, me) {
		return responseOrError(errors.New("cannot abort the DKG if you aren't the leader"))
	}

	nextState, err := current.Aborted()
	if err != nil {
		return responseOrError(err)
	}

	recipients := append(append(nextState.Joining, nextState.Remaining...), nextState.Leaving...)
	for _, r := range recipients {
		if r.Address == me.Address {
			continue
		}
		client, err := net.NewDKGClient(r.Address)
		if err != nil {
			return responseOrError(err)
		}

		response, err := client.Abort(context, &drand.AbortDKG{Metadata: &drand.DKGMetadata{BeaconID: beaconID, Epoch: nextState.Epoch}})
		if err != nil {
			return responseOrError(err)
		}
		if response.IsError {
			return responseOrError(errors.New(response.ErrorMessage))
		}
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
	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	current, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return responseOrError(err)
	}

	if !reflect.DeepEqual(current.Leader, me) {
		return responseOrError(errors.New("cannot start execution if you aren't the leader"))
	}

	nextState, err := current.Executing(me)
	if err != nil {
		return responseOrError(err)
	}

	recipients := append(append(nextState.Joining, nextState.Remaining...), nextState.Leaving...)

	for _, r := range recipients {
		if r.Address == me.Address {
			continue
		}
		client, err := net.NewDKGClient(r.Address)
		if err != nil {
			return responseOrError(err)
		}

		response, err := client.Execute(context, &drand.StartExecution{Metadata: &drand.DKGMetadata{BeaconID: beaconID, Epoch: nextState.Epoch}})
		if err != nil {
			return responseOrError(err)
		}
		if response.IsError {
			return responseOrError(errors.New(response.ErrorMessage))
		}
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
	d.log.Debugw(fmt.Sprintf("Joining DKG for beacon %s", options.BeaconID))
	beaconID := options.BeaconID
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
		d.log.Infow("DKG execution started successfully", "beaconID", beaconID)
	}

	return responseOrError(err)
}

func (d *DKGProcess) StartAccept(context context.Context, options *drand.AcceptOptions) (*drand.GenericResponseMessage, error) {
	d.log.Info(fmt.Sprintf("Accepting DKG terms for beacon %s", options.BeaconID))
	beaconID := options.BeaconID
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
	d.log.Info(fmt.Sprintf("Rejecting DKG terms for beacon %s", options.BeaconID))
	beaconID := options.BeaconID
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
	identity, err := d.fetchIdentityForBeacon(beaconID)
	if err != nil {
		return nil, errors.New("could not load keypair")
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
	d.log.Info("DKG completed successfully!")
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
