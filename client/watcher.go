package client

import (
	"context"

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
	Client
	watcher Watcher
}

func (c *watcherClient) Watch(ctx context.Context) <-chan Result {
	return c.watcher.Watch(ctx)
}
