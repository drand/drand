package client

import (
	"context"
	"errors"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
)

// NewPrioritizingClient is a meta client that asks each sub-client
// in succession on requests until an answer is found. If a sub-client
// fails, it is moved to the end of the list.
func NewPrioritizingClient(clients []Client, groupHash []byte, group *key.Group) (Client, error) {
	return &prioritizingClient{clients, groupHash, group}, nil
}

type prioritizingClient struct {
	Clients   []Client
	groupHash []byte
	group     *key.Group
}

// Get returns a the randomness at `round` or an error.
func (p *prioritizingClient) Get(ctx context.Context, round uint64) (res Result, err error) {
	for i, c := range p.Clients {
		res, err = c.Get(ctx, round)
		if err == nil {
			// previous clients failed. move them to end of priority.
			if i > 0 {
				p.Clients = append(p.Clients[i:], p.Clients[:i]...)
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

	for _, c := range p.Clients {
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
	if p.group == nil {
		if err := p.learnGroup(ctx); err != nil {
			log.DefaultLogger.Warn("prioritizing_client", "failed to learn group", "err", err)
			ch := make(chan Result, 0)
			close(ch)
			return ch
		}
	}
	return pollingWatcher(ctx, p, p.group, log.DefaultLogger)
}

// RoundAt will return the most recent round of randomness that will be available
// at time for the current client.
func (p *prioritizingClient) RoundAt(time time.Time) uint64 {
	return p.Clients[0].RoundAt(time)
}
