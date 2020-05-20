package client

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
)

// New Creates a client with specified configuration.
func New(options ...Option) (Client, error) {
	cfg := clientConfig{
		cacheSize: 32,
		log:       log.DefaultLogger,
	}
	for _, opt := range options {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}

	coreClient, err := makeClient(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.cacheSize > 0 {
		coreClient, err = NewCachingClient(coreClient, cfg.cacheSize, cfg.log)
		if err != nil {
			return nil, err
		}
	}
	if cfg.failoverGracePeriod > 0 {
		coreClient = NewFailoverWatcher(coreClient, cfg.chainInfo, cfg.failoverGracePeriod, cfg.log)
	}
	return newWatchAggregator(coreClient, cfg.log), nil
}

// makeClient creates a client from a configuration.
func makeClient(cfg clientConfig) (Client, error) {
	if !cfg.insecure && cfg.chainHash == nil && cfg.chainInfo == nil {
		return nil, errors.New("No root of trust specified")
	}
	if len(cfg.urls) == 0 {
		return nil, errors.New("No points of contact specified")
	}
	clients := []Client{}
	var c Client
	var err error
	for _, url := range cfg.urls {
		if cfg.chainInfo != nil {
			c, err = NewHTTPClientWithInfo(url, cfg.chainInfo, cfg.getter)
			if err != nil {
				return nil, err
			}
		} else {
			c, err = NewHTTPClient(url, cfg.chainHash, cfg.getter)
			if err != nil {
				return nil, err
			}
			group, err := c.(*httpClient).FetchChainInfo(cfg.chainHash)
			if err != nil {
				return nil, err
			}
			cfg.chainInfo = group
		}
		c.(*httpClient).l = cfg.log
		clients = append(clients, c)
	}
	if len(clients) == 1 {
		return clients[0], nil
	}
	return NewPrioritizingClient(nil, clients, cfg.chainHash, cfg.chainInfo, cfg.log)
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
	// getter configures the http transport parameters used when fetching randomness.
	getter HTTPGetter
	// cache size - how large of a cache to keep locally.
	cacheSize int
	// customized client log.
	log log.Logger
	// time after which a watcher will failover to using client.Get to get the latest randomness.
	failoverGracePeriod time.Duration
	// watcher
	watcher WatcherCtor
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

// WithHTTPGetter specifies the HTTP Client (or mocked equivalent) for fetching
// randomness from an HTTP endpoint.
func WithHTTPGetter(getter HTTPGetter) Option {
	return func(cfg *clientConfig) error {
		cfg.getter = getter
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

// WatcherCtor creates a Watcher once a group is known.
type WatcherCtor func(group *key.Group) (Watcher, error)

// WithWatcher specifies a channel that can provide notifications of new
// randomness bootstrappeed from the group information.
func WithWatcher(wc WatcherCtor) Option {
	return func(cfg *clientConfig) error {
		cfg.watcher = wc
		return nil
	}
}
