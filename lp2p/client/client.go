package client

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/proto"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/log"
	"github.com/drand/drand/lp2p"
	"github.com/drand/drand/protobuf/drand"
)

// Client is a concrete pubsub client implementation
type Client struct {
	cancel func()
	latest uint64
	cache  client.Cache
	log    log.Logger

	subs struct {
		sync.Mutex
		id uint64
		M  map[uint64]chan client.RandomData
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
//
//nolint:funlen // working as intended
func NewWithPubsub(ps *pubsub.PubSub, info *chain.Info, cache client.Cache) (*Client, error) {
	if info == nil {
		return nil, xerrors.Errorf("No chain supplied for joining")
	}

	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		cancel: cancel,
		cache:  cache,
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

	c.subs.M = make(map[uint64]chan client.RandomData)

	go func() {
		defer func() {
			c.log.Infow("removing all subs from pubsub")
			defer c.log.Infow("finished removing all subs from pubsub")
			c.subs.Lock()
			for _, ch := range c.subs.M {
				close(ch)
			}
			c.subs.M = make(map[uint64]chan client.RandomData)
			c.subs.Unlock()
			_ = t.Close()
			s.Cancel()
		}()

		for {
			select {
			case <-ctx.Done():
				err := ctx.Err()
				if err != nil {
					c.log.Debugw("subscription.Next() stopped", "err", err)
					return
				}
			default:
				c.log.Infow("calling subscription.Next()")
				msg, err := s.Next(ctx)
				c.log.Infow("finished subscription.Next()")

				if err != nil {
					c.log.Warnw("", "gossip client", "topic.Next error", "err", err)
					continue
				}
				var rand drand.PublicRandResponse
				err = proto.Unmarshal(msg.Data, &rand)
				if err != nil {
					c.log.Warnw("", "gossip client", "unmarshal random error", "err", err)
					continue
				}

				// TODO: verification, need to pass drand network public key in

				rnd := client.FromPublicRandResponse(&rand)
				if c.latest >= rnd.Rnd {
					continue
				}
				c.latest = rnd.Rnd

				c.subs.Lock()
				for _, ch := range c.subs.M {
					select {
					case ch <- rnd:
						c.log.Warnw("", "gossip client", "randomness notification sent to channel")
					default:
						c.log.Warnw("", "gossip client", "randomness notification dropped due to a full channel")
					}
				}
				c.subs.Unlock()
			}
		}
	}()

	return c, nil
}

// UnsubFunc is a cancel function for pubsub subscription
type UnsubFunc func()

// Sub subscribes to notfications about new randomness.
// Client instance owns the channel after it is passed to Sub function,
// thus the channel should not be closed by library user
//
// It is recommended to use a buffered channel. If the channel is full,
// notification about randomness will be dropped.
//
// Notification channels will be closed when the client is Closed
func (c *Client) Sub(ch chan client.RandomData) UnsubFunc {
	c.subs.Lock()
	c.subs.id++
	id := c.subs.id
	c.subs.M[id] = ch
	fmt.Printf("added subscription with ID: %d\n", id)
	c.subs.Unlock()
	return func() {
		fmt.Printf("removing subscription with ID: %d\n", id)
		defer fmt.Printf("removed subscription with ID: %d\n", id)

		c.subs.Lock()
		delete(c.subs.M, id)
		close(ch)
		c.subs.Unlock()
	}
}

// Watch implements the client.Watcher interface
func (c *Client) Watch(ctx context.Context) <-chan client.Result {
	innerCh := make(chan client.RandomData)
	outerCh := make(chan client.Result)
	end := c.Sub(innerCh)
	cache := c.cache
	if cache == nil {
		cache = &client.NilCache{}
	}

	w := sync.WaitGroup{}
	w.Add(1)

	go func() {
		defer close(outerCh)
		fmt.Println("releasing lock for Watch command")
		w.Done()

		fmt.Println("starting client.Watch loop")
		for {
			select {
			case resp, ok := <-innerCh:
				fmt.Println("message received on client.Watch loop")
				if !ok {
					c.log.Debugw("closed innerCh operation")
					return
				}

				fmt.Printf("adding new message for round %d to cache\n", resp.Rnd)
				c.cache.Add(resp.Rnd, resp)

				select {
				case outerCh <- resp:
					c.log.Debug("processed random message")
				default:
					c.log.Warnw("", "gossip client", "randomness notification dropped due to a full channel")
				}
			case <-ctx.Done():
				c.log.Debugw("client.Watch done")
				end()
				// drain leftover on innerCh
				for range innerCh {
				}
				c.log.Debugw("client.Watch finished draining the innerCh")
				return
			}
		}
	}()

	w.Wait()
	c.log.Infow("releasing control from client.Watch back to caller")

	return outerCh
}

// Close stops Client, cancels PubSub subscription and closes the topic.
func (c *Client) Close() error {
	c.cancel()
	return nil
}

// TODO: New for users without libp2p already running
