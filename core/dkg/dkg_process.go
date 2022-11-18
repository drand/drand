package dkg

import (
	"context"
	"errors"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
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

func (d *DKGProcess) StartNetwork(_ context.Context, options *drand.FirstProposalOptions) (*drand.GenericResponseMessage, error) {
	d.log.Debugw("Starting initial DKG")

	// fetch our keypair from the BeaconProcess and remap it into a `Participant`
	me, err := d.identityForBeacon(options.BeaconID)
	if err != nil {
		return errorResponse(err), err
	}

	protocolSteps := FirstProposalSteps{
		me: me,
	}
	err = executeProtocolSteps[*drand.FirstProposalOptions, *drand.ProposalTerms, *drand.Proposal](
		d,
		options.BeaconID,
		protocolSteps,
		options,
	)
	if err != nil {
		d.log.Debugw("Error starting the network", "error", err)
	} else {
		d.log.Debugw("Finished starting the network")
	}
	return responseOrError(err)
}

func (d *DKGProcess) StartProposal(_ context.Context, options *drand.ProposalOptions) (*drand.GenericResponseMessage, error) {
	me, err := d.identityForBeacon(options.BeaconID)
	if err != nil {
		return errorResponse(err), err
	}

	protocolSteps := ProposalSteps{
		me:    me,
		store: d.store,
	}
	err = executeProtocolSteps[*drand.ProposalOptions, *drand.ProposalTerms, *drand.Proposal](
		d,
		options.BeaconID,
		protocolSteps,
		options,
	)
	d.log.Debugw("Finished starting the network", "errors?", err.Error())
	return responseOrError(err)
}

func (d *DKGProcess) StartAbort(_ context.Context, options *drand.AbortOptions) (*drand.GenericResponseMessage, error) {
	return nil, errors.New("not implemented")
}

func (d *DKGProcess) StartExecute(_ context.Context, options *drand.ExecutionOptions) (*drand.GenericResponseMessage, error) {
	return nil, errors.New("not implemented")
}

func (d *DKGProcess) StartJoin(_ context.Context, options *drand.JoinOptions) (*drand.GenericResponseMessage, error) {
	beaconID := options.BeaconID
	me, err := d.identityForBeacon(beaconID)
	if err != nil {
		return errorResponse(err), err
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
	return responseOrError(err)
}

func (d *DKGProcess) StartAccept(_ context.Context, options *drand.AcceptOptions) (*drand.GenericResponseMessage, error) {
	return nil, errors.New("not implemented")
}

func (d *DKGProcess) StartReject(_ context.Context, options *drand.RejectOptions) (*drand.GenericResponseMessage, error) {
	return nil, errors.New("not implemented")
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

	return &drand.DKGStatusResponse{
		Complete: finished.IntoEntry(),
		Current:  current.IntoEntry(),
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

// responseOrError takes a DKGErrorCode and maps it to an error object if an error
// or a generic success if it's not an error
func responseOrError(err error) (*drand.GenericResponseMessage, error) {
	if err != nil {
		return errorResponse(err), err
	}

	return &drand.GenericResponseMessage{}, nil
}

func errorResponse(err error) *drand.GenericResponseMessage {
	return &drand.GenericResponseMessage{
		IsError:      true,
		ErrorMessage: err.Error(),
	}
}
