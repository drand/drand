package core

import (
	"context"

	"github.com/drand/drand/common/tracer"

	"github.com/drand/drand/protobuf/drand"
)

// PartialBeacon receives a beacon generation request and answers
// with the partial signature from this drand node.
func (dd *DrandDaemon) PartialBeacon(ctx context.Context, in *drand.PartialBeaconPacket) (*drand.Empty, error) {
	ctx, span := tracer.NewSpan(ctx, "dd.PartialBeacon")
	defer span.End()

	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	partialBeacon, err := bp.PartialBeacon(ctx, in)
	span.RecordError(err)
	return partialBeacon, err
}

// PublicRand returns a public random beacon according to the request. If the Round
// field is 0, then it returns the last one generated.
func (dd *DrandDaemon) PublicRand(ctx context.Context, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	ctx, span := tracer.NewSpan(ctx, "dd.DrandDaemon")
	defer span.End()

	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return bp.PublicRand(ctx, in)
}

// PublicRandStream exports a stream of new beacons as they are generated over gRPC
func (dd *DrandDaemon) PublicRandStream(in *drand.PublicRandRequest, stream drand.Public_PublicRandStreamServer) error {
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return err
	}

	return bp.PublicRandStream(in, stream)
}

// Home provides the address the local node is listening
func (dd *DrandDaemon) Home(c context.Context, _ *drand.HomeRequest) (*drand.HomeResponse, error) {
	_, span := tracer.NewSpan(c, "dd.Home")
	defer span.End()

	ctx := drand.NewMetadata(dd.version.ToProto())

	return &drand.HomeResponse{Metadata: ctx}, nil
}

// ChainInfo replies with the chain information this node participates to
func (dd *DrandDaemon) ChainInfo(ctx context.Context, in *drand.ChainInfoRequest) (*drand.ChainInfoPacket, error) {
	ctx, span := tracer.NewSpan(ctx, "dd.ChainInfo")
	defer span.End()

	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return bp.ChainInfo(ctx, in)
}

// SyncChain is an inter-node protocol that replies to a syncing request from a
// given round
func (dd *DrandDaemon) SyncChain(in *drand.SyncRequest, stream drand.Protocol_SyncChainServer) error {
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return err
	}

	return bp.SyncChain(in, stream)
}

// GetIdentity returns the identity of this drand node
func (dd *DrandDaemon) GetIdentity(ctx context.Context, in *drand.IdentityRequest) (*drand.IdentityResponse, error) {
	ctx, span := tracer.NewSpan(ctx, "dd.GetIdentity")
	defer span.End()

	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return bp.GetIdentity(ctx, in)
}
