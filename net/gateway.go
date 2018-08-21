package net

import (
	"time"

	"google.golang.org/grpc"

	"github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/protobuf/drand"
	"github.com/dedis/drand/protobuf/control"
)

var DefaultTimeout = time.Duration(30) * time.Second

// Gateway is the main interface to communicate to the drand world. It
// acts as a listener to receive incoming requests and acts a client connecting
// to drand particpants.
// The gateway fixes all drand functionalities offered by drand.
type Gateway struct {
	Listener
	InternalClient
	ControlListener
}

type ExternalClient interface {
	Public(p Peer, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error)
	Private(p Peer, in *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error)
	DistKey(p Peer, in *drand.DistKeyRequest) (*drand.DistKeyResponse, error)
}

type CallOption = grpc.CallOption

// InternalClient represents all methods callable on drand nodes which are
// internal to the system. See the folder api/ to get more info on the external
// API drand nodes offer.
type InternalClient interface {
	NewBeacon(p Peer, in *drand.BeaconRequest, opts ...CallOption) (*drand.BeaconResponse, error)
	Setup(p Peer, in *dkg.DKGPacket, opts ...CallOption) (*dkg.DKGResponse, error)
}

// Listener is the active listener for incoming requests.
type Listener interface {
	Service
	Start()
	Stop()
}

func NewGrpcGatewayInsecure(listen string, s Service, cs control.ControlServer, opts ...grpc.DialOption) Gateway {
	return Gateway{
		InternalClient: NewGrpcClient(opts...),
		Listener:       NewTCPGrpcListener(listen, s),
		ControlListener: NewTCPGrpcControlListener(cs),
	}
}

func NewGrpcGatewayFromCertManager(listen string, certPath, keyPath string, certs *CertManager, s Service, cs control.ControlServer, opts ...grpc.DialOption) Gateway {
	l, err := NewTLSGrpcListener(listen, certPath, keyPath, s)
	if err != nil {
		panic(err)
	}
	return Gateway{
		InternalClient: NewGrpcClientFromCertManager(certs, opts...),
		Listener:       l,
		ControlListener: NewTCPGrpcControlListener(cs),
	}
}

func (g Gateway) StartAll() {
	go g.ControlListener.Start()
	go g.Listener.Start()
}

func (g Gateway) StopAll() {
	g.Listener.Stop()
	g.ControlListener.Stop()
}
