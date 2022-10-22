package core

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
	GetCurrent(beaconID string) (*DKGDetails, error)

	// GetFinished returns the last completed DKG state (i.e. completed or aborted), or nil if one has not been finished
	GetFinished(beaconID string) (*DKGDetails, error)

	// SaveCurrent stores a DKG packet for an ongoing DKG
	SaveCurrent(beaconID string, state *DKGDetails) error

	// SaveFinished stores a completed, successful DKG and overwrites the current packet
	SaveFinished(beaconID string, state *DKGDetails) error

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
		return errorResponse(UnexpectedError, err), err
	}

	errorCode, err := executeDKGStateTransition[*drand.FirstProposalOptions, *ProposalTerms](
		d,
		options.BeaconID,
		FirstProposalMapping{me: me},
		options,
	)
	d.log.Debugw("Finished starting the network", "errorCode", errorCode.String(), "errorMessage", err)
	return responseOrError(errorCode, err)
}

func (d *DKGProcess) StartProposal(_ context.Context, options *drand.ProposalOptions) (*drand.GenericResponseMessage, error) {
	me, err := d.identityForBeacon(options.BeaconID)
	if err != nil {
		return errorResponse(UnexpectedError, err), err
	}
	errorCode, err := executeDKGStateTransition[*drand.ProposalOptions, *ProposalTerms](
		d,
		options.BeaconID,
		ProposalMapping{me: me},
		options,
	)
	d.log.Debugw("Finished starting the network", "errorCode", errorCode.String())
	return responseOrError(errorCode, err)
}

func (d *DKGProcess) StartAbort(_ context.Context, options *drand.AbortOptions) (*drand.GenericResponseMessage, error) {
	return nil, errors.New("not implemented")
}

func (d *DKGProcess) StartExecute(_ context.Context, options *drand.ExecutionOptions) (*drand.GenericResponseMessage, error) {
	return nil, errors.New("not implemented")
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
		Address:   identity.Address(),
		Tls:       identity.TLS,
		PubKey:    pubKey,
		Signature: identity.Signature,
	}, nil
}

// executeDKGStateTransition performs the mapping and state transitions for given DKG packet
// and updates the data store accordingly
func executeDKGStateTransition[T any, U any](
	d *DKGProcess,
	beaconID string,
	mapping DKGMapping[T, U],
	inputPacket T,
) (DKGErrorCode, error) {

	// remap the CLI payload into one useful for DKG state
	payload, err := mapping.Enrich(inputPacket)
	if err != nil {
		return InvalidPacket, err
	}

	// pull the latest DKG state from the database
	currentDKGState, err := d.store.GetCurrent(beaconID)
	if err != nil {
		return UnexpectedError, err
	}

	// apply our mapped DKG payload onto the current DKG state
	nextDKGState, errorCode := mapping.Apply(payload, currentDKGState)
	if errorCode != NoError {
		return errorCode, errors.New("error making DKG transition")
	}

	// save the output of the reducer
	if nextDKGState.State == Complete {
		err = d.store.SaveFinished(beaconID, nextDKGState)
	} else {
		err = d.store.SaveCurrent(beaconID, nextDKGState)
	}
	if err != nil {
		return UnexpectedError, err
	}

	return NoError, nil
}

// responseOrError takes a DKGErrorCode and maps it to an error object if an error
// or a generic success if it's not an error
func responseOrError(errorCode DKGErrorCode, err error) (*drand.GenericResponseMessage, error) {
	if errorCode != NoError {
		return errorResponse(errorCode, err), err
	}

	return &drand.GenericResponseMessage{}, nil
}

func errorResponse(errorCode DKGErrorCode, err error) *drand.GenericResponseMessage {
	return &drand.GenericResponseMessage{
		IsError:      true,
		ErrorMessage: err.Error(),
		ErrorCode:    uint32(errorCode),
	}
}
