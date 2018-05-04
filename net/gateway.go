package net

import (
	"context"
	"net"
	"sync"

	"google.golang.org/grpc"

	"github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/protobuf/drand"
)

// ADDRESSES and TLS
// https://github.com/denji/golang-tls
// How do we manage plain tcp and TLS connection with self-signed certificates
// or CA-signed certificates:
// (A) For non tls servers, when initiating connection just call with
// grpc.WithInsecure(). Same for listening ( see Golang gRPC API).
// (B) TLS communication using certificates
// How to differentiate (A) and (B) ?
// 	=> simple set of rules ? (xxx:443 | https | tls) == (B), rest is (A)
//
// For (B):
// Certificates is signed by a CA, so no options needed, simply
// 		crendentials.FromTLSCOnfig(&tls.Config{}) when connecting, or
// 		credentials.FromTLSConfig{&tls.Config{cert,private...}} for listening
// Certificates are given as a command line option "-cert xxx.crt" == (2) , otherwise (1)
// Since gRPC golang library does not allow us to access internal connections,
// every pair of communicating nodes is gonna have two active connections at the
// same time, one outgoing from each party.

// Peer is a simple interface that allows retrieving the address of a
// destination. It might further e enhanced with certificates properties and
// all.
type Peer interface {
	Address() string
	TLS() bool
}

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
	NewBeacon(p Peer, in *drand.BeaconPacket) (*drand.BeaconResponse, error)
	Setup(p Peer, in *dkg.DKGPacket) (*dkg.DKGResponse, error)
}

// Listener is the active listener for incoming requests.
type Listener interface {
	Start()
	Stop()
	// RegisterDrandService stores the given Service implementation and will
	// dispatch any incoming calls to this Service.
	RegisterDrandService(Service)
}

// grpcClient implements the Client functionalities using gRPC as its underlying
// mechanism
type grpcClient struct {
	sync.Mutex
	conns map[string]*grpc.ClientConn
}

// NewGrpcClient returns a Client using gRPC connections
func NewGrpcClient(opts ...grpc.DialOption) Client {
	return &grpcClient{
		conns: make(map[string]*grpc.ClientConn),
	}
}

func (g *grpcClient) Public(p Peer, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewRandomnessClient(c)
	return client.Public(context.Background(), in)
}

func (g *grpcClient) Setup(p Peer, in *dkg.DKGPacket) (*dkg.DKGResponse, error) {
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := dkg.NewDkgClient(c)
	return client.Setup(context.Background(), in)
}

func (g *grpcClient) NewBeacon(p Peer, in *drand.BeaconPacket) (*drand.BeaconResponse, error) {
	c, err := g.conn(p)
	if err != nil {
		return nil, err
	}
	client := drand.NewBeaconClient(c)
	return client.NewBeacon(context.Background(), in)
}

// conn retrieve an already existing conn to the given peer or create a new one
func (g *grpcClient) conn(p Peer) (*grpc.ClientConn, error) {
	g.Lock()
	defer g.Unlock()
	var err error
	c, ok := g.conns[p.Address()]
	if !ok {
		if !p.TLS() {
			c, err = grpc.Dial(p.Address(), grpc.WithInsecure())
			g.conns[p.Address()] = c
		} else {
			// TODO implement pool self signed certificates
		}
	}
	return c, err
}

// grpcListener implements Listener using gRPC connections
type grpcListener struct {
	service Service
	server  *grpc.Server
	lis     net.Listener
}

// NewGrpcListener returns a new Listener from the given network Listener and
// some options that may be necessary to gRPC. The caller should preferable use
// NewTCPGrpcListener or NewTLSgRPCListener.
func NewGrpcListener(l net.Listener, opts ...grpc.ServerOption) Listener {
	return &grpcListener{
		server: grpc.NewServer(opts...),
		lis:    l,
	}
}

// NewTCPGrpcListener returns a gRPC listener using plain TCP connections
// without TLS. The listener will bind to the given address:port
// tuple.
func NewTCPGrpcListener(addr string) Listener {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		panic("tcp listener: " + err.Error())
	}
	return NewGrpcListener(lis)
}

// RegisterDrandService implements the Listener interface.
func (g *grpcListener) RegisterDrandService(s Service) {
	g.service = s
	drand.RegisterRandomnessServer(g.server, g.service)
	drand.RegisterBeaconServer(g.server, g.service)
	dkg.RegisterDkgServer(g.server, g.service)
}

func (g *grpcListener) Start() {
	g.server.Serve(g.lis)
}

func (g *grpcListener) Stop() {
	g.server.GracefulStop()
}
