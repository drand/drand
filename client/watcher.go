package client

import (
	"context"
	"io"
)

type watcherClient struct {
	Client
	watcher Watcher
}

func (c *watcherClient) Watch(ctx context.Context) <-chan Result {
	return c.watcher.Watch(ctx)
}

func (c *watcherClient) Close() error {
	var err error
	cw, ok := c.watcher.(io.Closer)
	if ok {
		err = cw.Close()
	}
	cc, ok := c.Client.(io.Closer)
	if ok {
		err = cc.Close()
	}
	return err
}
