package net

import (
	"time"

	"github.com/dedis/drand/protobuf/drand"
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
	NewBeacon(p Peer, in *drand.BeaconRequest, opts ...CallOption) (*drand.BeaconResponse, error)
	Setup(p Peer, in *drand.SetupPacket, opts ...CallOption) (*drand.Empty, error)
	Reshare(p Peer, in *drand.ResharePacket, opts ...CallOption) (*drand.Empty, error)
	SetTimeout(time.Duration)
}

// PublicClient holds all the methods of the public API . See
// `protobuf/drand/public.proto` for more information.
type PublicClient interface {
	PublicRand(p Peer, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error)
	PrivateRand(p Peer, in *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error)
	DistKey(p Peer, in *drand.DistKeyRequest) (*drand.DistKeyResponse, error)
	Group(p Peer, in *drand.GroupRequest) (*drand.GroupResponse, error)
	Home(p Peer, in *drand.HomeRequest) (*drand.HomeResponse, error)
}
