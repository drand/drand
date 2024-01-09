package net

import (
	"context"

	pdkg "github.com/drand/drand/v2/protobuf/dkg"
	"google.golang.org/grpc"

	"github.com/drand/drand/v2/protobuf/drand"
)

// EmptyServer is an PublicServer + ProtocolServer that does nothing
type EmptyServer struct{}

// GetIdentity returns the identity of the server
func (s *EmptyServer) GetIdentity(_ context.Context, _ *drand.IdentityRequest) (*drand.IdentityResponse, error) {
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

// ChainInfo is an empty implementation
func (s *EmptyServer) ChainInfo(context.Context, *drand.ChainInfoRequest) (*drand.ChainInfoPacket, error) {
	return nil, nil
}

// Home is an empty implementation
func (s *EmptyServer) Home(context.Context, *drand.HomeRequest) (*drand.HomeResponse, error) {
	return nil, nil
}

// BroadcastDKG is an empty implementation
func (s *EmptyServer) BroadcastDKG(context.Context, *pdkg.DKGPacket) (*pdkg.EmptyDKGResponse, error) {
	return nil, nil
}

// SyncChain is an empty implementation
func (s *EmptyServer) SyncChain(*drand.SyncRequest, drand.Protocol_SyncChainServer) error {
	return nil
}

// StartFollowChain is the control method to instruct a drand daemon to sync its chain
func (s *EmptyServer) StartFollowChain(*drand.StartSyncRequest, drand.Control_StartFollowChainServer) error {
	return nil
}

// StartCheckChain is the control method to instruct a drand daemon to check its chain
func (s *EmptyServer) StartCheckChain(*drand.StartSyncRequest, drand.Control_StartCheckChainServer) error {
	return nil
}

// Status method
func (s *EmptyServer) Status(context.Context, *drand.StatusRequest) (*drand.StatusResponse, error) {
	return nil, nil
}

// PartialBeacon is an empty implementation
func (s *EmptyServer) PartialBeacon(context.Context, *drand.PartialBeaconPacket) (*drand.Empty, error) {
	return nil, nil
}

// PingPong is an empty implementation
func (s *EmptyServer) PingPong(context.Context, *drand.Ping) (*drand.Pong, error) {
	return nil, nil
}

// ListSchemes is an empty implementation
func (s *EmptyServer) ListSchemes(context.Context, *drand.ListSchemesRequest) (*drand.ListSchemesResponse, error) {
	return nil, nil
}

// ListBeaconIDs is an empty implementation
func (s *EmptyServer) ListBeaconIDs(context.Context, *drand.ListBeaconIDsRequest) (*drand.ListBeaconIDsResponse, error) {
	return nil, nil
}

// PublicKey is an empty implementation
func (s *EmptyServer) PublicKey(context.Context, *drand.PublicKeyRequest) (*drand.PublicKeyResponse, error) {
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

// LoadBeacon is an empty implementation
func (s *EmptyServer) LoadBeacon(context.Context, *drand.LoadBeaconRequest) (*drand.LoadBeaconResponse, error) {
	return nil, nil
}

// RemoteStatus is an empty implementation
func (s *EmptyServer) RemoteStatus(context.Context, *drand.RemoteStatusRequest) (*drand.RemoteStatusResponse, error) {
	return nil, nil
}

// BackupDatabase is an empty implementation
func (s *EmptyServer) BackupDatabase(context.Context, *drand.BackupDBRequest) (*drand.BackupDBResponse, error) {
	return nil, nil
}

// NodeVersionValidator is an empty implementation
func (s *EmptyServer) NodeVersionValidator(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (response interface{}, err error) {
	return handler(ctx, req)
}

// NodeVersionStreamValidator is an empty implementation
func (s *EmptyServer) NodeVersionStreamValidator(srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	return handler(srv, ss)
}

func (s *EmptyServer) Command(_ context.Context, _ *pdkg.DKGCommand) (*pdkg.EmptyDKGResponse, error) {
	return nil, nil
}

func (s *EmptyServer) Packet(_ context.Context, _ *pdkg.GossipPacket) (*pdkg.EmptyDKGResponse, error) {
	return nil, nil
}

func (s *EmptyServer) DKGStatus(_ context.Context, _ *pdkg.DKGStatusRequest) (*pdkg.DKGStatusResponse, error) {
	return nil, nil
}

func (s *EmptyServer) Migrate(_ context.Context, _ *drand.Empty) (*drand.Empty, error) {
	return nil, nil
}
