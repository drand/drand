package failover

import (
	"context"
	"time"

	"github.com/drand/drand/beacon"
	dclient "github.com/drand/drand/client"
	pclient "github.com/drand/drand/cmd/drand-gossip-relay/client"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
)

// WatcherFunc is a function that returns a channel watching for new randomness.
type WatcherFunc func(ctx context.Context, client dclient.Client, group *key.Group) <-chan Result

// NewWatcher creates a new watcher that failsover to getting new randomness after the passed grace period.
func NewWatcher(pubsubClient *pclient.Client, gracePeriod time.Duration) WatcherFunc {
	return func(ctx context.Context, client dclient.Client, group *key.Group) <-chan Result {
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
			effectiveSlack := slack
			if group.Period < effectiveSlack {
				effectiveSlack = group.Period
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(nextTime-time.Now().Unix())*time.Second + effectiveSlack):
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
}
