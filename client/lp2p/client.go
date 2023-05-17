package lp2p

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"

	clock "github.com/jonboulle/clockwork"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"google.golang.org/protobuf/proto"

	client2 "github.com/drand/drand/client"
	"github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/client"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/internal/lp2p"
	"github.com/drand/drand/protobuf/drand"
)

// Client is a concrete pubsub client implementation
type Client struct {
	cancel     func()
	latest     uint64
	cache      client2.Cache
	bufferSize int
	log        log.Logger

	subs struct {
		sync.Mutex
		M map[*int]chan drand.PublicRandResponse
	}
}

// DefaultBufferSize controls how many incoming messages can be in-flight until they start
// to be dropped by the library
const DefaultBufferSize = 100

// SetLog configures the client log output
func (c *Client) SetLog(l log.Logger) {
	c.log = l
}

// WithPubsub provides an option for integrating pubsub notification
// into a drand client.
func WithPubsub(l log.Logger, ps *pubsub.PubSub, clk clock.Clock, bufferSize int) client2.Option {
	return client2.WithWatcher(func(info *chain.Info, cache client2.Cache) (client2.Watcher, error) {
		c, err := NewWithPubsub(l, ps, info, cache, clk, bufferSize)
		if err != nil {
			return nil, err
		}
		return c, nil
	})
}

// NewWithPubsub creates a gossip randomness client.
//
//nolint:funlen,lll // This is the correct function length
func NewWithPubsub(l log.Logger, ps *pubsub.PubSub, info *chain.Info, cache client2.Cache, clk clock.Clock, bufferSize int) (*Client, error) {
	if info == nil {
		return nil, fmt.Errorf("no chain supplied for joining")
	}

	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		cancel:     cancel,
		cache:      cache,
		bufferSize: bufferSize,
		log:        l,
	}

	chainHash := hex.EncodeToString(info.Hash())
	topic := lp2p.PubSubTopic(chainHash)
	if err := ps.RegisterTopicValidator(topic, randomnessValidator(info, cache, c, clk)); err != nil {
		cancel()
		return nil, fmt.Errorf("creating topic: %w", err)
	}
	t, err := ps.Join(topic)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("joining pubsub: %w", err)
	}
	s, err := t.Subscribe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("subscribe: %w", err)
	}

	c.subs.M = make(map[*int]chan drand.PublicRandResponse)

	go func() {
		for {
			msg, err := s.Next(ctx)
			if ctx.Err() != nil {
				c.log.Debugw("NewPubSub closing because context was canceled", "msg", msg, "err", ctx.Err())

				s.Cancel()
				err := t.Close()
				if err != nil {
					c.log.Errorw("NewPubSub closing goroutine for topic", "err", err)
				}

				c.subs.Lock()
				for _, ch := range c.subs.M {
					close(ch)
				}
				c.subs.M = make(map[*int]chan drand.PublicRandResponse)
				c.subs.Unlock()
				return
			}
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

			if c.latest >= rand.Round {
				c.log.Debugw("received round older than the latest previously received one", "latest", c.latest, "round", rand.Round)
				continue
			}
			c.latest = rand.Round

			c.log.Debugw("newPubSub broadcasting round to listeners", "round", rand.Round)
			c.subs.Lock()
			for _, ch := range c.subs.M {
				select {
				case ch <- rand:
				default:
					c.log.Warnw("", "gossip client", "randomness notification dropped due to a full channel")
				}
			}
			c.subs.Unlock()
			c.log.Debugw("newPubSub finished broadcasting round to listeners", "round", rand.Round)
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
func (c *Client) Sub(ch chan drand.PublicRandResponse) UnsubFunc {
	id := new(int)
	c.subs.Lock()
	c.subs.M[id] = ch
	c.subs.Unlock()
	return func() {
		c.log.Debugw("closing sub")
		c.subs.Lock()
		delete(c.subs.M, id)
		close(ch)
		c.subs.Unlock()
	}
}

// Watch implements the client.Watcher interface
func (c *Client) Watch(ctx context.Context) <-chan client.Result {
	innerCh := make(chan drand.PublicRandResponse, c.bufferSize)
	outerCh := make(chan client.Result, c.bufferSize)
	end := c.Sub(innerCh)

	w := sync.WaitGroup{}
	w.Add(1)

	go func() {
		defer close(outerCh)

		w.Done()

		for {
			select {
			// TODO: do not copy by assignment any drand.PublicRandResponse
			case resp, ok := <-innerCh: //nolint:govet
				if !ok {
					return
				}
				dat := &client2.RandomData{
					Rnd:               resp.Round,
					Random:            resp.Randomness,
					Sig:               resp.Signature,
					PreviousSignature: resp.PreviousSignature,
				}
				if c.cache != nil {
					c.cache.Add(resp.Round, dat)
				}
				select {
				case outerCh <- dat:
					c.log.Debugw("processed random beacon", "round", dat.Round())
				default:
					c.log.Warnw("", "gossip client", "randomness notification dropped due to a full channel", "round", dat.Round())
				}
			case <-ctx.Done():
				c.log.Debugw("client.Watch done")
				end()
				// drain leftover on innerCh
				for range innerCh { //nolint:revive
				}
				c.log.Debugw("client.Watch finished draining the innerCh")
				return
			}
		}
	}()

	w.Wait()

	return outerCh
}

// Close stops Client, cancels PubSub subscription and closes the topic.
func (c *Client) Close() error {
	c.cancel()
	return nil
}

// NewPubsub constructs a basic libp2p pubsub module for use with the drand client.
func NewPubsub(ctx context.Context, listenAddr, relayAddr string) (*pubsub.PubSub, error) {
	h, err := libp2p.New(libp2p.ListenAddrStrings(listenAddr))
	if err != nil {
		return nil, err
	}

	relayMa, err := multiaddr.NewMultiaddr(relayAddr)
	if err != nil {
		return nil, err
	}

	relayAi, err := peer.AddrInfoFromP2pAddr(relayMa)
	if err != nil {
		return nil, err
	}

	dps := []peer.AddrInfo{*relayAi}
	return pubsub.NewGossipSub(ctx, h, pubsub.WithDirectPeers(dps))
}
