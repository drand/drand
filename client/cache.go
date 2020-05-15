package client

import (
	"context"
	"time"

	"github.com/drand/drand/log"
	lru "github.com/hashicorp/golang-lru"
)

// NewCachingClient is a meta client that stores an LRU cache of
// recently fetched random values.
func NewCachingClient(client Client, size int, log log.Logger) (Client, error) {
	cache, err := lru.NewARC(size)
	if err != nil {
		return nil, err
	}
	return &cachingClient{
		backing: client,
		cache:   cache,
		log:     log,
	}, nil
}

type cachingClient struct {
	backing Client
	cache   *lru.ARCCache
	log     log.Logger
}

// Get returns the randomness at `round` or an error.
func (c *cachingClient) Get(ctx context.Context, round uint64) (res Result, err error) {
	if val, ok := c.cache.Get(round); ok {
		return val.(Result), nil
	}
	val, err := c.backing.Get(ctx, round)
	if err == nil && val != nil {
		c.cache.Add(round, val)
	}
	return val, err
}

// Watch will stream new results as they are discovered.
func (c *cachingClient) Watch(ctx context.Context) <-chan Result {
	return c.backing.Watch(ctx)
}

// RoundAt will return the most recent round of randomness that will be available
// at time for the current client.
func (c *cachingClient) RoundAt(time time.Time) uint64 {
	return c.backing.RoundAt(time)
}
