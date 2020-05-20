package client

import (
	"context"
	"errors"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
)

// NewPrioritizingClient is a client combinator that asks sub-clients
// in succession until an answer is found.
// Get requests are sourced from get sub-clients.
// Watch requests are sourced from a watch sub-client if available, otherwise
// it is achieved as a long-poll from the prioritized get sub-clients.
func NewPrioritizingClient(watchClient Client, getClient []Client, groupHash []byte, group *key.Group, log log.Logger) (Client, error) {
	return &prioritizingClient{watchClient, getClient, groupHash, group, log}, nil
}

type prioritizingClient struct {
	watchClient Client
	getClient   []Client
	groupHash   []byte
	group       *key.Group
	log         log.Logger
}

// Get returns a the randomness at `round` or an error.
func (p *prioritizingClient) Get(ctx context.Context, round uint64) (res Result, err error) {
	for i, c := range p.getClient {
		res, err = c.Get(ctx, round)
		if err == nil {
			// previous clients failed. move them to end of priority.
			if i > 0 {
				p.getClient = append(p.getClient[i:], p.getClient[:i]...)
			}
			return
		}
		// context deadline hit
		if ctx.Err() != nil {
			return
		}
	}
	return
}

// Attempt to learn the trust root for the group from group-hash.
func (p *prioritizingClient) learnGroup(ctx context.Context) error {
	var group *key.Group
	var err error

	for _, c := range p.getClient {
		if hc, ok := c.(*httpClient); ok {
			group, err = hc.FetchGroupInfo(p.groupHash)
			if err == nil {
				p.group = group
				return nil
			}
		}
	}
	if err == nil {
		err = errors.New("No clients to learn group info from")
	}
	return err
}

// Watch returns new randomness as it becomes available.
func (p *prioritizingClient) Watch(ctx context.Context) <-chan Result {
	// prefer the watchClient channel for watches
	if p.watchClient != nil {
		return p.watchClient.Watch(ctx)
	}
	// otherwise, poll from the prioritized list of getClients
	if p.group == nil {
		if err := p.learnGroup(ctx); err != nil {
			log.DefaultLogger.Warn("prioritizing_client", "failed to learn group", "err", err)
			ch := make(chan Result, 0)
			close(ch)
			return ch
		}
	}
	return pollingWatcher(ctx, p, p.group, p.log)
}

// RoundAt will return the most recent round of randomness that will be available
// at time for the current client.
func (p *prioritizingClient) RoundAt(time time.Time) uint64 {
	return p.getClient[0].RoundAt(time)
}
