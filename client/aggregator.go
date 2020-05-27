package client

import (
	"context"
	"sync"

	"github.com/drand/drand/log"
)

// newWatchAggregator maintains state of consumers calling `Watch` so that a
// single `watch` request is made to the underlying client.
func newWatchAggregator(c Client, l log.Logger) *watchAggregator {
	return &watchAggregator{
		Client:      c,
		log:         l,
		subscribers: make([]subscriber, 0),
	}
}

type subscriber struct {
	ctx context.Context
	c   chan Result
}

type watchAggregator struct {
	Client
	log log.Logger

	subscriberLock sync.Mutex
	subscribers    []subscriber
}

func (c *watchAggregator) Watch(ctx context.Context) <-chan Result {
	c.subscriberLock.Lock()
	defer c.subscriberLock.Unlock()

	sub := subscriber{ctx, make(chan Result, 5)}
	c.subscribers = append(c.subscribers, sub)

	if len(c.subscribers) == 1 {
		ctx, cancel := context.WithCancel(context.Background())
		go c.distribute(c.Client.Watch(ctx), cancel)
	}
	return sub.c
}

func (c *watchAggregator) distribute(in <-chan Result, cancel context.CancelFunc) {
	defer cancel()
	for {
		var aCtx context.Context
		c.subscriberLock.Lock()
		if len(c.subscribers) == 0 {
			c.subscriberLock.Unlock()
			return
		}
		aCtx = c.subscribers[0].ctx
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
	}
}
