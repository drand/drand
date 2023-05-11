package client

import (
	"context"
	"time"

	"github.com/drand/drand/common"
	chain2 "github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/client"
	"github.com/drand/drand/internal/chain"
)

const emptyClientStringerValue = "EmptyClient"

// EmptyClientWithInfo makes a client that returns the given info but no randomness
func EmptyClientWithInfo(info *chain2.Info) client.Client {
	return &emptyClient{info}
}

type emptyClient struct {
	i *chain2.Info
}

func (m *emptyClient) String() string {
	return emptyClientStringerValue
}

func (m *emptyClient) Info(_ context.Context) (*chain2.Info, error) {
	return m.i, nil
}

func (m *emptyClient) RoundAt(t time.Time) uint64 {
	return chain.CurrentRound(t.Unix(), m.i.Period, m.i.GenesisTime)
}

func (m *emptyClient) Get(_ context.Context, _ uint64) (client.Result, error) {
	return nil, common.ErrEmptyClientUnsupportedGet
}

func (m *emptyClient) Watch(_ context.Context) <-chan client.Result {
	ch := make(chan client.Result, 1)
	close(ch)
	return ch
}

func (m *emptyClient) Close() error {
	return nil
}
