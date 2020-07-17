package client

import (
	"context"
	"encoding/hex"
	"sync"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/log"
	"github.com/drand/drand/lp2p"
	"github.com/drand/drand/protobuf/drand"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/proto"
)

// Client is a concrete pubsub client implementation
type Client struct {
	cancel func()
	latest uint64
	log    log.Logger

	subs struct {
		sync.Mutex
		M map[*int]chan drand.PublicRandResponse
	}
}

// SetLog configures the client log output
func (c *Client) SetLog(l log.Logger) {
	c.log = l
}

// WithPubsub provides an option for integrating pubsub notification
// into a drand client.
func WithPubsub(ps *pubsub.PubSub) client.Option {
	return client.WithWatcher(func(info *chain.Info, cache client.Cache) (client.Watcher, error) {
		c, err := NewWithPubsub(ps, info, cache)
		if err != nil {
			return nil, err
		}
		return c, nil
	})
}

// NewWithPubsub creates a gossip randomness client.
func NewWithPubsub(ps *pubsub.PubSub, info *chain.Info, cache client.Cache) (*Client, error) {
	if info == nil {
		return nil, xerrors.Errorf("No chain supplied for joining")
	}

	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		cancel: cancel,
		log:    log.DefaultLogger(),
	}

	chainHash := hex.EncodeToString(info.Hash())
	topic := lp2p.PubSubTopic(chainHash)
	if err := ps.RegisterTopicValidator(topic, randomnessValidator(info, cache, c)); err != nil {
		cancel()
		return nil, xerrors.Errorf("creating topic: %w", err)
	}
	t, err := ps.Join(topic)
	if err != nil {
		cancel()
		return nil, xerrors.Errorf("joining pubsub: %w", err)
	}
	s, err := t.Subscribe()
	if err != nil {
		cancel()
		return nil, xerrors.Errorf("subscribe: %w", err)
	}

	c.subs.M = make(map[*int]chan drand.PublicRandResponse)

	go func() {
		for {
			msg, err := s.Next(ctx)
			if ctx.Err() != nil {
				c.subs.Lock()
				for _, ch := range c.subs.M {
					close(ch)
				}
				c.subs.M = make(map[*int]chan drand.PublicRandResponse)
				c.subs.Unlock()
				t.Close()
				s.Cancel()
				return
			}
			if err != nil {
				c.log.Warn("gossip client", "topic.Next error", "err", err)
				continue
			}
			var rand drand.PublicRandResponse
			err = proto.Unmarshal(msg.Data, &rand)
			if err != nil {
				c.log.Warn("gossip client", "unmarshal random error", "err", err)
				continue
			}

			// TODO: verification, need to pass drand network public key in

			if c.latest >= rand.Round {
				continue
			}
			c.latest = rand.Round

			c.subs.Lock()
			for _, ch := range c.subs.M {
				select {
				case ch <- rand:
				default:
					c.log.Warn("gossip client", "randomness notification dropped due to a full channel")
				}
			}
			c.subs.Unlock()
		}
	}()

	return c, nil
}

// UnsubFunc is a cancel function for pubsub subscription
type UnsubFunc func()

// Sub subscribes to notfications about new randomness.
// Client instnace owns the channel after it is passed to Sub function,
// thus the channel should not be closed by library user
//
// It is recommended to use a buffered channel. If the channel is full,
// notification about randomness will be dropped.
//
// Notification channels will be closed when the client is Closed
func (c *Client) Sub(ch chan drand.PublicRandResponse) UnsubFunc {
	id := new(int)
	c.subs.Lock()
	c.subs.M[id] = ch
	c.subs.Unlock()
	return func() {
		c.subs.Lock()
		delete(c.subs.M, id)
		close(ch)
		c.subs.Unlock()
	}
}

// Watch implements the client.Watcher interface
func (c *Client) Watch(ctx context.Context) <-chan client.Result {
	innerCh := make(chan drand.PublicRandResponse)
	outerCh := make(chan client.Result)
	end := c.Sub(innerCh)

	go func() {
		for {
			select {
			case resp, ok := <-innerCh:
				if !ok {
					close(outerCh)
					return
				}
				select {
				case outerCh <- &client.RandomData{
					Rnd:               resp.Round,
					Random:            resp.Randomness,
					Sig:               resp.Signature,
					PreviousSignature: resp.PreviousSignature,
				}:
				default:
					c.log.Warn("gossip client", "randomness notification dropped due to a full channel")
				}
			case <-ctx.Done():
				close(outerCh)
				end()
				// drain leftover on innerCh
				for range innerCh {
				}
				return
			}
		}
	}()

	return outerCh
}

// Close stops Client, cancels PubSub subscription and closes the topic.
func (c *Client) Close() error {
	c.cancel()
	return nil
}

// TODO: New for users without libp2p already running
