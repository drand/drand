package client

import (
	"context"
	"sync"

	gossip "github.com/drand/drand/cmd/relay-gossip/client"
	drand "github.com/drand/drand/protobuf/drand"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

type gossipClient struct {
	Client
	gossip   *gossip.Client
	in       chan drand.PublicRandResponse
	unsub    gossip.UnsubFunc
	wlk      sync.Mutex
	watchers map[*gossipWatcher]struct{}
}

type gossipWatcher struct {
	ctx      context.Context
	resultCh chan Result
}

// NewGossipClient ...
func NewGossipClient(core Client, ps *pubsub.PubSub, network string) (Client, error) {
	g, err := gossip.NewWithPubsub(ps, network)
	if err != nil {
		return nil, err
	}
	in := make(chan drand.PublicRandResponse, 0)
	unsub := g.Sub(in)
	c := &gossipClient{
		Client:   core,
		gossip:   g,
		in:       in,
		unsub:    unsub,
		watchers: map[*gossipWatcher]struct{}{},
	}
	go c.poll()
	return c, nil
}

type gossipResult struct {
	round      uint64
	randomness []byte
}

func (r *gossipResult) Round() uint64 {
	return r.round
}

func (r *gossipResult) Randomness() []byte {
	return r.randomness
}

func (c *gossipClient) poll() {
	for {
		select {
		case pr, ok := <-c.in:
			if !ok {
				return
			}
			c.wlk.Lock()
			for w := range c.watchers {
				if w.ctx.Err() == nil {
					w.resultCh <- &gossipResult{round: X, randomness: X}
				} else {
					close(w.resultCh)
					delete(c.watchers, w)
				}
			}
			c.wlk.Unlock()
		}
	}
}

func (c *gossipClient) Watch(ctx context.Context) <-chan Result {
	c.wlk.Lock()
	defer c.wlk.Unlock()
	ch := make(chan Result)
	c.watchers[&gossipWatcher{ctx: ctx, resultCh: ch}] = struct{}{}
	return ch
}
