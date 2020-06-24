package client

import (
	"context"
	"errors"
	"time"

	"github.com/drand/drand/chain"
)

const emptyClientStringerValue = "EmptyClient"

// EmptyClientWithInfo makes a client that returns the given info but no randomness
func EmptyClientWithInfo(info *chain.Info) Client {
	return &emptyClient{info}
}

type emptyClient struct {
	i *chain.Info
}

func (m *emptyClient) String() string {
	return emptyClientStringerValue
}

func (m *emptyClient) Info(ctx context.Context) (*chain.Info, error) {
	return m.i, nil
}

func (m *emptyClient) RoundAt(t time.Time) uint64 {
	return chain.CurrentRound(t.Unix(), m.i.Period, m.i.GenesisTime)
}

func (m *emptyClient) Get(ctx context.Context, round uint64) (Result, error) {
	return nil, errors.New("not supported")
}

func (m *emptyClient) Watch(ctx context.Context) <-chan Result {
	ch := make(chan Result, 1)
	close(ch)
	return ch
}

func (m *emptyClient) Close() error {
	return nil
}
