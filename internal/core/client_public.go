package core

import (
	"context"

	"google.golang.org/grpc"

	chain2 "github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/internal/metrics"
	"github.com/drand/drand/internal/net"
	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
)

// Client is the endpoint logic, communicating with drand servers
// TODO: This API should go away. Do not extend any further.
type Client struct {
	client    net.PublicClient
	chainHash []byte
}

// NewGrpcClient returns a Client able to talk to drand instances using gRPC
// communication method.
func NewGrpcClient(lg log.Logger, chainHash []byte, opts ...grpc.DialOption) *Client {
	return &Client{
		client:    net.NewGrpcClient(lg, opts...),
		chainHash: chainHash,
	}
}

// NewGrpcClientFromCert returns a client that contact its peer over TLS
func NewGrpcClientFromCert(lg log.Logger, chainHash []byte, c *net.CertManager, opts ...grpc.DialOption) *Client {
	return &Client{
		client:    net.NewGrpcClientFromCertManager(lg, c, opts...),
		chainHash: chainHash,
	}
}

// ChainInfo returns the chain info as reported by the given peer.
func (c *Client) ChainInfo(ctx context.Context, p net.Peer) (*chain2.Info, error) {
	ctx, span := metrics.NewSpan(ctx, "c.ChainInfo")
	defer span.End()

	metadata := common.Metadata{ChainHash: c.chainHash}
	resp, err := c.client.ChainInfo(ctx, p, &drand.ChainInfoRequest{Metadata: &metadata})
	if err != nil {
		return nil, err
	}

	return chain2.InfoFromProto(resp)
}
