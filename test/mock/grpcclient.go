package mock

import (
	"context"
	"time"

	"github.com/drand/drand/v2/common/chain"
	"github.com/drand/drand/v2/common/client"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/core"
	"github.com/drand/drand/v2/protobuf/drand"
)

type GrpcClient struct {
	s *Server
}

func NewGrpcClient(s *Server) *GrpcClient {
	return &GrpcClient{s: s}
}

func (c *GrpcClient) Get(ctx context.Context, round uint64) (client.Result, error) {
	rand, err := c.s.PublicRand(ctx, &drand.PublicRandRequest{
		Round:    round,
		Metadata: nil,
	})
	if err != nil {
		return nil, err
	}

	// we need to set the randomness now since it isn't sent over the wire anymore in V2
	rand.Randomness = crypto.RandomnessFromSignature(rand.GetSignature())

	return rand, nil
}

func (c *GrpcClient) Watch(ctx context.Context) <-chan client.Result {
	proxy := core.Proxy(c.s)
	return proxy.Watch(ctx)
}

func (c *GrpcClient) Info(ctx context.Context) (*chain.Info, error) {
	resp, err := c.s.ChainInfo(ctx, nil)
	if err != nil {
		return nil, err
	}

	return chain.InfoFromProto(resp)
}

func (c *GrpcClient) RoundAt(_ time.Time) uint64 {
	// not implemented in the mock client
	return 0
}

func (c *GrpcClient) Close() error {
	return nil
}
