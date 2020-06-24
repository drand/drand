package net

import (
	"context"

	"github.com/drand/drand/protobuf/drand"
)

var _ Service = (*EmptyServer)(nil)

// EmptyServer is an PublicServer + ProtocolServer that does nothing
type EmptyServer struct{}

// GetIdentity returns the identity of the server
func (s *EmptyServer) GetIdentity(ctx context.Context, in *drand.IdentityRequest) (*drand.Identity, error) {
	return nil, nil
}

// PublicRandStream ...
func (s *EmptyServer) PublicRandStream(*drand.PublicRandRequest, drand.Public_PublicRandStreamServer) error {
	return nil
}

// PublicRand ...
func (s *EmptyServer) PublicRand(context.Context, *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	return nil, nil
}

// PrivateRand ...
func (s *EmptyServer) PrivateRand(context.Context, *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	return nil, nil
}

// ChainInfo ...
func (s *EmptyServer) ChainInfo(context.Context, *drand.ChainInfoRequest) (*drand.ChainInfoPacket, error) {
	return nil, nil
}

// Home ...
func (s *EmptyServer) Home(context.Context, *drand.HomeRequest) (*drand.HomeResponse, error) {
	return nil, nil
}

// SignalDKGParticipant ...
func (s *EmptyServer) SignalDKGParticipant(context.Context, *drand.SignalDKGPacket) (*drand.Empty, error) {
	return nil, nil
}

// PushDKGInfo ...
func (s *EmptyServer) PushDKGInfo(context.Context, *drand.DKGInfoPacket) (*drand.Empty, error) {
	return nil, nil
}

// FreshDKG ...
func (s *EmptyServer) FreshDKG(context.Context, *drand.DKGPacket) (*drand.Empty, error) {
	return nil, nil
}

// SyncChain ...
func (s *EmptyServer) SyncChain(*drand.SyncRequest, drand.Protocol_SyncChainServer) error {
	return nil
}

// ReshareDKG ...
func (s *EmptyServer) ReshareDKG(context.Context, *drand.ResharePacket) (*drand.Empty, error) {
	return nil, nil
}

// PartialBeacon ...
func (s *EmptyServer) PartialBeacon(context.Context, *drand.PartialBeaconPacket) (*drand.Empty, error) {
	return nil, nil
}

// PingPong ...
func (s *EmptyServer) PingPong(context.Context, *drand.Ping) (*drand.Pong, error) {
	return nil, nil
}

// InitDKG ...
func (s *EmptyServer) InitDKG(context.Context, *drand.InitDKGPacket) (*drand.GroupPacket, error) {
	return nil, nil
}

// InitReshare ...
func (s *EmptyServer) InitReshare(context.Context, *drand.InitResharePacket) (*drand.GroupPacket, error) {
	return nil, nil
}

// Share ...
func (s *EmptyServer) Share(context.Context, *drand.ShareRequest) (*drand.ShareResponse, error) {
	return nil, nil
}

// PublicKey ...
func (s *EmptyServer) PublicKey(context.Context, *drand.PublicKeyRequest) (*drand.PublicKeyResponse, error) {
	return nil, nil
}

// PrivateKey ...
func (s *EmptyServer) PrivateKey(context.Context, *drand.PrivateKeyRequest) (*drand.PrivateKeyResponse, error) {
	return nil, nil
}

// CollectiveKey ...
func (s *EmptyServer) CollectiveKey(context.Context, *drand.CokeyRequest) (*drand.CokeyResponse, error) {
	return nil, nil
}

// GroupFile ...
func (s *EmptyServer) GroupFile(context.Context, *drand.GroupRequest) (*drand.GroupPacket, error) {
	return nil, nil
}

// Shutdown ...
func (s *EmptyServer) Shutdown(context.Context, *drand.ShutdownRequest) (*drand.ShutdownResponse, error) {
	return nil, nil
}
