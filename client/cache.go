package client

import (
	"context"
	"fmt"

	lru "github.com/hashicorp/golang-lru"

	"github.com/drand/drand/common/client"
	"github.com/drand/drand/common/log"
)

// Cache provides a mechanism to check for rounds in the cache.
type Cache interface {
	// TryGet provides a round beacon or nil if it is not cached.
	TryGet(round uint64) client.Result
	// Add adds an item to the cache
	Add(uint64, client.Result)
}

// makeCache creates a cache of a given size
func makeCache(size int) (Cache, error) {
	if size == 0 {
		return &nilCache{}, nil
	}
	c, err := lru.NewARC(size)
	if err != nil {
		return nil, err
	}
	return &typedCache{c}, nil
}

// typedCache wraps an ARCCache containing beacon results.
type typedCache struct {
	*lru.ARCCache
}

// Add a result to the cache
func (t *typedCache) Add(round uint64, result client.Result) {
	t.ARCCache.Add(round, result)
}

// TryGet attempts to get a result from the cache
func (t *typedCache) TryGet(round uint64) client.Result {
	if val, ok := t.ARCCache.Get(round); ok {
		return val.(client.Result)
	}
	return nil
}

// nilCache implements a cache with size 0
type nilCache struct{}

// Add a result to the cache
func (*nilCache) Add(_ uint64, _ client.Result) {
}

// TryGet attempts to get ar esult from the cache
func (*nilCache) TryGet(_ uint64) client.Result {
	return nil
}

// NewCachingClient is a meta client that stores an LRU cache of
// recently fetched random values.
func NewCachingClient(l log.Logger, c client.Client, cache Cache) (client.Client, error) {
	return &cachingClient{
		Client: c,
		cache:  cache,
		log:    l,
	}, nil
}

type cachingClient struct {
	client.Client

	cache Cache
	log   log.Logger
}

// SetLog configures the client log output
func (c *cachingClient) SetLog(l log.Logger) {
	c.log = l
}

// String returns the name of this client.
func (c *cachingClient) String() string {
	if arc, ok := c.cache.(*typedCache); ok {
		return fmt.Sprintf("%s.(+%d el cache)", c.Client, arc.ARCCache.Len())
	}
	return fmt.Sprintf("%s.(+nil cache)", c.Client)
}

// Get returns the randomness at `round` or an error.
func (c *cachingClient) Get(ctx context.Context, round uint64) (res client.Result, err error) {
	if val := c.cache.TryGet(round); val != nil {
		return val, nil
	}
	val, err := c.Client.Get(ctx, round)
	if err == nil && val != nil {
		c.cache.Add(val.Round(), val)
	}
	return val, err
}

func (c *cachingClient) Watch(ctx context.Context) <-chan client.Result {
	in := c.Client.Watch(ctx)
	out := make(chan client.Result)
	go func() {
		for result := range in {
			c.cache.Add(result.Round(), result)
			out <- result
		}
		close(out)
	}()
	return out
}

func (c *cachingClient) Close() error {
	return c.Client.Close()
}
