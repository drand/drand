package client

import (
	"context"

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
		Client: client,
		cache:  cache,
		log:    log,
	}, nil
}

type cachingClient struct {
	Client

	cache *lru.ARCCache
	log   log.Logger
}

// Get returns the randomness at `round` or an error.
func (c *cachingClient) Get(ctx context.Context, round uint64) (res Result, err error) {
	if val, ok := c.cache.Get(round); ok {
		return val.(Result), nil
	}
	val, err := c.Client.Get(ctx, round)
	if err == nil && val != nil {
		c.cache.Add(round, val)
	}
	return val, err
}

func (c *cachingClient) Watch(ctx context.Context) <-chan Result {
	in = c.Client.Watch(ctx)
	out = make(chan Result)
	go func() {
		for result := range in {
			c.cache.Add(result.Round(), result.Randomness())
			out <- result
		}
		close(out)
	}()
	return out
}
