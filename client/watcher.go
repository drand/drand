package client

import (
	"context"

	"github.com/drand/drand/key"
)

func newWatcherClient(base Client, group *key.Group, ctor WatcherCtor) (Client, error) {
	w, err := ctor(group)
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
