package client

import (
	"context"
	"errors"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
)

// NewPrioritizingClient is a client combinator that asks sub-clients
// in succession until an answer is found.
// Get requests are sourced from get sub-clients.
// Watches are achieved as a long-poll from the prioritized get sub-clients.
func NewPrioritizingClient(clients []Client, chainHash []byte, chainInfo *chain.Info) (Client, error) {
	return &prioritizingClient{clients, chainHash, chainInfo, log.DefaultLogger}, nil
}

type prioritizingClient struct {
	clients   []Client
	chainHash []byte
	chainInfo *chain.Info
	log       log.Logger
}

// SetLog configures the client log output
func (p *prioritizingClient) SetLog(l log.Logger) {
	p.log = l
}

// Get returns a the randomness at `round` or an error.
func (p *prioritizingClient) Get(ctx context.Context, round uint64) (res Result, err error) {
	for i, c := range p.clients {
		res, err = c.Get(ctx, round)
		if err == nil {
			// previous clients failed. move them to end of priority.
			if i > 0 {
				p.clients = append(p.clients[i:], p.clients[:i]...)
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
	var chainInfo *chain.Info
	var err error

	for _, c := range p.clients {
		if hc, ok := c.(*httpClient); ok {
			chainInfo, err = hc.FetchChainInfo(p.chainHash)
			if err == nil {
				p.chainInfo = chainInfo
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
	// otherwise, poll from the prioritized list of getClients
	if p.chainInfo == nil {
		if err := p.learnGroup(ctx); err != nil {
			log.DefaultLogger.Warn("prioritizing_client", "failed to learn group", "err", err)
			ch := make(chan Result, 0)
			close(ch)
			return ch
		}
	}
	return pollingWatcher(ctx, p, p.chainInfo, p.log)
}

// Info returns information about the chain.
func (p *prioritizingClient) Info(ctx context.Context) (*chain.Info, error) {
	if p.chainInfo != nil {
		return p.chainInfo, nil
	}
	if err := p.learnGroup(ctx); err != nil {
		return nil, err
	}
	return p.chainInfo, nil
}

// RoundAt will return the most recent round of randomness that will be available
// at time for the current client.
func (p *prioritizingClient) RoundAt(time time.Time) uint64 {
	return p.clients[0].RoundAt(time)
}
