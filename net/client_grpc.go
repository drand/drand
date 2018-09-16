package net

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dedis/drand/protobuf/control"
	"github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/protobuf/drand"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/nikkolasg/slog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Service holds all functionalities that a drand node should implement
type Service interface {
	drand.RandomnessServer
	drand.InfoServer
	drand.BeaconServer
	dkg.DkgServer
}

var defaultJSONMarshaller = &runtime.JSONBuiltin{}

// grpcClient implements both InternalClient and ExternalClient functionalities
// using gRPC as its underlying mechanism
type grpcClient struct {
	sync.Mutex
	conns   map[string]*grpc.ClientConn
	opts    []grpc.DialOption
	timeout time.Duration
	manager *CertManager
}

// NewGrpcClient returns an implementation of an InternalClient  and
// ExternalClient using gRPC connections
func NewGrpcClient(opts ...grpc.DialOption) *grpcClient {
	return &grpcClient{
		opts:    opts,
		conns:   make(map[string]*grpc.ClientConn),
		timeout: DefaultTimeout,
		manager: NewCertManager(),
	}
}

func NewGrpcClientFromCertManager(c *CertManager, opts ...grpc.DialOption) *grpcClient {
	client := NewGrpcClient(opts...)
	client.manager = c
	return client
}

func NewGrpcClientWithTimeout(timeout time.Duration, opts ...grpc.DialOption) *grpcClient {
	c := NewGrpcClient(opts...)
	c.timeout = timeout
	return c
}

func (g *grpcClient) SetTimeout(t time.Duration) {
	g.timeout = t
}

func (g *grpcClient) Public(p Peer, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewRandomnessClient(c)
	//ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	//defer cancel()
	r, err := client.Public(context.Background(), in)
	return r, err
}

func (g *grpcClient) Private(p Peer, in *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewRandomnessClient(c)
	//ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	//defer cancel()
	return client.Private(context.Background(), in)

}

func (g *grpcClient) DistKey(p Peer, in *drand.DistKeyRequest) (*drand.DistKeyResponse, error) {
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewInfoClient(c)
	return client.DistKey(context.Background(), in)
}

func (g *grpcClient) Setup(p Peer, in *dkg.DKGPacket, opts ...CallOption) (*dkg.DKGResponse, error) {
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := dkg.NewDkgClient(c)
	return client.Setup(context.Background(), in, opts...)
}

func (g *grpcClient) Reshare(p Peer, in *dkg.DKGPacket, opts ...CallOption) (*dkg.DKGResponse, error) {
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := dkg.NewDkgClient(c)
	return client.Reshare(context.Background(), in, opts...)
}

func (g *grpcClient) NewBeacon(p Peer, in *drand.BeaconRequest, opts ...CallOption) (*drand.BeaconResponse, error) {
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
		slog.Debugf("grpc-client: attempting connection to %s (TLS %v)", p.Address(), p.IsTLS())
		if !p.IsTLS() {
			c, err = grpc.Dial(p.Address(), append(g.opts, grpc.WithInsecure())...)
		} else {
			pool := g.manager.Pool()
			creds := credentials.NewClientTLSFromCert(pool, "")
			opts := append(g.opts, grpc.WithTransportCredentials(creds))
			c, err = grpc.Dial(p.Address(), opts...)
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
func (p *proxyClient) DistKey(c context.Context, in *drand.DistKeyRequest, opts ...grpc.CallOption) (*drand.DistKeyResponse, error) {
	return p.s.DistKey(c, in)
}

//ControlClient is a struct that implement control.ControlClient and is used to request
//a Share to a ControlListener on a specific port
type ControlClient struct {
	conn   *grpc.ClientConn
	client control.ControlClient
}

// NewControlClient creates a client connection to the given target (localhost:8888)
func NewControlClient(port string) ControlClient {
	var conn *grpc.ClientConn
	conn, err := grpc.Dial(fmt.Sprintf("%s:%s", "localhost", port), grpc.WithInsecure())
	if err != nil {
		slog.Fatalf("control: did not connect: %s", err)
		return ControlClient{}
	}
	c := control.NewControlClient(conn)
	return ControlClient{conn: conn, client: c}
}

func (c ControlClient) Share() (*control.ShareResponse, error) {
	return c.client.Share(context.Background(), &control.ShareRequest{})
}
