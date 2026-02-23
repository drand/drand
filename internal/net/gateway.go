package net

import (
	"context"
	"net/http"
	"time"

	"google.golang.org/grpc"

	"github.com/drand/drand/v2/common/log"
	pdkg "github.com/drand/drand/v2/protobuf/dkg"
	"github.com/drand/drand/v2/protobuf/drand"
)

// PrivateGateway is the main interface to communicate to other drand nodes. It
// acts as a listener to receive incoming requests and acts a client connecting
// to drand participants.
// The gateway fixes all drand functionalities offered by drand.
type PrivateGateway struct {
	Listener
	ProtocolClient
	PublicClient
	DKGClient
	MetricsClient
}

// StartAll starts the control and public functionalities of the node
func (g *PrivateGateway) StartAll() {
	go g.Start()
}

// StopAll stops the control and public functionalities of the node
func (g *PrivateGateway) StopAll(ctx context.Context) {
	if s, ok := g.ProtocolClient.(Stoppable); ok {
		s.Stop()
	}
	g.Stop(ctx)
}

// Listener is the active listener for incoming requests.
type Listener interface {
	Start()
	Stop(ctx context.Context)
	Addr() string
}

// Service holds all functionalities that a drand node should implement
type Service interface {
	drand.PublicServer
	drand.ControlServer
	drand.ProtocolServer
	pdkg.DKGPublicServer
	pdkg.DKGControlServer
	drand.Interceptors
	drand.MetricsServer
}

// NewGRPCPrivateGateway returns a grpc gateway listening on "listen" for the
// public methods, listening on "port" for the control methods, using the given
// Service s with the given options.
func NewGRPCPrivateGateway(ctx context.Context, listen string, s Service, opts ...grpc.DialOption) (*PrivateGateway, error) {
	lg := log.FromContextOrDefault(ctx)

	//nolint:mnd // we set the timeout to something smallish but not too small
	l, err := NewGRPCListenerForPrivate(ctx, listen, s, grpc.ConnectionTimeout(7*time.Second))
	if err != nil {
		return nil, err
	}
	pg := &PrivateGateway{Listener: l}

	// we re-use the same client for all protocol-related connections
	client := NewGrpcClient(lg, opts...)
	pg.ProtocolClient = client
	pg.PublicClient = client
	// we create new clients for DKG and metrics to ensure that lock contention or slowdown there won't affect
	// randomness production
	pg.DKGClient = NewGrpcClient(lg.Named("dkg"), opts...)
	pg.MetricsClient = NewGrpcClient(lg.Named("metrics"), opts...)

	return pg, nil
}

// PublicGateway is the main interface to communicate to users.
// The gateway fixes all drand functionalities offered by drand.
type PublicGateway struct {
	Listener
}

// StartAll starts the control and public functionalities of the node
func (g *PublicGateway) StartAll() {
	go g.Start()
}

// StopAll stops the control and public functionalities of the node
func (g *PublicGateway) StopAll(ctx context.Context) {
	g.Stop(ctx)
}

// NewRESTPublicGateway returns a grpc gateway listening on "listen" for the
// public methods, listening on "port" for the control methods, using the given
// Service s with the given options.
func NewRESTPublicGateway(ctx context.Context, listen string, handler http.Handler) (*PublicGateway, error) {
	l, err := NewRESTListenerForPublic(ctx, listen, handler)
	if err != nil {
		return nil, err
	}
	return &PublicGateway{Listener: l}, nil
}
