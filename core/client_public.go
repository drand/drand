package core

import (
	"context"

	"google.golang.org/grpc"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
)

// Client is the endpoint logic, communicating with drand servers
// XXX: This API should go away. Do not extend any further.
type Client struct {
	client    net.PublicClient
	chainHash []byte
}

// NewGrpcClient returns a Client able to talk to drand instances using gRPC
// communication method.
//
// Deprecated: Use NewGrpcClientWithLogger
func NewGrpcClient(chainHash []byte, opts ...grpc.DialOption) *Client {
	lg := log.DefaultLogger()
	return NewGrpcClientWithLogger(lg, chainHash, opts...)
}

// NewGrpcClientWithLogger returns a Client able to talk to drand instances using gRPC
// communication method
func NewGrpcClientWithLogger(lg log.Logger, chainHash []byte, opts ...grpc.DialOption) *Client {
	return &Client{
		client:    net.NewGrpcClientWithLogger(lg, opts...),
		chainHash: chainHash,
	}
}

// NewGrpcClientFromCert returns a client that contact its peer over TLS
//
// Deprecated: Use NewGrpcClientFromCertWithLogger
func NewGrpcClientFromCert(chainHash []byte, c *net.CertManager, opts ...grpc.DialOption) *Client {
	lg := log.DefaultLogger()
	return NewGrpcClientFromCertWithLogger(lg, chainHash, c, opts...)
}

// NewGrpcClientFromCertWithLogger returns a client that contact its peer over TLS
func NewGrpcClientFromCertWithLogger(lg log.Logger, chainHash []byte, c *net.CertManager, opts ...grpc.DialOption) *Client {
	return &Client{
		client:    net.NewGrpcClientFromCertManagerWithLogger(lg, c, opts...),
		chainHash: chainHash,
	}
}

// ChainInfo returns the chain info as reported by the given peer.
func (c *Client) ChainInfo(p net.Peer) (*chain.Info, error) {
	metadata := common.Metadata{ChainHash: c.chainHash}
	resp, err := c.client.ChainInfo(context.TODO(), p, &drand.ChainInfoRequest{Metadata: &metadata})
	if err != nil {
		return nil, err
	}

	return chain.InfoFromProto(resp)
}
