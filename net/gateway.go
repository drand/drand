package net

import (
	"context"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/protobuf/drand"
)

var DefaultTimeout = time.Duration(30) * time.Second

// Gateway is the main interface to communicate to the drand world. It
// acts as a listener to receive incoming requests and acts a client connecting
// to drand particpants.
// The gateway fixes all drand functionalities offered by drand.
type Gateway struct {
	Listener
	Client
}

// Service holds all functionalities that a drand node should implement
type Service interface {
	drand.RandomnessServer
	drand.BeaconServer
	dkg.DkgServer
}

// Client represents all methods that are callable on drand nodes
type Client interface {
	Public(p Peer, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error)
	Private(p Peer, in *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error)
	NewBeacon(p Peer, in *drand.BeaconRequest) (*drand.BeaconResponse, error)
	Setup(p Peer, in *dkg.DKGPacket) (*dkg.DKGResponse, error)
}

// Listener is the active listener for incoming requests.
type Listener interface {
	Start()
	Stop()
	Service
}

func NewGrpcGateway(listen string, s Service, opts ...grpc.DialOption) Gateway {
	return Gateway{
		Client:   NewGrpcClient(opts...),
		Listener: NewTCPGrpcListener(listen, s),
	}
}

// grpcClient implements the Client functionalities using gRPC as its underlying
// mechanism
type grpcClient struct {
	sync.Mutex
	conns   map[string]*grpc.ClientConn
	opts    []grpc.DialOption
	timeout time.Duration
}

// NewGrpcClient returns a Client using gRPC connections
func NewGrpcClient(opts ...grpc.DialOption) Client {
	return &grpcClient{
		opts:    opts,
		conns:   make(map[string]*grpc.ClientConn),
		timeout: DefaultTimeout,
	}
}

func NewGrpcClientWithTimeout(timeout time.Duration, opts ...grpc.DialOption) Client {
	c := NewGrpcClient(opts...).(*grpcClient)
	c.timeout = timeout
	return c
}

func (g *grpcClient) Public(p Peer, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewRandomnessClient(c)
	//ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	//defer cancel()
	return client.Public(context.Background(), in, grpc.FailFast(false))
}

func (g *grpcClient) Private(p Peer, in *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewRandomnessClient(c)
	//ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	//defer cancel()
	return client.Private(context.Background(), in, grpc.FailFast(false))

}

func (g *grpcClient) Setup(p Peer, in *dkg.DKGPacket) (*dkg.DKGResponse, error) {
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := dkg.NewDkgClient(c)
	//ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	//defer cancel()
	return client.Setup(context.Background(), in, grpc.FailFast(false))
}

func (g *grpcClient) NewBeacon(p Peer, in *drand.BeaconRequest) (*drand.BeaconResponse, error) {
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewBeaconClient(c)
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()
	return client.NewBeacon(ctx, in, grpc.FailFast(false))
	//return client.NewBeacon(context.Background(), in, grpc.FailFast(true))
}

// conn retrieve an already existing conn to the given peer or create a new one
func (g *grpcClient) conn(p Peer) (*grpc.ClientConn, error) {
	g.Lock()
	defer g.Unlock()
	var err error
	c, ok := g.conns[p.Address()]
	if !ok {
		if !IsTLS(p.Address()) {
			c, err = grpc.Dial(p.Address(), append(g.opts, grpc.WithInsecure())...)
		} else {
			c, err = grpc.Dial(p.Address(), g.opts...)
		}
		g.conns[p.Address()] = c
	}
	return c, err
}

// grpcListener implements Listener using gRPC connections
type grpcListener struct {
	Service
	server *grpc.Server
	lis    net.Listener
}

// NewGrpcListener returns a new Listener from the given network Listener and
// some options that may be necessary to gRPC. The caller should preferable use
// NewTCPGrpcListener or NewTLSgRPCListener.
func NewGrpcListener(l net.Listener, s Service, opts ...grpc.ServerOption) Listener {
	g := &grpcListener{
		Service: s,
		server:  grpc.NewServer(opts...),
		lis:     l,
	}
	drand.RegisterRandomnessServer(g.server, g.Service)
	drand.RegisterBeaconServer(g.server, g.Service)
	dkg.RegisterDkgServer(g.server, g.Service)
	return g
}

// NewTCPGrpcListener returns a gRPC listener using plain TCP connections
// without TLS. The listener will bind to the given address:port
// tuple.
func NewTCPGrpcListener(addr string, s Service) Listener {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		panic("tcp listener: " + err.Error())
	}
	return NewGrpcListener(lis, s)
}

func (g *grpcListener) Start() {
	g.server.Serve(g.lis)
}

func (g *grpcListener) Stop() {
	g.server.GracefulStop()
}
