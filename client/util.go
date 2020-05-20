package client

import (
	"context"
	"sync"
	"time"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
)

// pollingWatcher generalizes the `Watch` interface for clients which learn new values
// by asking for them once each group period.
func pollingWatcher(ctx context.Context, client Client, group *key.Group, log log.Logger) <-chan Result {
	ch := make(chan Result, 1)
	r := client.RoundAt(time.Now())
	val, err := client.Get(ctx, r)
	if err != nil {
		log.Error("polling_client", "failed to watch", "err", err)
		close(ch)
		return ch
	}
	ch <- val

	go func() {
		defer close(ch)

		// Initially, wait to synchronize to the round boundary.
		_, nextTime := beacon.NextRound(time.Now().Unix(), group.Period, group.GenesisTime)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(nextTime-time.Now().Unix()) * time.Second):
		}

		r, err := client.Get(ctx, client.RoundAt(time.Now()))
		if err == nil {
			ch <- r
		} else {
			log.Error("polling_client", "failed to watch", "err", err)
		}

		// Then tick each period.
		t := time.NewTicker(group.Period)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				r, err := client.Get(ctx, client.RoundAt(time.Now()))
				if err == nil {
					ch <- r
				} else {
					log.Error("polling_client", "failed to watch", "err", err)
				}
				// TODO: keep trying on errors?
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch
}

// NewWatchAggregator maintains state of consumers calling `Watch` so that a
// single `watch` request is made to the underlying client.
func NewWatchAggregator(c Client, l log.Logger) Client {
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
		select {
		case res, ok := <-in:
			if !ok {
				return
			}
			m = res
		case <-aCtx.Done():
		}

		c.subscriberLock.Lock()
		curr := c.subscribers
		c.subscribers = c.subscribers[:0]

		for _, s := range curr {
			if s.ctx.Err() == nil {
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
