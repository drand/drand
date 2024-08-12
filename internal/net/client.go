package net

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc"

	pdkg "github.com/drand/drand/v2/protobuf/dkg"

	"github.com/drand/drand/v2/protobuf/drand"
)

// Client implements methods to call on the protocol API and the public API of a
// drand node
type Client interface {
	ProtocolClient
	PublicClient
	DKGClient
	MetricsClient
}

// Stoppable is an interface that some clients can implement to close their
// operations
type Stoppable interface {
	Stop()
}

// CallOption is simply a wrapper around the grpc options
type CallOption = grpc.CallOption

// ProtocolClient holds all the methods of the protocol API that drand protocols
// use. See protobuf/drand/protocol.proto for more information.
type ProtocolClient interface {
	GetIdentity(ctx context.Context, p Peer, in *drand.IdentityRequest, opts ...CallOption) (*drand.IdentityResponse, error)
	SyncChain(ctx context.Context, p Peer, in *drand.SyncRequest, opts ...CallOption) (chan *drand.BeaconPacket, error)
	PartialBeacon(ctx context.Context, p Peer, in *drand.PartialBeaconPacket, opts ...CallOption) error
	Status(context.Context, Peer, *drand.StatusRequest, ...grpc.CallOption) (*drand.StatusResponse, error)
	Check(ctx context.Context, p Peer) error
}

// PublicClient holds all the methods of the public API . See
// `protobuf/drand/public.proto` for more information.
type PublicClient interface {
	PublicRandStream(ctx context.Context, p Peer, in *drand.PublicRandRequest, opts ...CallOption) (chan *drand.PublicRandResponse, error)
	PublicRand(ctx context.Context, p Peer, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error)
	ChainInfo(ctx context.Context, p Peer, in *drand.ChainInfoRequest) (*drand.ChainInfoPacket, error)
	ListBeaconIDs(ctx context.Context, p Peer) (*drand.ListBeaconIDsResponse, error)
}

type MetricsClient interface {
	GetMetrics(ctx context.Context, addr string) (string, error)
}

// listenAddrFor parses the address specified into a dialable / listenable address
func listenAddrFor(listenAddr string) (network, addr string) {
	if strings.HasPrefix(listenAddr, "unix://") {
		return "unix", strings.TrimPrefix(listenAddr, "unix://")
	}
	if strings.Contains(listenAddr, ":") {
		return grpcDefaultIPNetwork, listenAddr
	}
	return grpcDefaultIPNetwork, fmt.Sprintf("%s:%s", "127.0.0.1", listenAddr)
}

type DKGClient interface {
	Packet(ctx context.Context, p Peer, packet *pdkg.GossipPacket, opts ...grpc.CallOption) (*pdkg.EmptyDKGResponse, error)
	BroadcastDKG(ctx context.Context, p Peer, in *pdkg.DKGPacket, opts ...grpc.CallOption) (*pdkg.EmptyDKGResponse, error)
}
