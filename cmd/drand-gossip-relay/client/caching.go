package client

import (
	"context"
	"sync"
	"time"

	dclient "github.com/drand/drand/client"
	"github.com/drand/drand/log"
	lru "github.com/hashicorp/golang-lru"
)

// NewCachingClient is a meta client that stores an LRU cache of
// recently fetched random values.
func NewCachingClient(client dclient.Client, size int, log log.Logger) (dclient.Client, error) {
	cache, err := lru.NewARC(size)
	if err != nil {
		return nil, err
	}
	return &cachingClient{
		backing:     client,
		cache:       cache,
		log:         log,
		subscribers: make([]subscriber, 0),
	}, nil
}

type cachingClient struct {
	backing dclient.Client
	cache   *lru.ARCCache
	log     log.Logger

	subscriberLock sync.Mutex
	subscribers    []subscriber
}

type subscriber struct {
	ctx context.Context
	c   chan dclient.Result
}

// Get returns a the randomness at `round` or an error.
func (c *cachingClient) Get(ctx context.Context, round uint64) (res dclient.Result, err error) {
	if val, ok := c.cache.Get(round); ok {
		return val.(dclient.Result), nil
	}
	val, err := c.backing.Get(ctx, round)
	if err == nil && val != nil {
		c.cache.Add(round, val)
	}
	return val, err
}

// Watch returns new randomness as it becomes available.
func (c *cachingClient) Watch(ctx context.Context) <-chan dclient.Result {
	c.subscriberLock.Lock()
	defer c.subscriberLock.Unlock()

	sub := subscriber{ctx, make(chan dclient.Result, 5)}
	c.subscribers = append(c.subscribers, sub)

	if len(c.subscribers) == 1 {
		ctx, cancel := context.WithCancel(context.Background())
		go c.distribute(c.backing.Watch(ctx), cancel)
	}
	return sub.c
}

func (c *cachingClient) distribute(in <-chan dclient.Result, cancel context.CancelFunc) {
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

		var m dclient.Result
		select {
		case res, ok := <-in:
			if !ok {
				return
			}
			c.cache.Add(res.Round(), res)
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

// RoundAt will return the most recent round of randomness that will be available
// at time for the current client.
func (c *cachingClient) RoundAt(time time.Time) uint64 {
	return c.backing.RoundAt(time)
}
