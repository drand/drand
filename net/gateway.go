package net

import (
	"context"
	"net/http"
	"time"

	"google.golang.org/grpc"

	"github.com/drand/drand/protobuf/drand"
)

// PrivateGateway is the main interface to communicate to other drand nodes. It
// acts as a listener to receive incoming requests and acts a client connecting
// to drand particpants.
// The gateway fixes all drand functionalities offered by drand.
type PrivateGateway struct {
	Listener
	ProtocolClient
	PublicClient
}

// StartAll starts the control and public functionalities of the node
func (g *PrivateGateway) StartAll() {
	go g.Listener.Start()
}

// StopAll stops the control and public functionalities of the node
func (g *PrivateGateway) StopAll(ctx context.Context) {
	g.Listener.Stop(ctx)
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
}

// NewGRPCPrivateGateway returns a grpc gateway listening on "listen" for the
// public methods, listening on "port" for the control methods, using the given
// Service s with the given options.
func NewGRPCPrivateGateway(ctx context.Context,
	listen, certPath, keyPath string,
	certs *CertManager,
	s Service,
	insecure bool,
	opts ...grpc.DialOption) (*PrivateGateway, error) {
	l, err := NewGRPCListenerForPrivate(ctx, listen, certPath, keyPath, s, insecure, grpc.ConnectionTimeout(time.Second))
	if err != nil {
		return nil, err
	}
	pg := &PrivateGateway{
		Listener: l,
	}
	if !insecure {
		pg.ProtocolClient = NewGrpcClientFromCertManager(certs, opts...)
	} else {
		pg.ProtocolClient = NewGrpcClient(opts...)
	}
	// duplication since client implements both...
	// XXX Find a better fix
	pg.PublicClient = pg.ProtocolClient.(*grpcClient)
	return pg, nil
}

// PublicGateway is the main interface to communicate to users.
// The gateway fixes all drand functionalities offered by drand.
type PublicGateway struct {
	Listener
}

// StartAll starts the control and public functionalities of the node
func (g *PublicGateway) StartAll() {
	go g.Listener.Start()
}

// StopAll stops the control and public functionalities of the node
func (g *PublicGateway) StopAll(ctx context.Context) {
	g.Listener.Stop(ctx)
}

// NewRESTPublicGateway returns a grpc gateway listening on "listen" for the
// public methods, listening on "port" for the control methods, using the given
// Service s with the given options.
func NewRESTPublicGateway(
	ctx context.Context,
	listen, certPath, keyPath string,
	certs *CertManager,
	handler http.Handler,
	insecure bool) (*PublicGateway, error) {
	l, err := NewRESTListenerForPublic(ctx, listen, certPath, keyPath, handler, insecure)
	if err != nil {
		return nil, err
	}
	return &PublicGateway{Listener: l}, nil
}
