package net

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/grpc"

	"github.com/drand/drand/protobuf/drand"
)

// Client implements methods to call on the protocol API and the public API of a
// drand node
type Client interface {
	ProtocolClient
	PublicClient
	HTTPClient
	DKGClient
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
}

// PublicClient holds all the methods of the public API . See
// `protobuf/drand/public.proto` for more information.
type PublicClient interface {
	PublicRandStream(ctx context.Context, p Peer, in *drand.PublicRandRequest, opts ...CallOption) (chan *drand.PublicRandResponse, error)
	PublicRand(ctx context.Context, p Peer, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error)
	ChainInfo(ctx context.Context, p Peer, in *drand.ChainInfoRequest) (*drand.ChainInfoPacket, error)
	Home(ctx context.Context, p Peer, in *drand.HomeRequest) (*drand.HomeResponse, error)
}

// HTTPClient is an optional extension to the protocol client relaying of HTTP over the GRPC connection.
// it is currently used for relaying metrics between group members.
type HTTPClient interface {
	HandleHTTP(p Peer) (http.Handler, error)
}

// listenAddrFor parses the address specified into a dialable / listenable address
func listenAddrFor(listenAddr string) (network, addr string) {
	if strings.HasPrefix(listenAddr, "unix://") {
		return "unix", strings.TrimPrefix(listenAddr, "unix://")
	}
	if strings.Contains(listenAddr, ":") {
		return grpcDefaultIPNetwork, listenAddr
	}
	return grpcDefaultIPNetwork, fmt.Sprintf("%s:%s", "localhost", listenAddr)
}

type DKGClient interface {
	Propose(ctx context.Context, p Peer, in *drand.ProposalTerms, opts ...grpc.CallOption) (*drand.EmptyResponse, error)
	Abort(ctx context.Context, p Peer, in *drand.AbortDKG, opts ...grpc.CallOption) (*drand.EmptyResponse, error)
	Execute(ctx context.Context, p Peer, in *drand.StartExecution, opts ...grpc.CallOption) (*drand.EmptyResponse, error)
	Accept(ctx context.Context, p Peer, in *drand.AcceptProposal, opts ...grpc.CallOption) (*drand.EmptyResponse, error)
	Reject(ctx context.Context, p Peer, in *drand.RejectProposal, opts ...grpc.CallOption) (*drand.EmptyResponse, error)
	BroadcastDKG(ctx context.Context, p Peer, in *drand.DKGPacket, opts ...grpc.CallOption) (*drand.EmptyResponse, error)
}
