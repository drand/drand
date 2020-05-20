package client

import (
	"context"
	"time"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
)

// pollingWatcher generalizes the `Watch` interface for clients which learn new values
// by asking for them once each group period.
func pollingWatcher(ctx context.Context, client Client, group *key.Group, log log.Logger) <-chan Result {
	ch := make(chan Result, 1)
	r := client.RoundAt(time.Now())
	val, err := client.Get(ctx, r)
	if err != nil {
		log.Error("polling_client", "failed to watch", "err", err)
		close(ch)
		return ch
	}
	ch <- val

	go func() {
		defer close(ch)

		// Initially, wait to synchronize to the round boundary.
		_, nextTime := beacon.NextRound(time.Now().Unix(), group.Period, group.GenesisTime)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(nextTime-time.Now().Unix()) * time.Second):
		}

		r, err := client.Get(ctx, client.RoundAt(time.Now()))
		if err == nil {
			ch <- r
		} else {
			log.Error("polling_client", "failed to watch", "err", err)
		}

		// Then tick each period.
		t := time.NewTicker(group.Period)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				r, err := client.Get(ctx, client.RoundAt(time.Now()))
				if err == nil {
					ch <- r
				} else {
					log.Error("polling_client", "failed to watch", "err", err)
				}
				// TODO: keep trying on errors?
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch
}
