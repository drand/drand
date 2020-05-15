package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/drand/drand/beacon"
	dclient "github.com/drand/drand/client"
	"github.com/drand/drand/cmd/relay-gossip/lp2p"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
	"github.com/gogo/protobuf/proto"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"golang.org/x/xerrors"
)

var (
	// ErrNotAvailable is returned when Get is called.
	ErrNotAvailable = fmt.Errorf("not available")
)

type Client struct {
	group  *key.Group
	cancel func()
	latest uint64

	subs struct {
		sync.Mutex
		M map[*int]chan drand.PublicRandResponse
	}
}

// NewWithPubsub creates a new drand client that uses pubsub to receive randomness updates
func NewWithPubsub(ps *pubsub.PubSub, groupHash []byte, log log.Logger) (*Client, error) {
	t, err := ps.Join(lp2p.PubSubTopic(string(groupHash)))
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
				log.Warn("topic.Next error:", err)
				continue
			}
			var rand drand.PublicRandResponse
			err = proto.Unmarshal(msg.Data, &rand)
			if err != nil {
				log.Warn("unmarshaling randomness:", err)
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

// Get returns a the randomness at `round` or an error.
func (c *Client) Get(ctx context.Context, round uint64) (dclient.Result, error) {
	return nil, ErrNotAvailable
}

type result struct {
	res drand.PublicRandResponse
}

func (r *result) Round() uint64 {
	return r.res.Round
}

func (r *result) Randomness() []byte {
	return r.res.Randomness
}

// Watch returns new randomness as it becomes available.
func (c *Client) Watch(ctx context.Context) <-chan dclient.Result {
	crC := make(chan dclient.Result)
	drC := make(chan drand.PublicRandResponse, 5)
	unsub := c.sub(drC)

	go func() {
		defer func() {
			unsub()
			close(crC)
		}()

		for {
			select {
			case prr, ok := <-drC:
				if !ok {
					return
				}
				select {
				case crC <- &result{res: prr}:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return crC
}

// RoundAt will return the most recent round of randomness that will be available
// at time for the current client.
func (c *Client) RoundAt(time time.Time) uint64 {
	return beacon.CurrentRound(time.Unix(), c.group.Period, c.group.GenesisTime)
}

type unsubFunc func()

// sub subscribes to notfications about new randomness.
// Client instnace owns the channel after it is passed to sub function,
// thus the channel should not be closed by library user
//
// It is recommended to use a buffered channel. If the channel is full,
// notification about randomness will be dropped.
func (c *Client) sub(ch chan drand.PublicRandResponse) unsubFunc {
	id := new(int)
	c.subs.Lock()
	c.subs.M[id] = ch
	c.subs.Unlock()

	return func() {
		c.subs.Lock()
		delete(c.subs.M, id)
		c.subs.Unlock()
	}
}

// Close stops Client, cancels PubSub subscription and closes the topic.
func (c *Client) Close() error {
	c.cancel()
	return nil
}

// TODO: New for users without libp2p already running
