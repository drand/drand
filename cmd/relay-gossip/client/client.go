package client

import (
	"context"
	"encoding/hex"
	"sync"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	dclient "github.com/drand/drand/client"
	"github.com/drand/drand/cmd/relay-gossip/lp2p"
	"github.com/drand/drand/protobuf/drand"
	"github.com/gogo/protobuf/proto"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"golang.org/x/xerrors"
)

var (
	log = logging.Logger("drand-client")
)

// Client is a concrete pubsub client implementation
type Client struct {
	cancel func()
	latest uint64

	subs struct {
		sync.Mutex
		M map[*int]chan drand.PublicRandResponse
	}
}

// WithPubsub provides an option for integrating pubsub notification
// into a drand client.
func WithPubsub(ps *pubsub.PubSub) dclient.Option {
	return dclient.WithWatcher(func(info *chain.Info) (dclient.Watcher, error) {
		c, err := NewWithPubsub(ps, info)
		if err != nil {
			return nil, err
		}
		return c, nil
	})
}

func randomnessValidator(info *chain.Info) func(context.Context, peer.ID, *pubsub.Message) bool {
	return func(ctx context.Context, p peer.ID, m *pubsub.Message) bool {
		var rand drand.PublicRandResponse
		err := proto.Unmarshal(m.Data, &rand)
		if err != nil {
			return false
		}

		if info == nil {
			log.Warn("Not validating received randomness due to lack of trust root.")
			return true
		}

		b := chain.Beacon{
			Round:     rand.GetRound(),
			Signature: rand.GetSignature(),
		}

		if err := chain.VerifyBeacon(info.PublicKey, &b); err != nil {
			return false
		}
		return true
	}
}

// NewWithPubsub creates a gossip randomness client.
func NewWithPubsub(ps *pubsub.PubSub, info *chain.Info) (*Client, error) {
	chainHash := hex.EncodeToString(info.Hash())
	ps.RegisterTopicValidator(chainHash, randomnessValidator(info), pubsub.WithValidatorInline(true))
	t, err := ps.Join(lp2p.PubSubTopic(chainHash))
	if err != nil {
		return nil, xerrors.Errorf("joining pubsub: %w", err)
	}
	s, err := t.Subscribe()
	if err != nil {
		return nil, xerrors.Errorf("subscribe: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		cancel: cancel,
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
				log.Warnf("topic.Next error: %+v", err)
				continue
			}
			var rand drand.PublicRandResponse
			err = proto.Unmarshal(msg.Data, &rand)
			if err != nil {
				log.Warnf("unmarshaling randomness: %+v", err)
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
					log.Warn("randomness notification dropped due to a full channel")
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

// Watch implements the dclient.Watcher interface
func (c *Client) Watch(ctx context.Context) <-chan client.Result {
	innerCh := make(chan drand.PublicRandResponse)
	outerCh := make(chan dclient.Result)
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
				case outerCh <- &result{resp.Round, resp.Randomness, resp.Signature}:
				default:
					log.Warn("randomness notification dropped due to a full channel")
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

type result struct {
	round      uint64
	randomness []byte
	signature  []byte
}

func (r *result) Round() uint64 {
	return r.round
}

func (r *result) Randomness() []byte {
	return r.randomness
}

func (r *result) Signature() []byte {
	return r.signature
}

// Close stops Client, cancels PubSub subscription and closes the topic.
func (c *Client) Close() error {
	c.cancel()
	return nil
}

// TODO: New for users without libp2p already running
