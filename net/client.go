package net

import (
	"context"
	"time"

	"github.com/drand/drand/protobuf/drand"
)

// Client implements methods to call on the protocol API and the public API of a
// drand node
type Client interface {
	ProtocolClient
	PublicClient
}

// ProtocolClient holds all the methods of the protocol API that drand protocols
// use. See protobuf/drand/protocol.proto for more information.
type ProtocolClient interface {
	SyncChain(ctx context.Context, p Peer, in *drand.SyncRequest, opts ...CallOption) (chan *drand.SyncResponse, error)
	NewBeacon(p Peer, in *drand.BeaconPacket, opts ...CallOption) (*drand.Empty, error)
	Setup(ctx context.Context, p Peer, in *drand.SetupPacket, opts ...CallOption) (*drand.Empty, error)
	Reshare(ctx context.Context, p Peer, in *drand.ResharePacket, opts ...CallOption) (*drand.Empty, error)
	SetTimeout(time.Duration)
}

// PublicClient holds all the methods of the public API . See
// `protobuf/drand/public.proto` for more information.
type PublicClient interface {
	PublicRandStream(ctx context.Context, p Peer, in *drand.PublicRandRequest, opts ...CallOption) (chan *drand.PublicRandResponse, error)
	PublicRand(ctx context.Context, p Peer, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error)
	PrivateRand(ctx context.Context, p Peer, in *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error)
	DistKey(ctx context.Context, p Peer, in *drand.DistKeyRequest) (*drand.DistKeyResponse, error)
	Group(ctx context.Context, p Peer, in *drand.GroupRequest) (*drand.GroupResponse, error)
	Home(ctx context.Context, p Peer, in *drand.HomeRequest) (*drand.HomeResponse, error)
}
