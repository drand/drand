package client

import (
	"context"
	"io"

	"github.com/hashicorp/go-multierror"
)

type watcherClient struct {
	Client
	watcher Watcher
}

func (c *watcherClient) Watch(ctx context.Context) <-chan Result {
	return c.watcher.Watch(ctx)
}

func (c *watcherClient) Close() error {
	var errs *multierror.Error
	cw, ok := c.watcher.(io.Closer)
	if ok {
		errs = multierror.Append(cw.Close())
	}
	errs = multierror.Append(c.Client.Close())
	return errs.ErrorOrNil()
}
