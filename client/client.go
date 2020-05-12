package client

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/drand/drand/key"
)

// New Creates a client with specified configuration.
func New(options ...Option) (Client, error) {
	cfg := clientConfig{}
	for _, opt := range options {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}

	return makeClient(cfg)
}

// makeClient creates a client from a configuration.
func makeClient(cfg clientConfig) (Client, error) {
	if !cfg.insecure && cfg.groupHash == nil && cfg.group == nil {
		fmt.Printf("%#v\n", cfg)
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
		clients = append(clients, c)
	}
	if len(clients) == 1 {
		return clients[0], nil
	}
	return NewPrioritizingClient(clients, cfg.groupHash, cfg.group)
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
		cfg.group = group
		cfg.groupHash = nil
		return nil
	}
}
