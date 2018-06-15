package net

import (
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
	InternalClient
}

type ExternalClient interface {
	Public(p Peer, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error)
	Private(p Peer, in *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error)
}

// InternalClient represents all methods callable on drand nodes which are
// internal to the system. See the folder api/ to get more info on the external
// API drand nodes offer.
type InternalClient interface {
	NewBeacon(p Peer, in *drand.BeaconRequest) (*drand.BeaconResponse, error)
	Setup(p Peer, in *dkg.DKGPacket) (*dkg.DKGResponse, error)
}

// Listener is the active listener for incoming requests.
type Listener interface {
	Service
	Start()
	Stop()
}

func NewGrpcGateway(listen string, s Service, opts ...grpc.DialOption) Gateway {
	return Gateway{
		InternalClient: NewGrpcClient(opts...),
		Listener:       NewTCPGrpcListener(listen, s),
	}
}
