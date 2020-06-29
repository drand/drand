package client

import (
	"context"
	"fmt"
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
		errs = multierror.Append(errs, cw.Close())
	}
	errs = multierror.Append(errs, c.Client.Close())
	return errs.ErrorOrNil()
}

// String returns the name of this client.
func (c *watcherClient) String() string {
	return fmt.Sprintf("%s.(+watcher)", c.Client)
}
