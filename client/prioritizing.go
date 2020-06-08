package client

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
)

// NewPrioritizingClient is a client combinator that asks sub-clients
// in succession until an answer is found.
// Get requests are sourced from get sub-clients.
// Watches are achieved as a long-poll from the prioritized get sub-clients.
func NewPrioritizingClient(clients []Client, chainHash []byte, chainInfo *chain.Info) (Client, error) {
	var ir *infoResult
	if chainInfo != nil {
		ir = &infoResult{chainInfo, nil}
	}
	return &prioritizingClient{clients, chainHash, sync.RWMutex{}, ir, log.DefaultLogger}, nil
}

type infoResult struct {
	*chain.Info
	err error
}

type prioritizingClient struct {
	clients   []Client
	chainHash []byte
	infoMutex sync.RWMutex
	info      *infoResult
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
func (p *prioritizingClient) learnGroup(ctx context.Context) {
	var chainInfo *chain.Info
	var err error

	for _, c := range p.clients {
		chainInfo, err = c.Info(ctx)
		if err == nil {
			p.info = &infoResult{chainInfo, nil}
			return
		}
	}
	if err == nil {
		err = errors.New("No clients to learn group info from")
	}
	log.DefaultLogger.Warn("prioritizing_client", "failed to learn group", "err", err)
	p.info = &infoResult{nil, err}
}

// Watch returns new randomness as it becomes available.
func (p *prioritizingClient) Watch(ctx context.Context) <-chan Result {
	info, err := p.Info(ctx)
	if err != nil || info == nil {
		log.DefaultLogger.Warn("prioritizing_client", "failing watch due to lack of group")
		ch := make(chan Result, 0)
		close(ch)
		return ch
	}
	return PollingWatcher(ctx, p, info, p.log)
}

// Info returns information about the chain.
func (p *prioritizingClient) Info(ctx context.Context) (*chain.Info, error) {
	p.infoMutex.RLock()
	if p.info == nil {
		p.infoMutex.RUnlock()
		p.infoMutex.Lock()
		if p.info == nil {
			p.learnGroup(ctx)
		}
		p.infoMutex.Unlock()
	} else {
		p.infoMutex.RUnlock()
	}
	return p.info.Info, p.info.err
}

// RoundAt will return the most recent round of randomness that will be available
// at time for the current client.
func (p *prioritizingClient) RoundAt(time time.Time) uint64 {
	return p.clients[0].RoundAt(time)
}
