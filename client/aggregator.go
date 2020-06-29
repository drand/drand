package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/drand/drand/log"
)

const aggregatorWatchBuffer = 5

// newWatchAggregator maintains state of consumers calling `Watch` so that a
// single `watch` request is made to the underlying client.
func newWatchAggregator(c Client, autoWatch bool) *watchAggregator {
	aggregator := &watchAggregator{
		Client:      c,
		autoWatch:   autoWatch,
		log:         log.DefaultLogger(),
		subscribers: make([]subscriber, 0),
	}
	if autoWatch {
		ctx, cancel := context.WithCancel(context.Background())
		aggregator.cancelAutoWatch = cancel
		go aggregator.distribute(aggregator.Client.Watch(ctx), cancel, true)
	}
	return aggregator
}

type subscriber struct {
	ctx context.Context
	c   chan Result
}

type watchAggregator struct {
	Client
	autoWatch       bool
	log             log.Logger
	cancelAutoWatch context.CancelFunc

	subscriberLock sync.Mutex
	subscribers    []subscriber
}

// SetLog configures the client log output
func (c *watchAggregator) SetLog(l log.Logger) {
	c.log = l
}

// String returns the name of this client.
func (c *watchAggregator) String() string {
	return fmt.Sprintf("%s.(+aggregator)", c.Client)
}

func (c *watchAggregator) Watch(ctx context.Context) <-chan Result {
	c.subscriberLock.Lock()
	defer c.subscriberLock.Unlock()

	sub := subscriber{ctx, make(chan Result, aggregatorWatchBuffer)}
	c.subscribers = append(c.subscribers, sub)

	if len(c.subscribers) == 1 && !c.autoWatch {
		ctx, cancel := context.WithCancel(context.Background())
		go c.distribute(c.Client.Watch(ctx), cancel, false)
	}
	return sub.c
}

func (c *watchAggregator) distribute(in <-chan Result, cancel context.CancelFunc, autoWatch bool) {
	defer cancel()
	for {
		aCtx := context.Background()
		c.subscriberLock.Lock()
		if len(c.subscribers) == 0 && !autoWatch {
			c.subscriberLock.Unlock()
			return
		} else if len(c.subscribers) > 0 {
			aCtx = c.subscribers[0].ctx
		}
		c.subscriberLock.Unlock()

		var m Result
		var ok bool

		select {
		case m, ok = <-in:
		case <-aCtx.Done():
		}

		c.subscriberLock.Lock()
		curr := c.subscribers
		c.subscribers = c.subscribers[:0]

		for _, s := range curr {
			if ok && s.ctx.Err() == nil {
				c.subscribers = append(c.subscribers, s)
				if m != nil {
					select {
					case s.c <- m:
					default:
						c.log.Warn("msg", "dropped watch message to subscriber. full channel")
					}
				}
			} else {
				close(s.c)
			}
		}
		c.subscriberLock.Unlock()

		if !ok {
			return
		}
	}
}

func (c *watchAggregator) Close() error {
	err := c.Client.Close()
	if c.cancelAutoWatch != nil {
		c.cancelAutoWatch()
	}
	return err
}
