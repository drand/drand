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
// There are 3 modes taken by this aggregator. If autowatch is set, a single `watch`
// will always be invoked on the provided client. If it is not set, but a `watch client`(wc)
// is passed, a `watch` will be run on the watch client in the absence of external watchers,
// which will swap watching over to the main client. If no watch client is set and autowatch is off
// then a single watch will only run when an external watch is requested.
func newWatchAggregator(c, wc Client, autoWatch bool, autoWatchRetry time.Duration) *watchAggregator {
	if autoWatchRetry == 0 {
		autoWatchRetry = defaultAutoWatchRetry
	}
	aggregator := &watchAggregator{
		Client:         c,
		passiveClient:  wc,
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
	passiveClient   Client
	autoWatch       bool
	autoWatchRetry  time.Duration
	log             log.Logger
	cancelAutoWatch context.CancelFunc

	subscriberLock sync.Mutex
	subscribers    []subscriber
	cancelPassive  context.CancelFunc
}

// Start initiates auto watching if configured to do so.
// SetLog should not be called after Start.
func (c *watchAggregator) Start() {
	if c.autoWatch {
		c.startAutoWatch(true)
	} else if c.passiveClient != nil {
		c.startAutoWatch(false)
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

func (c *watchAggregator) startAutoWatch(full bool) {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancelAutoWatch = cancel
	go func() {
		for {
			var results <-chan Result
			if full {
				results = c.Watch(ctx)
			} else if c.passiveClient != nil {
				results = c.passiveWatch(ctx)
			}
		LOOP:
			for {
				select {
				case _, ok := <-results:
					if !ok {
						c.log.Infow("", "watch_aggregator", "auto watch ended")
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
			c.log.Infow("", "watch_aggregator", "retrying auto watch")
		}
	}()
}

// passiveWatch is a degraded form of watch, where watch only hits the 'passive client'
// unless distribution is actually needed.
func (c *watchAggregator) passiveWatch(ctx context.Context) <-chan Result {
	c.subscriberLock.Lock()
	defer c.subscriberLock.Unlock()

	if c.cancelPassive != nil {
		c.log.Warn("watch_aggregator", "only support one passive watch")
		return nil
	}

	wc := make(chan Result)
	if len(c.subscribers) == 0 {
		ctx, cancel := context.WithCancel(ctx)
		c.cancelPassive = cancel
		go c.sink(c.passiveClient.Watch(ctx), wc)
	} else {
		// trigger the startAutowatch to retry on backoff
		close(wc)
	}
	return wc
}

func (c *watchAggregator) Watch(ctx context.Context) <-chan Result {
	c.subscriberLock.Lock()
	defer c.subscriberLock.Unlock()

	sub := subscriber{ctx, make(chan Result, aggregatorWatchBuffer)}
	c.subscribers = append(c.subscribers, sub)

	if len(c.subscribers) == 1 {
		if c.cancelPassive != nil {
			c.cancelPassive()
			c.cancelPassive = nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		go c.distribute(c.Client.Watch(ctx), cancel)
	}
	return sub.c
}

func (c *watchAggregator) sink(in <-chan Result, out chan Result) {
	defer close(out)
	for range in {
		continue
	}
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
