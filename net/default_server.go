package net

import (
	"context"

	"github.com/dedis/drand/protobuf/drand"
)

var _ Service = (*EmptyServer)(nil)

// EmptyServer is an PublicServer + ProtocolServer that does nothing
type EmptyServer struct{}

// PublicRand ...
func (s *EmptyServer) PublicRand(context.Context, *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	return nil, nil
}

// PrivateRand ...
func (s *EmptyServer) PrivateRand(context.Context, *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	return nil, nil
}

// Group ...
func (s *EmptyServer) Group(context.Context, *drand.GroupRequest) (*drand.GroupResponse, error) {
	return nil, nil
}

// DistKey ...
func (s *EmptyServer) DistKey(context.Context, *drand.DistKeyRequest) (*drand.DistKeyResponse, error) {
	return nil, nil
}

// Home ...
func (s *EmptyServer) Home(context.Context, *drand.HomeRequest) (*drand.HomeResponse, error) {
	return nil, nil
}

// Setup ...
func (s *EmptyServer) Setup(context.Context, *drand.SetupPacket) (*drand.Empty, error) {
	return nil, nil
}

// Reshare ...
func (s *EmptyServer) Reshare(context.Context, *drand.ResharePacket) (*drand.Empty, error) {
	return nil, nil
}

// NewBeacon ...
func (s *EmptyServer) NewBeacon(context.Context, *drand.BeaconRequest) (*drand.BeaconResponse, error) {
	return nil, nil
}

// PingPong ...
func (s *EmptyServer) PingPong(context.Context, *drand.Ping) (*drand.Pong, error) {
	return nil, nil
}

// InitDKG ...
func (s *EmptyServer) InitDKG(context.Context, *drand.InitDKGPacket) (*drand.Empty, error) {
	return nil, nil
}

// InitReshare ...
func (s *EmptyServer) InitReshare(context.Context, *drand.InitResharePacket) (*drand.Empty, error) {
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
func (s *EmptyServer) GroupFile(context.Context, *drand.GroupTOMLRequest) (*drand.GroupTOMLResponse, error) {
	return nil, nil
}
