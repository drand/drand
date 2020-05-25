package client

import (
	"context"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
)

const defaultFailoverGracePeriod = time.Second * 5

// NewFailoverWatcher creates a client whose Watch function will failover to
// Get-ing new randomness if it does not receive it after the passed grace period.
//
// Note that this client may skip rounds in some cases: e.g. if the group halts
// for a bit and then catches up quickly, this could jump up to 'current round'
// and not emit the intermediate values.
func NewFailoverWatcher(core Client, chainInfo *chain.Info, gracePeriod time.Duration, l log.Logger) Client {
	if gracePeriod == 0 {
		gracePeriod = defaultFailoverGracePeriod
	}

	return &failoverWatcher{
		Client:      core,
		chainInfo:   chainInfo,
		gracePeriod: gracePeriod,
		log:         l,
	}
}

type failoverWatcher struct {
	Client
	chainInfo   *chain.Info
	gracePeriod time.Duration
	log         log.Logger
}

// Watch returns new randomness as it becomes available.
func (c *failoverWatcher) Watch(ctx context.Context) <-chan Result {
	var latest uint64
	ch := make(chan Result, 5)

	sendResult := func(r Result) {
		if latest >= r.Round() {
			c.log.Warn("failover_client", "randomness notification dropped: out of date", "round", r.Round(), "latest", latest)
			return
		}
		latest = r.Round()

		select {
		case ch <- r:
		default:
			c.log.Warn("failover_client", "randomness notification dropped: full channel")
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
			_, nextTime := chain.NextRound(time.Now().Unix(), c.chainInfo.Period, c.chainInfo.GenesisTime)
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
				sendResult(res)
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch
}
