package basic

import (
	"context"

	"github.com/drand/drand/client"
)

type watcherClient struct {
	client.Client
	watcher Watcher
}

func (c *watcherClient) Watch(ctx context.Context) <-chan client.Result {
	return c.watcher.Watch(ctx)
}
