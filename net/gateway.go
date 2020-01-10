package net

import (
	"time"

	"google.golang.org/grpc"

	//"github.com/drand/drand/protobuf/control"

	"github.com/drand/drand/protobuf/drand"
)

//var DefaultTimeout = time.Duration(30) * time.Second

// Gateway is the main interface to communicate to other drand nodes. It
// acts as a listener to receive incoming requests and acts a client connecting
// to drand particpants.
// The gateway fixes all drand functionalities offered by drand.
type Gateway struct {
	Listener
	ProtocolClient
}

// CallOption is simply a wrapper around the grpc options
type CallOption = grpc.CallOption

// Listener is the active listener for incoming requests.
type Listener interface {
	Service
	Start()
	Stop()
}

// Service holds all functionalities that a drand node should implement
type Service interface {
	drand.PublicServer
	drand.ControlServer
	drand.ProtocolServer
}

// NewGrpcGatewayInsecure returns a grpc Gateway listening on "listen" for the
// public methods, listening on "port" for the control methods, using the given
// Service s with the given options.
func NewGrpcGatewayInsecure(listen string, s Service, opts ...grpc.DialOption) Gateway {
	return Gateway{
		ProtocolClient: NewGrpcClient(opts...),
		Listener:       NewTCPGrpcListener(listen, s),
	}
}

// NewGrpcGatewayFromCertManager returns a grpc gateway using the TLS
// certificate manager
func NewGrpcGatewayFromCertManager(listen string, certPath, keyPath string, certs *CertManager, s Service, opts ...grpc.DialOption) Gateway {
	l, err := NewTLSGrpcListener(listen, certPath, keyPath, s, grpc.ConnectionTimeout(500*time.Millisecond))
	if err != nil {
		panic(err)
	}
	return Gateway{
		ProtocolClient: NewGrpcClientFromCertManager(certs, opts...),
		Listener:       l,
	}
}

// StartAll starts the control and public functionalities of the node
func (g Gateway) StartAll() {
	go g.Listener.Start()
}

// StopAll stops the control and public functionalities of the node
func (g Gateway) StopAll() {
	g.Listener.Stop()
}
