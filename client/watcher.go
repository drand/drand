package client

import (
	"context"
	"time"

	"github.com/drand/drand/chain"
)

func newWatcherClient(base Client, chainInfo *chain.Info, ctor WatcherCtor) (Client, error) {
	w, err := ctor(chainInfo)
	if err != nil {
		return nil, err
	}
	return &watcherClient{base, w}, nil
}

type watcherClient struct {
	base    Client
	watcher Watcher
}

func (c *watcherClient) Watch(ctx context.Context) <-chan Result {
	return c.watcher.Watch(ctx)
}

func (c *watcherClient) Get(ctx context.Context, round uint64) (Result, error) {
	if c.base != nil {
		return c.base.Get(ctx, round)
	}
	panic("Get not supported by gossip watcher")
}

func (c *watcherClient) RoundAt(time time.Time) uint64 {
	if c.base != nil {
		return c.base.RoundAt(time)
	}
	panic("RoundAt not supported by gossip watcher")
}
