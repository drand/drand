package core

import (
	"context"

	"github.com/drand/drand/common/tracer"

	"google.golang.org/grpc"

	"github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/internal/net"
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

// ChainInfo returns the chain info as reported by the given peer.
func (c *Client) ChainInfo(ctx context.Context, p net.Peer) (*chain.Info, error) {
	ctx, span := tracer.NewSpan(ctx, "c.ChainInfo")
	defer span.End()

	metadata := drand.Metadata{ChainHash: c.chainHash}
	resp, err := c.client.ChainInfo(ctx, p, &drand.ChainInfoRequest{Metadata: &metadata})
	if err != nil {
		return nil, err
	}

	return chain.InfoFromProto(resp)
}
