package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/drand/drand/log"
)

const (
	aggregatorWatchBuffer = 5
	// defaultAutoWatchRetry is the time after which the watch channel
	// created by the autoWatch is re-opened when no context error occurred.
	defaultAutoWatchRetry = time.Second * 30
)

// newWatchAggregator maintains state of consumers calling `Watch` so that a
// single `watch` request is made to the underlying client.
func newWatchAggregator(c Client, autoWatch bool, autoWatchRetry time.Duration) *watchAggregator {
	if autoWatchRetry == 0 {
		autoWatchRetry = defaultAutoWatchRetry
	}
	aggregator := &watchAggregator{
		Client:         c,
		autoWatch:      autoWatch,
		autoWatchRetry: autoWatchRetry,
		log:            log.DefaultLogger(),
		subscribers:    make([]subscriber, 0),
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
	autoWatchRetry  time.Duration
	log             log.Logger
	cancelAutoWatch context.CancelFunc

	subscriberLock sync.Mutex
	subscribers    []subscriber
}

// Start initiates auto watching if configured to do so.
// SetLog should not be called after Start.
func (c *watchAggregator) Start() {
	if c.autoWatch {
		c.startAutoWatch()
	}
}

// SetLog configures the client log output
func (c *watchAggregator) SetLog(l log.Logger) {
	c.log = l
}

// String returns the name of this client.
func (c *watchAggregator) String() string {
	return fmt.Sprintf("%s.(+aggregator)", c.Client)
}

func (c *watchAggregator) startAutoWatch() {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancelAutoWatch = cancel
	go func() {
		for {
			results := c.Watch(ctx)
		LOOP:
			for {
				select {
				case _, ok := <-results:
					if !ok {
						c.log.Info("watch_aggregator", "auto watch ended")
						break LOOP
					}
				case <-ctx.Done():
					return
				}
			}
			if c.autoWatchRetry < 0 {
				return
			}
			t := time.NewTimer(c.autoWatchRetry)
			select {
			case <-t.C:
			case <-ctx.Done():
				t.Stop()
			}
			c.log.Info("watch_aggregator", "retrying auto watch")
		}
	}()
}

func (c *watchAggregator) Watch(ctx context.Context) <-chan Result {
	c.subscriberLock.Lock()
	defer c.subscriberLock.Unlock()

	sub := subscriber{ctx, make(chan Result, aggregatorWatchBuffer)}
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
		c.subscriberLock.Lock()
		if len(c.subscribers) == 0 {
			c.subscriberLock.Unlock()
			c.log.Warn("watch_aggregator", "no subscribers to distribute results to")
			return
		}
		aCtx := c.subscribers[0].ctx
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
						c.log.Warn("watch_aggregator", "dropped watch message to subscriber. full channel")
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
