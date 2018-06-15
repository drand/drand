package net

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/protobuf/drand"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/soheilhy/cmux"
	"google.golang.org/grpc"
)

// Service holds all functionalities that a drand node should implement
type Service interface {
	drand.RandomnessServer
	drand.BeaconServer
	dkg.DkgServer
}

var defaultJSONMarshaller = &runtime.JSONPb{}

// grpcListener implements Listener using gRPC connections and regular HTTP
// connections for the JSON REST API.
// NOTE: This use cmux under the hood to be able to use non-tls connection. The
// reason of this relatively high costs (multiple routines etc) is described in
// the issue https://github.com/grpc/grpc-go/issues/555.
type grpcListener struct {
	Service
	grpcServer *grpc.Server
	restServer *http.Server
	mux        cmux.CMux
	lis        net.Listener
}

// NewGrpcListener returns a new Listener from the given network Listener and
// some options that may be necessary to gRPC. The caller should preferable use
// NewTCPGrpcListener or NewTLSgRPCListener.
func NewGrpcListener(l net.Listener, s Service, opts ...grpc.ServerOption) Listener {

	mux := cmux.New(l)

	// grpc API
	grpcServer := grpc.NewServer(opts...)

	// REST api
	gwMux := runtime.NewServeMux(runtime.WithMarshalerOption("application/json", defaultJSONMarshaller))
	proxyClient := newProxyClient(s)
	ctx := context.TODO()
	if err := drand.RegisterRandomnessHandlerClient(ctx, gwMux, proxyClient); err != nil {
		panic(err)
	}
	restRouter := http.NewServeMux()
	restRouter.Handle("/", gwMux)
	restServer := &http.Server{
		Handler: restRouter,
	}

	g := &grpcListener{
		Service:    s,
		grpcServer: grpcServer,
		restServer: restServer,
		mux:        mux,
		lis:        l,
	}
	drand.RegisterRandomnessServer(g.grpcServer, g.Service)
	drand.RegisterBeaconServer(g.grpcServer, g.Service)
	dkg.RegisterDkgServer(g.grpcServer, g.Service)
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
	grpcL := g.mux.Match(cmux.HTTP2HeaderField("content-type", "application/grpc"))
	restL := g.mux.Match(cmux.Any())

	go g.grpcServer.Serve(grpcL)
	go g.restServer.Serve(restL)
	g.mux.Serve()
}

func (g *grpcListener) Stop() {
	g.grpcServer.GracefulStop()
	g.restServer.Shutdown(context.Background())
	g.lis.Close()
}

// grpcClient implements both InternalClient and ExternalClient functionalities
// using gRPC as its underlying mechanism
type grpcClient struct {
	sync.Mutex
	conns   map[string]*grpc.ClientConn
	opts    []grpc.DialOption
	timeout time.Duration
}

// NewGrpcClient returns an implementation of an InternalClient  and
// ExternalClient using gRPC connections
func NewGrpcClient(opts ...grpc.DialOption) *grpcClient {
	return &grpcClient{
		opts:    opts,
		conns:   make(map[string]*grpc.ClientConn),
		timeout: DefaultTimeout,
	}
}

func NewGrpcClientWithTimeout(timeout time.Duration, opts ...grpc.DialOption) *grpcClient {
	c := NewGrpcClient(opts...)
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
	//ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	//defer cancel()
	//return client.NewBeacon(ctx, in, grpc.FailFast(false))
	return client.NewBeacon(context.Background(), in, grpc.FailFast(true))
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

// proxyClient is used by the gRPC json gateway to dispatch calls to the
// underlying gRPC server. It needs only to implement the public facing API
type proxyClient struct {
	s Service
}

func newProxyClient(s Service) *proxyClient {
	return &proxyClient{s}
}

func (p *proxyClient) Public(c context.Context, in *drand.PublicRandRequest, opts ...grpc.CallOption) (*drand.PublicRandResponse, error) {
	return p.s.Public(c, in)
}
func (p *proxyClient) Private(c context.Context, in *drand.PrivateRandRequest, opts ...grpc.CallOption) (*drand.PrivateRandResponse, error) {
	return p.s.Private(c, in)
}
