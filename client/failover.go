package client

import (
	"context"
	"sync"
	"time"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
)

const defaultFailoverGracePeriod = time.Second * 5

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

// NewFailoverWatcher creates a client whose Watch function will failover to
// Get-ing new randomness if it does not receive it after the passed grace period.
func NewFailoverWatcher(core Client, group *key.Group, gracePeriod time.Duration, l log.Logger) Client {
	if gracePeriod == 0 {
		gracePeriod = defaultFailoverGracePeriod
	}

	return &failoverWatcher{
		Client:      core,
		group:       group,
		gracePeriod: gracePeriod,
		log:         l,
	}
}

type failoverWatcher struct {
	Client
	group       *key.Group
	gracePeriod time.Duration
	log         log.Logger
}

// Watch returns new randomness as it becomes available.
func (c *failoverWatcher) Watch(ctx context.Context) <-chan Result {
	latestRound := &roundTracker{}
	ch := make(chan Result, 5)

	sendResult := func(r Result) {
		select {
		case ch <- r:
		default:
			c.log.Warn("failover_client", "randomness notification dropped due to a full channel")
		}
	}

	go func() {
		watchC := c.Client.Watch(ctx)
		var t *time.Timer
		defer func() {
			t.Stop()
			close(ch)
		}()

		for {
			_, nextTime := beacon.NextRound(time.Now().Unix(), c.group.Period, c.group.GenesisTime)
			remPeriod := time.Duration(nextTime-time.Now().Unix()) * time.Second
			t = time.NewTimer(remPeriod + c.gracePeriod)

			select {
			case res, ok := <-watchC:
				if !ok {
					return
				}
				t.Stop()
				sendResult(res)
			case <-t.C:
				res, err := c.Get(ctx, 0)
				if ctx.Err() != nil {
					return
				}
				if err != nil {
					c.log.Warn("failover_client", "failed to failover", "error", err)
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
