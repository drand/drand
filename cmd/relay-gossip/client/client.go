package client

import (
	"context"
	"encoding/hex"

	dclient "github.com/drand/drand/client"
	"github.com/drand/drand/cmd/relay-gossip/lp2p"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
	"github.com/gogo/protobuf/proto"
	lru "github.com/hashicorp/golang-lru"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"golang.org/x/xerrors"
)

type gossipClient struct {
	dclient.Client
	group *key.Group
	topic *pubsub.Topic
	log   log.Logger
}

type gossipResult struct {
	res drand.PublicRandResponse
}

func (r *gossipResult) Round() uint64 {
	return r.res.Round
}

func (r *gossipResult) Randomness() []byte {
	return r.res.Randomness
}

// NewWithPubsub creates a new drand client that uses pubsub to receive randomness updates.
func NewWithPubsub(core dclient.Client, ps *pubsub.PubSub, g *key.Group, l log.Logger) (dclient.Client, error) {
	if l == nil {
		l = log.DefaultLogger
	}

	topicName := lp2p.PubSubTopic(hex.EncodeToString(g.Hash()))
	l.Info("client", "pubsub subscribe topic", "topic", topicName)

	t, err := ps.Join(topicName)
	if err != nil {
		return nil, xerrors.Errorf("joining pubsub: %w", err)
	}

	gc := gossipClient{Client: core, group: g, topic: t, log: l}
	return dclient.NewWatchAggregator(&gc, l), nil
}

// Watch returns new randomness as it becomes available.
func (c *gossipClient) Watch(ctx context.Context) <-chan dclient.Result {
	ch := make(chan dclient.Result, 5)

	s, err := c.topic.Subscribe()
	if err != nil {
		c.log.Error("relay-gossip", "topic.Subscribe", "error", err)
		close(ch)
		return ch
	}

	go func() {
		var latest uint64

		defer func() {
			s.Cancel()
			close(ch)
		}()

		for {
			msg, err := s.Next(ctx)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				c.log.Warn("relay-gossip", "subscription.Next", "error", err)
				continue
			}
			var rand drand.PublicRandResponse
			err = proto.Unmarshal(msg.Data, &rand)
			if err != nil {
				c.log.Warn("relay-gossip", "unmarshaling randomness", "error", err)
				continue
			}

			// TODO: verification, need to pass drand network public key in

			res := &gossipResult{res: rand}

			// Cache this value if we have a caching client
			cc, ok := c.Client.(interface{ Cache() *lru.ARCCache })
			if ok && !cc.Cache().Contains(res.Round()) {
				cc.Cache().Add(res.Round(), res)
			}

			if latest >= res.Round() {
				continue
			}
			latest = res.Round()

			select {
			case ch <- res:
			default:
				c.log.Warn("relay-gossip", "randomness notification dropped due to a full channel")
			}
		}
	}()

	return ch
}
