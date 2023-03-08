package core

import (
	"context"
	"fmt"

	"github.com/drand/drand/protobuf/drand"
)

func (dd *DrandDaemon) StartNetwork(ctx context.Context, options *drand.FirstProposalOptions) (*drand.EmptyResponse, error) {
	return dd.dkg.StartNetwork(ctx, options)
}

func (dd *DrandDaemon) StartProposal(ctx context.Context, options *drand.ProposalOptions) (*drand.EmptyResponse, error) {
	return dd.dkg.StartProposal(ctx, options)
}

func (dd *DrandDaemon) StartAbort(ctx context.Context, options *drand.AbortOptions) (*drand.EmptyResponse, error) {
	beaconID := options.BeaconID

	if !dd.beaconExists(beaconID) {
		return nil, fmt.Errorf("beacon with ID %s is not running on this daemon", beaconID)
	}

	return dd.dkg.StartAbort(ctx, options)
}

func (dd *DrandDaemon) StartExecute(ctx context.Context, options *drand.ExecutionOptions) (*drand.EmptyResponse, error) {
	beaconID := options.BeaconID

	if !dd.beaconExists(beaconID) {
		return nil, fmt.Errorf("beacon with ID %s is not running on this daemon", beaconID)
	}

	return dd.dkg.StartExecute(ctx, options)
}

func (dd *DrandDaemon) StartJoin(ctx context.Context, options *drand.JoinOptions) (*drand.EmptyResponse, error) {
	beaconID := options.BeaconID

	if !dd.beaconExists(beaconID) {
		return nil, fmt.Errorf("beacon with ID %s is not running on this daemon", beaconID)
	}

	return dd.dkg.StartJoin(ctx, options)
}

func (dd *DrandDaemon) StartAccept(ctx context.Context, options *drand.AcceptOptions) (*drand.EmptyResponse, error) {
	beaconID := options.BeaconID

	if !dd.beaconExists(beaconID) {
		return nil, fmt.Errorf("beacon with ID %s is not running on this daemon", beaconID)
	}

	return dd.dkg.StartAccept(ctx, options)
}

func (dd *DrandDaemon) StartReject(ctx context.Context, options *drand.RejectOptions) (*drand.EmptyResponse, error) {
	beaconID := options.BeaconID

	if !dd.beaconExists(beaconID) {
		return nil, fmt.Errorf("beacon with ID %s is not running on this daemon", beaconID)
	}

	return dd.dkg.StartReject(ctx, options)
}

func (dd *DrandDaemon) DKGStatus(ctx context.Context, request *drand.DKGStatusRequest) (*drand.DKGStatusResponse, error) {
	beaconID := request.BeaconID

	if !dd.beaconExists(beaconID) {
		return nil, fmt.Errorf("beacon with ID %s is not running on this daemon", beaconID)
	}

	return dd.dkg.DKGStatus(ctx, request)
}

func (dd *DrandDaemon) Propose(ctx context.Context, terms *drand.ProposalTerms) (*drand.EmptyResponse, error) {
	return dd.dkg.Propose(ctx, terms)
}

func (dd *DrandDaemon) Abort(ctx context.Context, abortDKG *drand.AbortDKG) (*drand.EmptyResponse, error) {
	beaconID := abortDKG.Metadata.BeaconID

	if !dd.beaconExists(beaconID) {
		return nil, fmt.Errorf("beacon with ID %s is not running on this daemon", beaconID)
	}

	return dd.dkg.Abort(ctx, abortDKG)
}

func (dd *DrandDaemon) Execute(ctx context.Context, execution *drand.StartExecution) (*drand.EmptyResponse, error) {
	beaconID := execution.Metadata.BeaconID

	if !dd.beaconExists(beaconID) {
		return nil, fmt.Errorf("beacon with ID %s is not running on this daemon", beaconID)
	}

	return dd.dkg.Execute(ctx, execution)
}

func (dd *DrandDaemon) Accept(ctx context.Context, proposal *drand.AcceptProposal) (*drand.EmptyResponse, error) {
	beaconID := proposal.Metadata.BeaconID

	if !dd.beaconExists(beaconID) {
		return nil, fmt.Errorf("beacon with ID %s is not running on this daemon", beaconID)
	}

	return dd.dkg.Accept(ctx, proposal)
}

func (dd *DrandDaemon) Reject(ctx context.Context, proposal *drand.RejectProposal) (*drand.EmptyResponse, error) {
	beaconID := proposal.Metadata.BeaconID

	if !dd.beaconExists(beaconID) {
		return nil, fmt.Errorf("beacon with ID %s is not running on this daemon", beaconID)
	}

	return dd.dkg.Reject(ctx, proposal)
}

func (dd *DrandDaemon) BroadcastDKG(ctx context.Context, packet *drand.DKGPacket) (*drand.EmptyResponse, error) {
	beaconID := packet.Metadata.BeaconID

	if !dd.beaconExists(beaconID) {
		return nil, fmt.Errorf("beacon with ID %s is not running on this daemon", beaconID)
	}

	return dd.dkg.BroadcastDKG(ctx, packet)
}

func (dd *DrandDaemon) beaconExists(beaconID string) bool {
	_, exists := dd.beaconProcesses[beaconID]
	return exists
}
