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

const (
	defaultRequestTimeout    = time.Second * 5
	defaultSpeedTestInterval = time.Minute * 5
)

// NewOptimizingClient creates a drand client that measures the speed of clients
// and uses the fastest one. Speeds are measured as the optimizing client is
// used, so there is no background routine and no additional calls to other
// clients.
//
// A speed measurement lives for a certain period before it is reset. The next
// call to `.Get` will then attempt to use a reset client even if it was
// previously considered to be slower. Pass a value > 0 for `rttTTL` to
// customize the period - it defaults to 5 minutes.
//
// If a client fails to return a value it'll be attempted from the next fastest
// client. Failed clients are given a large RTT and are moved to the back of the
// list. This RTT expires as usual and allows failed clients to be re-introduced
// and become the fastest again. i.e. failed clients are not taken out of the
// list, just considered to be very slow.
func NewOptimizingClient(clients []Client, chainInfo *chain.Info, requestTimeout time.Duration, speedTestInterval time.Duration) (Client, error) {
	if len(clients) == 0 {
		return nil, errors.New("missing clients")
	}
	if chainInfo == nil {
		return nil, errors.New("missing chain info")
	}
	stats := make([]*requestStat, len(clients))
	now := time.Now()
	for i, c := range clients {
		stats[i] = &requestStat{client: c, rtt: 0, startTime: now}
	}
	done := make(chan struct{})
	if requestTimeout <= 0 {
		requestTimeout = defaultRequestTimeout
	}
	if speedTestInterval <= 0 {
		speedTestInterval = defaultSpeedTestInterval
	}
	c := &optimizingClient{
		clients:           clients,
		stats:             stats,
		requestTimeout:    requestTimeout,
		speedTestInterval: speedTestInterval,
		chainInfo:         chainInfo,
		log:               log.DefaultLogger,
		done:              done,
	}
	go c.testSpeed()
	return c, nil
}

type optimizingClient struct {
	sync.RWMutex
	clients           []Client
	stats             []*requestStat
	requestTimeout    time.Duration
	speedTestInterval time.Duration
	chainInfo         *chain.Info
	log               log.Logger
	done              chan struct{}
}

type requestStat struct {
	// client is the client used to make the request.
	client Client
	// rtt is the time it took to make the request.
	rtt time.Duration
	// startTime is the time at which the request was started.
	startTime time.Time
}

func (oc *optimizingClient) testSpeed() {
	for {
		nextStats := []*requestStat{}
		for _, c := range oc.clients {
			ctx, cancel := context.WithTimeout(context.Background(), oc.requestTimeout)
			ch := get(ctx, c, 1)

			select {
			case rr, ok := <-ch:
				cancel()
				if !ok {
					break
				}
				if rr.err != nil && ctx.Err() == nil {
					oc.log.Warn("optimising_client", "speed test request failed", "err", rr.err)
				}
				nextStats = append(nextStats, rr.stat)
			case <-ctx.Done():
			case <-oc.done:
				cancel()
				return
			}
		}

		oc.updateStats(nextStats)

		t := time.NewTimer(oc.speedTestInterval)
		select {
		case <-t.C:
		case <-oc.done:
			return
		}
	}
}

// requestResult records all the information about a call to Get.
type requestResult struct {
	res  Result
	err  error
	stat *requestStat
}

// get calls Get on the passed client and returns a channel that yields a single
// raceResult when the call completes.
func get(ctx context.Context, client Client, round uint64) <-chan *requestResult {
	ch := make(chan *requestResult, 1)
	go func() {
		start := time.Now()
		res, err := client.Get(ctx, round)
		rtt := time.Now().Sub(start)

		// client failure, set a large RTT so it is sent to the back of the list
		if err != nil {
			rtt = math.MaxInt64
		}

		stat := requestStat{client, rtt, start}

		if err != nil {
			ch <- &requestResult{nil, err, &stat}
		} else {
			ch <- &requestResult{res, nil, &stat}
		}
		close(ch)
	}()
	return ch
}

// SetLog configures the client log output
func (oc *optimizingClient) SetLog(l log.Logger) {
	oc.log = l
}

func (oc *optimizingClient) Close() {
	close(oc.done)
}

// Get returns a the randomness at `round` or an error.
func (oc *optimizingClient) Get(ctx context.Context, round uint64) (Result, error) {
	oc.RLock()
	// copy the current ordered client list so we iterate over a stable slice
	var clients []Client
	for i := range oc.stats {
		clients = append(clients, oc.stats[i].client)
	}
	oc.RUnlock()

	if len(clients) == 1 {
		ctx, cancel := context.WithTimeout(context.Background(), oc.requestTimeout)
		ch := get(ctx, clients[0], round)
		select {
		case rr, _ := <-ch:
			oc.updateStats([]*requestStat{rr.stat})
			if rr.err != nil {
				return nil, rr.err
			}
			return rr.res, nil
		case <-oc.done:
			cancel()
			return nil, errors.New("client closed")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), oc.requestTimeout)
	ch0 := get(ctx, clients[0], round)
	ch1 := get(ctx, clients[1], round)
	select {
	case rr, _ := <-ch0:
		oc.updateStats([]*requestStat{rr.stat})
		cancel()
		if rr.err != nil {
			return nil, rr.err
		}
		return rr.res, nil
	case rr, _ := <-ch1:
		oc.updateStats([]*requestStat{rr.stat})
		cancel()
		if rr.err != nil {
			return nil, rr.err
		}
		return rr.res, nil
	case <-oc.done:
		cancel()
		return nil, errors.New("client closed")
	}

	// TODO fallback to other clients
}

func (oc *optimizingClient) updateStats(samples []*requestStat) {
	oc.Lock()
	defer oc.Unlock()

	// update the round trip times with new samples
	for _, next := range samples {
		for _, curr := range oc.stats {
			if curr.client == next.client {
				if curr.startTime.Before(next.startTime) {
					curr.rtt = next.rtt
					curr.startTime = next.startTime
				}
				break
			}
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
