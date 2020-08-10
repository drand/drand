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
	var wc Client
	if cfg.watcher != nil {
		wc, err = makeWatcherClient(cfg, cache)
		if err != nil {
			return nil, err
		}
		cfg.clients = append(cfg.clients, wc)
	}

	for _, c := range cfg.clients {
		trySetLog(c, cfg.log)
	}

	var c Client

	verifiers := make([]Client, 0, len(cfg.clients))
	for _, source := range cfg.clients {
		nv := newVerifyingClient(source, cfg.previousResult, cfg.fullVerify)
		verifiers = append(verifiers, nv)
		if source == wc {
			wc = nv
		}
	}

	c, err = makeOptimizingClient(cfg, verifiers, wc, cache)
	if err != nil {
		return nil, err
	}

	wa := newWatchAggregator(c, cfg.autoWatch, cfg.autoWatchRetry)
	c = wa
	trySetLog(c, cfg.log)

	wa.Start()

	return attachMetrics(cfg, c)
}

func makeOptimizingClient(cfg *clientConfig, verifiers []Client, watcher Client, cache Cache) (Client, error) {
	oc, err := newOptimizingClient(verifiers, 0, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	if watcher != nil {
		oc.MarkPassive(watcher)
	}
	c := Client(oc)
	trySetLog(c, cfg.log)

	if cfg.cacheSize > 0 {
		c, err = NewCachingClient(c, cache)
		if err != nil {
			return nil, err
		}
		trySetLog(c, cfg.log)
	}
	for _, v := range verifiers {
		trySetLog(v, cfg.log)
		v.(*verifyingClient).indirectClient = c
	}

	oc.Start()
	return c, nil
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
	// A previously fetched result serving as a verification checkpoint if one exists.
	previousResult Result
	// chain signature verification back to the 1st round, or to a know result to ensure
	// determinism in the event of a compromised chain.
	fullVerify bool
	// insecure indicates the root of trust does not need to be present.
	insecure bool
	// cache size - how large of a cache to keep locally.
	cacheSize int
	// customized client log.
	log log.Logger
	// autoWatch causes the client to start watching immediately in the background so that new randomness
	// is proactively fetched and added to the cache.
	autoWatch bool
	// autoWatchRetry specifies the time after which the watch channel
	// created by the autoWatch is re-opened when no context error occurred.
	autoWatchRetry time.Duration
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

// WithVerifiedResult provides a checkpoint of randomness verified at a given round.
// Used in combination with `VerifyFullChain`, this allows for catching up only on
// previously not-yet-verified results.
func WithVerifiedResult(result Result) Option {
	return func(cfg *clientConfig) error {
		if cfg.previousResult != nil && cfg.previousResult.Round() > result.Round() {
			return errors.New("refusing to override verified result with an earlier result")
		}
		cfg.previousResult = result
		return nil
	}
}

// WithFullChainVerification validates random beacons not just as being generated correctly
// from the group signature, but ensures that the full chain is deterministic by making sure
// each round is derived correctly from the previous one. In cases of compromise where
// a single party learns sufficient shares to derive the full key, malicious randomness
// could otherwise be generated that is signed, but not properly derived from previous rounds
// according to protocol.
func WithFullChainVerification() Option {
	return func(cfg *clientConfig) error {
		cfg.fullVerify = true
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

// WithAutoWatchRetry specifies the time after which the watch channel
// created by the autoWatch is re-opened when no context error occurred.
// Set to a negative value to disable retrying auto watch.
func WithAutoWatchRetry(interval time.Duration) Option {
	return func(cfg *clientConfig) error {
		cfg.autoWatchRetry = interval
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
