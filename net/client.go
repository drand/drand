package net

import (
	"context"
	"net/http"

	"github.com/drand/drand/protobuf/drand"
	"google.golang.org/grpc"
)

// Client implements methods to call on the protocol API and the public API of a
// drand node
type Client interface {
	ProtocolClient
	PublicClient
	HTTPClient
}

// CallOption is simply a wrapper around the grpc options
type CallOption = grpc.CallOption

// ProtocolClient holds all the methods of the protocol API that drand protocols
// use. See protobuf/drand/protocol.proto for more information.
type ProtocolClient interface {
	GetIdentity(ctx context.Context, p Peer, in *drand.IdentityRequest, opts ...CallOption) (*drand.Identity, error)
	SyncChain(ctx context.Context, p Peer, in *drand.SyncRequest, opts ...CallOption) (chan *drand.BeaconPacket, error)
	PartialBeacon(ctx context.Context, p Peer, in *drand.PartialBeaconPacket, opts ...CallOption) error
	BroadcastDKG(c context.Context, p Peer, in *drand.DKGPacket, opts ...CallOption) error
	SignalDKGParticipant(ctx context.Context, p Peer, in *drand.SignalDKGPacket, opts ...CallOption) error
	PushDKGInfo(ctx context.Context, p Peer, in *drand.DKGInfoPacket, opts ...grpc.CallOption) error
}

// PublicClient holds all the methods of the public API . See
// `protobuf/drand/public.proto` for more information.
type PublicClient interface {
	PublicRandStream(ctx context.Context, p Peer, in *drand.PublicRandRequest, opts ...CallOption) (chan *drand.PublicRandResponse, error)
	PublicRand(ctx context.Context, p Peer, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error)
	PrivateRand(ctx context.Context, p Peer, in *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error)
	ChainInfo(ctx context.Context, p Peer, in *drand.ChainInfoRequest) (*drand.ChainInfoPacket, error)
	Home(ctx context.Context, p Peer, in *drand.HomeRequest) (*drand.HomeResponse, error)
}

// HTTPClient is an optional extension to the protocol client relaying of HTTP over the GRPC connection.
// it is currently used for relaying metrics between group members.
type HTTPClient interface {
	HandleHTTP(p Peer) (http.Handler, error)
}
