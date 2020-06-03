package client

import (
	"context"
	"errors"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
)

// defaultTTL is the time a RTT sample will live before it is tested again.
const defaultTTL = time.Minute * 5

// NewOptimizingClient creates a drand client that measures the speed of clients
// and uses the fastest one. Speeds are measured as the optimising client is
// used, so there is no background routine and no additional calls to other
// clients.
//
// A speed measurement lives for a certain period before it is reset. The next
// call to `.Get` will then attempt to use a reset client even if it was
// previously considered to be slower. Pass a value > 0 for `rttTTL` to
// customise the period - it defaults to 5 minutes.
//
// If a client fails to return a value it'll be attempted from the next fastest
// client. Failed clients are given a large RTT and are moved to the back of the
// list. This RTT expires as usual and allows failed clients to be re-introduced
// and become the fastest again. i.e. failed clients are not taken out of the
// list, just considered to be very slow.
func NewOptimizingClient(clients []Client, chainInfo *chain.Info, rttTTL time.Duration) (Client, error) {
	if len(clients) == 0 {
		return nil, errors.New("missing clients")
	}
	if chainInfo == nil {
		return nil, errors.New("missing chain info")
	}
	stats := make([]*clientStat, len(clients))
	now := time.Now()
	for i, c := range clients {
		stats[i] = &clientStat{client: c, rtt: 0, ts: now}
	}
	if rttTTL <= 0 {
		rttTTL = defaultTTL
	}
	return &optimizingClient{stats: stats, rttTTL: rttTTL, chainInfo: chainInfo, log: log.DefaultLogger}, nil
}

type optimizingClient struct {
	sync.RWMutex
	stats     []*clientStat
	rttTTL    time.Duration
	chainInfo *chain.Info
	log       log.Logger
}

type clientStat struct {
	client Client
	rtt    time.Duration
	ts     time.Time
}

// SetLog configures the client log output
func (oc *optimizingClient) SetLog(l log.Logger) {
	oc.log = l
}

// Get returns a the randomness at `round` or an error.
func (oc *optimizingClient) Get(ctx context.Context, round uint64) (res Result, err error) {
	oc.RLock()
	// take a copy of the current client stats so we iterate over a stable slice
	currStats := make([]*clientStat, len(oc.stats))
	copy(currStats, oc.stats)
	oc.RUnlock()

	nextStats := []*clientStat{}
	for _, s := range currStats {
		start := time.Now()
		res, err = s.client.Get(ctx, round)
		rtt := time.Now().Sub(start)

		// client failure, set a large RTT so it is sent to the back of the list
		if err != nil {
			rtt = math.MaxInt64
		}

		nextStats = append(nextStats, &clientStat{s.client, rtt, start})

		if err == nil {
			break
		}

		// context deadline hit
		if ctx.Err() != nil {
			return
		}
	}

	oc.updateStats(nextStats)
	return
}

func (oc *optimizingClient) updateStats(samples []*clientStat) {
	oc.Lock()
	defer oc.Unlock()

	// update the round trip times with new samples
	for _, next := range samples {
		for _, curr := range oc.stats {
			if curr.client == next.client {
				if curr.ts.Before(next.ts) {
					curr.rtt = next.rtt
					curr.ts = next.ts
				}
				break
			}
		}
	}

	// update expired round trip times
	now := time.Now()
	for _, curr := range oc.stats {
		if now.Sub(curr.ts) >= oc.rttTTL {
			curr.rtt = 0
			curr.ts = now
		}
	}

	// sort by fastest
	sort.Slice(oc.stats, func(i, j int) bool {
		return oc.stats[i].rtt < oc.stats[j].rtt
	})
}

// Watch returns new randomness as it becomes available.
func (oc *optimizingClient) Watch(ctx context.Context) <-chan Result {
	return pollingWatcher(ctx, oc, oc.chainInfo, oc.log)
}

// RoundAt will return the most recent round of randomness that will be available
// at time for the current client.
func (oc *optimizingClient) RoundAt(time time.Time) uint64 {
	return oc.stats[0].client.RoundAt(time)
}
