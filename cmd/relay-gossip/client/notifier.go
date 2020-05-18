package client

import (
	"context"
	"sync"
	"time"

	"github.com/drand/drand/beacon"
	dclient "github.com/drand/drand/client"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
	"github.com/gogo/protobuf/proto"
	lru "github.com/hashicorp/golang-lru"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

type roundTracker struct {
	sync.Mutex
	current uint64
}

func (rt *roundTracker) Get() uint64 {
	rt.Lock()
	defer rt.Unlock()
	return rt.current
}

func (rt *roundTracker) Set(r uint64) bool {
	rt.Lock()
	defer rt.Unlock()
	if rt.current >= r {
		return false
	}
	rt.current = r
	return true
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

// NewNotifier creates a new GetNotifierFunc that notifies when new drand pubsub messages are received for the given topic.
func NewNotifier(topic *pubsub.Topic, log log.Logger) dclient.GetNotifierFunc {
	return newNotifier(topic, &roundTracker{}, log)
}

// NewFailoverNotifier creates a new GetNotifierFunc with failover to getting new randomness over HTTP after the passed grace period.
func NewFailoverNotifier(topic *pubsub.Topic, gracePeriod time.Duration, log log.Logger) dclient.GetNotifierFunc {
	latestRound := &roundTracker{}
	getNotifier := newNotifier(topic, latestRound, log)

	return func(ctx context.Context, client dclient.Client, group *key.Group) <-chan dclient.Result {
		ch := make(chan dclient.Result, 5)
		psNotifier := getNotifier(ctx, client, group)
		sendResult := func(r dclient.Result) {
			select {
			case ch <- r:
			default:
				log.Warn("randomness notification dropped due to a full channel")
			}
		}

		go func() {
			var t *time.Timer
			defer func() {
				t.Stop()
				close(ch)
			}()

			for {
				_, nextTime := beacon.NextRound(time.Now().Unix(), group.Period, group.GenesisTime)
				remPeriod := time.Duration(nextTime-time.Now().Unix()) * time.Second
				t = time.NewTimer(remPeriod + gracePeriod)

				select {
				case res, ok := <-psNotifier:
					if !ok {
						return
					}
					t.Stop()
					sendResult(res)
				case <-t.C:
					res, err := client.Get(ctx, 0)
					if ctx.Err() != nil {
						return
					}
					if err != nil {
						log.Warn("failed HTTP failover:", err)
						continue
					}
					ok := latestRound.Set(res.Round())
					if !ok {
						continue // Not the latest round we've seen
					}
					sendResult(res)
				case <-ctx.Done():
					return
				}
			}
		}()

		return ch
	}
}

func newNotifier(topic *pubsub.Topic, latestRound *roundTracker, log log.Logger) dclient.GetNotifierFunc {
	return func(ctx context.Context, client dclient.Client, group *key.Group) <-chan dclient.Result {
		ch := make(chan dclient.Result, 5)
		s, err := topic.Subscribe()
		if err != nil {
			log.Error("topic.Subscribe error:", err)
			close(ch)
			return ch
		}

		go func() {
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
					log.Warn("subscription.Next error:", err)
					continue
				}
				var rand drand.PublicRandResponse
				err = proto.Unmarshal(msg.Data, &rand)
				if err != nil {
					log.Warn("unmarshaling randomness:", err)
					continue
				}

				// TODO: verification, need to pass drand network public key in

				res := &result{res: rand}
				ok := latestRound.Set(res.Round())
				if !ok {
					continue // Not the latest round we've seen
				}

				// Cache this value if we have a caching client
				c, ok := client.(interface{ Cache() *lru.ARCCache })
				if ok {
					c.Cache().Add(res.Round(), res)
				}

				select {
				case ch <- res:
				default:
					log.Warn("randomness notification dropped due to a full channel")
				}
			}
		}()

		return ch
	}
}
