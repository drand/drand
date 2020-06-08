package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// New Creates a client with specified configuration.
func New(options ...Option) (Client, error) {
	cfg := clientConfig{
		cacheSize: 32,
		log:       log.DefaultLogger,
		timeout:   time.Second * 5,
		rttTTL:    time.Minute * 5,
	}
	for _, opt := range options {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	return makeClient(cfg)
}

func trySetLog(c Client, l log.Logger) {
	if lc, ok := c.(LoggingClient); ok {
		lc.SetLog(l)
	}
}

// makeClient creates a client from a configuration.
func makeClient(cfg clientConfig) (Client, error) {
	if !cfg.insecure && cfg.chainHash == nil && cfg.chainInfo == nil {
		return nil, errors.New("No root of trust specified")
	}
	if len(cfg.urls) == 0 {
		return nil, errors.New("No points of contact specified")
	}

	// provision REST clients
	restClients := []Client{}
	var c Client
	var err error
	for _, url := range cfg.urls {
		if cfg.chainInfo != nil {
			c, err = NewHTTPClientWithInfo(url, cfg.chainInfo, cfg.transport)
			if err != nil {
				return nil, err
			}
		} else {
			c, err = NewHTTPClient(url, cfg.chainHash, cfg.transport)
			if err != nil {
				return nil, err
			}
			chainInfo, err := c.(*httpClient).FetchChainInfo(cfg.chainHash)
			if err != nil {
				return nil, err
			}
			cfg.chainInfo = chainInfo
		}
		trySetLog(c, cfg.log)
		restClients = append(restClients, c)
	}
	if cfg.prometheus != nil {
		newHTTPHealthMetrics(cfg.urls, restClients, cfg.chainInfo)
	}

	if len(restClients) > 1 {
		c, err = NewOptimizingClient(restClients, cfg.chainInfo, cfg.timeout, cfg.headStart, cfg.rttTTL)
		if err != nil {
			return nil, err
		}
		trySetLog(c, cfg.log)
	}

	// provision cache
	cache, err := makeCache(cfg.cacheSize)
	if err != nil {
		return nil, err
	}

	// provision watcher client
	if cfg.watcher != nil {
		w, err := cfg.watcher(cfg.chainInfo, cache)
		if err != nil {
			return nil, err
		}
		if lw, ok := w.(LoggingClient); ok {
			lw.SetLog(cfg.log)
		}
		c = &watcherClient{c, w}
	}

	if cfg.cacheSize > 0 {
		c, err = NewCachingClient(c, cache)
		if err != nil {
			return nil, err
		}
		trySetLog(c, cfg.log)
	}

	if cfg.failoverGracePeriod > 0 {
		c, err = NewFailoverWatcher(c, cfg.chainInfo, cfg.failoverGracePeriod)
		if err != nil {
			return nil, err
		}
		trySetLog(c, cfg.log)
	}

	c = newWatchAggregator(c, cfg.autoWatch)
	trySetLog(c, cfg.log)

	if cfg.prometheus != nil {
		metrics.RegisterClientMetrics(cfg.prometheus)
		if cfg.chainInfo == nil {
			return nil, fmt.Errorf("prometheus enabled, but chain info not known")
		}
		if c, err = newWatchLatencyMetricClient(c, cfg.chainInfo); err != nil {
			return nil, err
		}
	}

	return c, nil
}

type clientConfig struct {
	// URLs when specified will create an HTTP client.
	urls []string
	// Insecure will allow creating the HTTP client without a bound group.
	insecure bool
	// from `chainInfo.Hash()` - serves as a root of trust for a given
	// randomness chain.
	chainHash []byte
	// Full chain information - serves as a root of trust.
	chainInfo *chain.Info
	// transport configures the http parameters used when fetching randomness.
	transport http.RoundTripper
	// cache size - how large of a cache to keep locally.
	cacheSize int
	// customized client log.
	log log.Logger
	// time after which a watcher will failover to using client.Get to get the latest randomness.
	failoverGracePeriod time.Duration
	// watcher is a constructor function that creates a new Watcher
	watcher WatcherCtor
	// autoWatch causes the client to start watching immediately in the background so that new randomness is proactively fetched and added to the cache.
	autoWatch bool
	// prometheus is an interface to a Prometheus system
	prometheus prometheus.Registerer
	// rttTTL is the time a RTT sample will live before it is tested again.
	rttTTL time.Duration
	// timeout is the timeout for calls to `Get`. By default this is 5s.
	timeout time.Duration
	// headStart is the time given to the fastest client before which we race
	// the others to `Get` a `Result`.
	headStart time.Duration
}

// Option is an option configuring a client.
type Option func(cfg *clientConfig) error

// WithHTTPEndpoints configures the client to use the provided URLs.
func WithHTTPEndpoints(urls []string) Option {
	return func(cfg *clientConfig) error {
		if cfg.insecure {
			return errors.New("Cannot mix secure and insecure URLs")
		}
		cfg.urls = append(cfg.urls, urls...)
		return nil
	}
}

// WithHTTPTransport specifies the HTTP Client (or mocked equivalent) for fetching
// randomness from an HTTP endpoint.
func WithHTTPTransport(transport http.RoundTripper) Option {
	return func(cfg *clientConfig) error {
		cfg.transport = transport
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

// WithInsecureHTTPEndpoints configures the client to pull randomness from
// provided URLs without validating the group trust root.
func WithInsecureHTTPEndpoints(urls []string) Option {
	return func(cfg *clientConfig) error {
		if len(cfg.urls) != 0 && !cfg.insecure {
			return errors.New("Cannot mix secure and insecure URLs")
		}
		cfg.urls = append(cfg.urls, urls...)
		cfg.insecure = true
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

// WithFailoverGracePeriod enables failover if set and configures the time after
// which a watcher will failover to using client.Get to get the latest randomness.
func WithFailoverGracePeriod(d time.Duration) Option {
	return func(cfg *clientConfig) error {
		cfg.failoverGracePeriod = d
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

// WithTimeout sets the timeout for calls to `Get`. By default this is 5s. Set
// to 0 for no timeout.
func WithTimeout(t time.Duration) Option {
	return func(cfg *clientConfig) error {
		cfg.timeout = t
		return nil
	}
}

// WithHeadStart is the time a call to `.Get` will have before which other
// clients are raced for the result. When multiple clients are available they
// are measured for speed and the fastest one is used. This duration gives the
// fastest client time to respond before the other clients are asked.
func WithHeadStart(t time.Duration) Option {
	return func(cfg *clientConfig) error {
		cfg.headStart = t
		return nil
	}
}

// WithRoundTripTTL is the time a RTT sample will live before it is tested
// again. Calls to `.Get` are timed so that when multiple endpoints are
// available the fastest one can be used. Sample round trip times live for a
// certain period before they are reset. This allows a slow or unavailable
// client to recover and become the fastest.
func WithRoundTripTTL(t time.Duration) Option {
	return func(cfg *clientConfig) error {
		cfg.rttTTL = t
		return nil
	}
}
