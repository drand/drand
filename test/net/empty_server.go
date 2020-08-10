package net

import (
	"context"

	"github.com/drand/drand/protobuf/drand"
)

// EmptyServer is an PublicServer + ProtocolServer that does nothing
type EmptyServer struct{}

// GetIdentity returns the identity of the server
func (s *EmptyServer) GetIdentity(ctx context.Context, in *drand.IdentityRequest) (*drand.Identity, error) {
	return nil, nil
}

// PublicRandStream is an empty implementation
func (s *EmptyServer) PublicRandStream(*drand.PublicRandRequest, drand.Public_PublicRandStreamServer) error {
	return nil
}

// PublicRand is an empty implementation
func (s *EmptyServer) PublicRand(context.Context, *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	return nil, nil
}

// PrivateRand is an empty implementation
func (s *EmptyServer) PrivateRand(context.Context, *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	return nil, nil
}

// ChainInfo is an empty implementation
func (s *EmptyServer) ChainInfo(context.Context, *drand.ChainInfoRequest) (*drand.ChainInfoPacket, error) {
	return nil, nil
}

// Home is an empty implementation
func (s *EmptyServer) Home(context.Context, *drand.HomeRequest) (*drand.HomeResponse, error) {
	return nil, nil
}

// SignalDKGParticipant is an empty implementation
func (s *EmptyServer) SignalDKGParticipant(context.Context, *drand.SignalDKGPacket) (*drand.Empty, error) {
	return nil, nil
}

// PushDKGInfo is an empty implementation
func (s *EmptyServer) PushDKGInfo(context.Context, *drand.DKGInfoPacket) (*drand.Empty, error) {
	return nil, nil
}

// BroadcastDKG is an empty implementation
func (s *EmptyServer) BroadcastDKG(context.Context, *drand.DKGPacket) (*drand.Empty, error) {
	return nil, nil
}

// SyncChain is an empty implementation
func (s *EmptyServer) SyncChain(*drand.SyncRequest, drand.Protocol_SyncChainServer) error {
	return nil
}

// StartFollowChain is the control method to instruct a drand daemon to follow
// its chain
func (s *EmptyServer) StartFollowChain(*drand.StartFollowRequest, drand.Control_StartFollowChainServer) error {
	return nil
}

// PartialBeacon is an empty implementation
func (s *EmptyServer) PartialBeacon(context.Context, *drand.PartialBeaconPacket) (*drand.Empty, error) {
	return nil, nil
}

// PingPong is an empty implementation
func (s *EmptyServer) PingPong(context.Context, *drand.Ping) (*drand.Pong, error) {
	return nil, nil
}

// InitDKG is an empty implementation
func (s *EmptyServer) InitDKG(context.Context, *drand.InitDKGPacket) (*drand.GroupPacket, error) {
	return nil, nil
}

// InitReshare is an empty implementation
func (s *EmptyServer) InitReshare(context.Context, *drand.InitResharePacket) (*drand.GroupPacket, error) {
	return nil, nil
}

// Share is an empty implementation
func (s *EmptyServer) Share(context.Context, *drand.ShareRequest) (*drand.ShareResponse, error) {
	return nil, nil
}

// PublicKey is an empty implementation
func (s *EmptyServer) PublicKey(context.Context, *drand.PublicKeyRequest) (*drand.PublicKeyResponse, error) {
	return nil, nil
}

// PrivateKey is an empty implementation
func (s *EmptyServer) PrivateKey(context.Context, *drand.PrivateKeyRequest) (*drand.PrivateKeyResponse, error) {
	return nil, nil
}

// CollectiveKey is an empty implementation
func (s *EmptyServer) CollectiveKey(context.Context, *drand.CokeyRequest) (*drand.CokeyResponse, error) {
	return nil, nil
}

// GroupFile is an empty implementation
func (s *EmptyServer) GroupFile(context.Context, *drand.GroupRequest) (*drand.GroupPacket, error) {
	return nil, nil
}

// Shutdown is an empty implementation
func (s *EmptyServer) Shutdown(context.Context, *drand.ShutdownRequest) (*drand.ShutdownResponse, error) {
	return nil, nil
}
