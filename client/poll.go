package client

import (
	"context"
	"time"

	chain2 "github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/client"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/internal/chain"
)

// PollingWatcher generalizes the `Watch` interface for clients which learn new values
// by asking for them once each group period.
func PollingWatcher(ctx context.Context, c client.Client, chainInfo *chain2.Info, l log.Logger) <-chan client.Result {
	ch := make(chan client.Result, 1)
	r := c.RoundAt(time.Now())
	val, err := c.Get(ctx, r)
	if err != nil {
		l.Errorw("", "polling_client", "failed synchronous get", "from", c, "err", err)
		close(ch)
		return ch
	}
	ch <- val

	go func() {
		defer close(ch)

		// Initially, wait to synchronize to the round boundary.
		_, nextTime := chain.NextRound(time.Now().Unix(), chainInfo.Period, chainInfo.GenesisTime)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(nextTime-time.Now().Unix()) * time.Second):
		}

		r, err := c.Get(ctx, c.RoundAt(time.Now()))
		if err == nil {
			ch <- r
		} else {
			l.Errorw("", "polling_client", "failed first async get", "from", c, "err", err)
		}

		// Then tick each period.
		t := time.NewTicker(chainInfo.Period)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				r, err := c.Get(ctx, c.RoundAt(time.Now()))
				if err == nil {
					ch <- r
				} else {
					l.Errorw("", "polling_client", "failed subsequent watch poll", "from", c, "err", err)
				}
				// TODO: keep trying on errors?
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch
}
