package mock

import (
	"context"
	"github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/client"
	"github.com/drand/drand/protobuf/drand"
	"time"
)

type GrpcClient struct {
	s *Server
}

func NewGrpcClient(s *Server) *GrpcClient {
	return &GrpcClient{s: s}
}

func (c *GrpcClient) Get(ctx context.Context, round uint64) (client.Result, error) {
	return c.s.PublicRand(ctx, &drand.PublicRandRequest{
		Round:    round,
		Metadata: nil,
	})
}

func (c *GrpcClient) Watch(ctx context.Context) <-chan client.Result {
	return nil
}

func (c *GrpcClient) Info(ctx context.Context) (*chain.Info, error) {
	resp, err := c.s.ChainInfo(ctx, nil)
	if err != nil {
		return nil, err
	}

	//return &chain.Info{
	//	PublicKey:   resp.PublicKey,
	//	Period:      time.Duration(resp.Period),
	//	Scheme:      resp.SchemeID,
	//	GenesisTime: resp.GenesisTime,
	//}, err
	return chain.InfoFromProto(resp)
}

func (c *GrpcClient) RoundAt(time time.Time) uint64 {
	return 0
}

func (c *GrpcClient) Close() error {
	return nil
}
