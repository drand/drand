package core

import (
	"context"

	"github.com/drand/drand/protobuf/common"

	"github.com/drand/drand/protobuf/drand"
)

// BroadcastDKG is the public method to call during a DKG protocol.
func (dd *DrandDaemon) BroadcastDKG(c context.Context, in *drand.DKGPacket) (*drand.Empty, error) {
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.BroadcastDKG(c, in)
}

// PartialBeacon receives a beacon generation request and answers
// with the partial signature from this drand node.
func (dd *DrandDaemon) PartialBeacon(c context.Context, in *drand.PartialBeaconPacket) (*drand.Empty, error) {
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.PartialBeacon(c, in)
}

// PublicRand returns a public random beacon according to the request. If the Round
// field is 0, then it returns the last one generated.
func (dd *DrandDaemon) PublicRand(c context.Context, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.PublicRand(c, in)
}

// PublicRandStream exports a stream of new beacons as they are generated over gRPC
func (dd *DrandDaemon) PublicRandStream(in *drand.PublicRandRequest, stream drand.Public_PublicRandStreamServer) error {
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return err
	}

	return bp.PublicRandStream(in, stream)
}

// PrivateRand returns an ECIES encrypted random blob of 32 bytes from /dev/urandom
func (dd *DrandDaemon) PrivateRand(c context.Context, in *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.PrivateRand(c, in)
}

// Home provides the address the local node is listening
func (dd *DrandDaemon) Home(c context.Context, in *drand.HomeRequest) (*drand.HomeResponse, error) {
	ctx := common.NewMetadata(dd.version.ToProto())

	return &drand.HomeResponse{Metadata: ctx}, nil
}

// ChainInfo replies with the chain information this node participates to
func (dd *DrandDaemon) ChainInfo(ctx context.Context, in *drand.ChainInfoRequest) (*drand.ChainInfoPacket, error) {
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.ChainInfo(ctx, in)
}

// SignalDKGParticipant receives a dkg signal packet from another member
func (dd *DrandDaemon) SignalDKGParticipant(ctx context.Context, in *drand.SignalDKGPacket) (*drand.Empty, error) {
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.SignalDKGParticipant(ctx, in)
}

// PushDKGInfo triggers sending DKG info to other members
func (dd *DrandDaemon) PushDKGInfo(ctx context.Context, in *drand.DKGInfoPacket) (*drand.Empty, error) {
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.PushDKGInfo(ctx, in)
}

// SyncChain is a inter-node protocol that replies to a syncing request from a
// given round
func (dd *DrandDaemon) SyncChain(in *drand.SyncRequest, stream drand.Protocol_SyncChainServer) error {
	metadata := in.GetMetadata()
	bp, err := dd.getBeaconProcessFromRequest(metadata)
	if err != nil {
		return err
	}

	return bp.SyncChain(in, stream)
}

// GetIdentity returns the identity of this drand node
func (dd *DrandDaemon) GetIdentity(ctx context.Context, in *drand.IdentityRequest) (*drand.IdentityResponse, error) {
	bp, err := dd.getBeaconProcessFromRequest(in.GetMetadata())
	if err != nil {
		return nil, err
	}

	return bp.GetIdentity(ctx, in)
}
