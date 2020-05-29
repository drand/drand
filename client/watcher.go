package client

import (
	"context"
)

type watcherClient struct {
	Client
	watcher Watcher
}

func (c *watcherClient) Watch(ctx context.Context) <-chan Result {
	return c.watcher.Watch(ctx)
}
