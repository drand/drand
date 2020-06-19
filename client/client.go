package client

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
	"github.com/drand/drand/metrics"

	"github.com/prometheus/client_golang/prometheus"
)

const clientStartupTimeoutDefault = time.Second * 5

// New Creates a client with specified configuration.
func New(options ...Option) (Client, error) {
	cfg := clientConfig{
		cacheSize: 32,
		log:       log.DefaultLogger(),
	}
	for _, opt := range options {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	return makeClient(&cfg)
}

// Wrap provides a single entrypoint for wrapping a concrete client
// implementation with configured aggregation, caching, and retry logic
func Wrap(clients []Client, options ...Option) (Client, error) {
	return New(append(options, From(clients...))...)
}

func trySetLog(c Client, l log.Logger) {
	if lc, ok := c.(LoggingClient); ok {
		lc.SetLog(l)
	}
}

// makeClient creates a client from a configuration.
func makeClient(cfg *clientConfig) (Client, error) {
	if !cfg.insecure && cfg.chainHash == nil && cfg.chainInfo == nil {
		return nil, errors.New("no root of trust specified")
	}
	if len(cfg.clients) == 0 && cfg.watcher == nil {
		return nil, errors.New("no points of contact specified")
	}

	var err error

	// provision cache
	cache, err := makeCache(cfg.cacheSize)
	if err != nil {
		return nil, err
	}

	// provision watcher client
	if cfg.watcher != nil {
		wc, err := makeWatcherClient(cfg, cache)
		if err != nil {
			return nil, err
		}
		cfg.clients = append(cfg.clients, wc)
	}

	for _, c := range cfg.clients {
		trySetLog(c, cfg.log)
	}

	var c Client

	oc, err := newOptimizingClient(cfg.clients, 0, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	c = oc
	trySetLog(c, cfg.log)
	oc.Start()

	if cfg.cacheSize > 0 {
		c, err = NewCachingClient(c, cache)
		if err != nil {
			return nil, err
		}
		trySetLog(c, cfg.log)
	}

	c = newWatchAggregator(c, cfg.autoWatch)
	trySetLog(c, cfg.log)

	return attachMetrics(cfg, c)
}

func makeWatcherClient(cfg *clientConfig, cache Cache) (Client, error) {
	if err := cfg.tryPopulateInfo(cfg.clients...); err != nil {
		return nil, err
	}
	w, err := cfg.watcher(cfg.chainInfo, cache)
	if err != nil {
		return nil, err
	}
	ec := EmptyClientWithInfo(cfg.chainInfo)
	return &watcherClient{ec, w}, nil
}

func attachMetrics(cfg *clientConfig, c Client) (Client, error) {
	if cfg.prometheus != nil {
		if err := metrics.RegisterClientMetrics(cfg.prometheus); err != nil {
			return nil, err
		}
		if err := cfg.tryPopulateInfo(c); err != nil {
			return nil, err
		}
		return newWatchLatencyMetricClient(c, cfg.chainInfo), nil
	}
	return c, nil
}

type clientConfig struct {
	// clients is the set of options for fetching randomness
	clients []Client
	// watcher is a constructor function for generating a new partial client of randomness
	watcher WatcherCtor
	// from `chainInfo.Hash()` - serves as a root of trust for a given
	// randomness chain.
	chainHash []byte
	// Full chain information - serves as a root of trust.
	chainInfo *chain.Info
	// insecure indicates the root of trust does not need to be present.
	insecure bool
	// cache size - how large of a cache to keep locally.
	cacheSize int
	// customized client log.
	log log.Logger
	// autoWatch causes the client to start watching immediately in the background so that new randomness
	// is proactively fetched and added to the cache.
	autoWatch bool
	// prometheus is an interface to a Prometheus system
	prometheus prometheus.Registerer
}

func (c *clientConfig) tryPopulateInfo(clients ...Client) (err error) {
	if c.chainInfo == nil {
		ctx, cancel := context.WithTimeout(context.Background(), clientStartupTimeoutDefault)
		defer cancel()
		for _, cli := range clients {
			c.chainInfo, err = cli.Info(ctx)
			if err == nil {
				return
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
	}
	return
}

// Option is an option configuring a client.
type Option func(cfg *clientConfig) error

// From constructs the client from a set of clients providing randomness
func From(c ...Client) Option {
	return func(cfg *clientConfig) error {
		cfg.clients = c
		return nil
	}
}

// Insecurely indicates the client should be allowed to provide randomness
// when the root of trust is not fully provided in a validate-able way.
func Insecurely() Option {
	return func(cfg *clientConfig) error {
		cfg.insecure = true
		return nil
	}
}

// WithCacheSize specifies how large of a cache of randomness values should be
// kept locally. Default 32
func WithCacheSize(size int) Option {
	return func(cfg *clientConfig) error {
		cfg.cacheSize = size
		return nil
	}
}

// WithLogger overrides the logging options for the client,
// allowing specification of additional tags, or redirection / configuration
// of logging level and output.
func WithLogger(l log.Logger) Option {
	return func(cfg *clientConfig) error {
		cfg.log = l
		return nil
	}
}

// WithChainHash configures the client to root trust with a given randomness
// chain hash, the chain parameters will be fetched from an HTTP endpoint.
func WithChainHash(chainHash []byte) Option {
	return func(cfg *clientConfig) error {
		if cfg.chainInfo != nil && !bytes.Equal(cfg.chainInfo.Hash(), chainHash) {
			return errors.New("refusing to override group with non-matching hash")
		}
		cfg.chainHash = chainHash
		return nil
	}
}

// WithChainInfo configures the client to root trust in the given randomness
// chain information
func WithChainInfo(chainInfo *chain.Info) Option {
	return func(cfg *clientConfig) error {
		if cfg.chainHash != nil && !bytes.Equal(cfg.chainHash, chainInfo.Hash()) {
			return errors.New("refusing to override hash with non-matching group")
		}
		cfg.chainInfo = chainInfo
		return nil
	}
}

// Watcher supplies the `Watch` portion of the drand client interface.
type Watcher interface {
	Watch(ctx context.Context) <-chan Result
}

// WatcherCtor creates a Watcher once chain info is known.
type WatcherCtor func(chainInfo *chain.Info, cache Cache) (Watcher, error)

// WithWatcher specifies a channel that can provide notifications of new
// randomness bootstrappeed from the chain info.
func WithWatcher(wc WatcherCtor) Option {
	return func(cfg *clientConfig) error {
		cfg.watcher = wc
		return nil
	}
}

// WithAutoWatch causes the client to automatically attempt to get
// randomness for rounds, so that it will hopefully already be cached
// when `Get` is called.
func WithAutoWatch() Option {
	return func(cfg *clientConfig) error {
		cfg.autoWatch = true
		return nil
	}
}

// WithPrometheus specifies a registry into which to report metrics
func WithPrometheus(r prometheus.Registerer) Option {
	return func(cfg *clientConfig) error {
		cfg.prometheus = r
		return nil
	}
}
