package client

import (
	"bytes"
	"errors"

	"github.com/drand/drand/key"
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
	if !cfg.withoutAggregation {
		coreClient = newWatchAggregator(coreClient, cfg.log)
	}
	return coreClient, nil
}

// makeClient creates a client from a configuration.
func makeClient(cfg clientConfig) (Client, error) {
	if !cfg.insecure && cfg.groupHash == nil && cfg.group == nil {
		return nil, errors.New("No root of trust specified")
	}
	if len(cfg.urls) == 0 {
		return nil, errors.New("No points of contact specified")
	}
	clients := []Client{}
	var c Client
	var err error
	for _, url := range cfg.urls {
		if cfg.group != nil {
			c, err = NewHTTPClientWithGroup(url, cfg.group, cfg.getter)
			if err != nil {
				return nil, err
			}
		} else {
			c, err = NewHTTPClient(url, cfg.groupHash, cfg.getter)
			if err != nil {
				return nil, err
			}
			group, err := c.(*httpClient).FetchGroupInfo(cfg.groupHash)
			if err != nil {
				return nil, err
			}
			cfg.group = group
		}
		c.(*httpClient).l = cfg.log
		clients = append(clients, c)
	}
	if len(clients) == 1 {
		return clients[0], nil
	}
	return NewPrioritizingClient(clients, cfg.groupHash, cfg.group, cfg.log)
}

type clientConfig struct {
	// URLs when specified will create an HTTP client.
	urls []string
	// Insecure will allow creating the HTTP client without a bound group.
	insecure bool
	// from `key.GroupInfo.Hash()` - serves as a root of trust.
	groupHash []byte
	// Full group information - serves as a root of trust.
	group *key.Group
	// getter configures the http transport parameters used when fetching randomness.
	getter HTTPGetter
	// cache size - how large of a cache to keep locally.
	cacheSize int
	// don't aggregate calls to `watch`
	withoutAggregation bool
	// customized client log.
	log log.Logger
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

// WithoutAggregation disables multiple calls to `Watch` being served by
// a single underlying http poll.
func WithoutAggregation() Option {
	return func(cfg *clientConfig) error {
		cfg.withoutAggregation = true
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

// WithGroupHash configures the client to root trust with a given drand group
// hash, the group parameters will be fetched from an HTTP endpoint.
func WithGroupHash(grouphash []byte) Option {
	return func(cfg *clientConfig) error {
		if cfg.group != nil && !bytes.Equal(cfg.group.Hash(), grouphash) {
			return errors.New("refusing to override group with non-matching hash")
		}
		cfg.groupHash = grouphash
		return nil
	}
}

// WithGroup configures the client to root trust in the given group information
func WithGroup(group *key.Group) Option {
	return func(cfg *clientConfig) error {
		if cfg.groupHash != nil && !bytes.Equal(cfg.groupHash, group.Hash()) {
			return errors.New("refusing to override hash with non-matching group")
		}
		cfg.group = group
		return nil
	}
}
