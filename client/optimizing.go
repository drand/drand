package client

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
	"go.uber.org/atomic"
)

const headStart = time.Second

const (
	// defaultTTL is the time a RTT sample will live before it is tested again.
	defaultTTL = time.Minute * 5
	// defaultHeadStart is the time given to the fastest client before which we
	// race the others to `Get` a `Result`.
	defaultHeadStart = time.Second
)

// NewOptimizingClient creates a drand client that measures the speed of clients
// and uses the fastest one. Speeds are measured as the optimizing client is
// used, so there is no background routine and making additional calls to other
// clients.
//
// The optimizing client "learns" which client is the fastest by selecting a new
// client on each call to `.Get` until all clients have a speed measurment.
//
// A speed measurement lives for a certain period before it is reset. The next
// call to `.Get` will then attempt to use a reset client even if it was
// previously considered to be slower. Pass a value > 0 for `rttTTL` to
// customize the period - it defaults to 5 minutes.
//
// If the fastest client fails to return a value within a certain period (called
// the `headStart` period) then ALL the other clients are raced for the value.
//
// Failed clients are given a large RTT and are moved to the back of the list.
// This RTT expires as usual and allows failed clients to be re-introduced
// and become the fastest again. i.e. failed clients are not taken out of the
// list, just considered to be very slow.
func NewOptimizingClient(clients []Client, chainInfo *chain.Info, timeout time.Duration, headStart time.Duration, rttTTL time.Duration) (Client, error) {
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
	if headStart <= 0 {
		headStart = defaultHeadStart
	}
	if rttTTL <= 0 {
		rttTTL = defaultTTL
	}
	return &optimizingClient{stats: stats, rttTTL: rttTTL, chainInfo: chainInfo, log: log.DefaultLogger}, nil
}

type optimizingClient struct {
	sync.RWMutex
	stats     []*requestStat
	timeout   time.Duration
	headStart time.Duration
	rttTTL    time.Duration
	chainInfo *chain.Info
	log       log.Logger
}

type requestStat struct {
	// client is the client used to make the request.
	client Client
	// rtt is the time it took to make the request.
	rtt time.Duration
	// startTime is the time at which the request was started.
	startTime time.Time
}

// SetLog configures the client log output
func (oc *optimizingClient) SetLog(l log.Logger) {
	oc.log = l
}

// Get returns a the randomness at `round` or an error.
func (oc *optimizingClient) Get(ctx context.Context, round uint64) (Result, error) {
	// Set overall timeout for this Get request.
	if oc.timeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, oc.timeout)
		defer cancel()
	}

	oc.RLock()
	// copy the current ordered client list so we iterate over a stable slice
	var clients []Client
	for i := range oc.stats {
		clients = append(clients, oc.stats[i].client)
	}
	oc.RUnlock()

	// error stored from previous failures so last error can be returned if no
	// client is able to return a value
	var err error
	results := raceWithHeadStart(ctx, clients, round, oc.headStart)

	for {
		select {
		case rr, ok := <-results:
			if !ok {
				return nil, err // no clients were able to return values
			}

			oc.updateStat(rr.stat)

			if rr.err != nil {
				err = rr.err
				if ctx.Err() == nil { // error from a client, wait for next result
					oc.log.Warn("optimising_client", "get request failed", "err", rr.err)
				}
				continue
			}

			// we got a result!
			// consume the remainder of the racing requests in the background
			go func() {
				for {
					select {
					case rr, ok := <-results:
						if !ok {
							return
						}
						oc.updateStat(rr.stat)
					case <-ctx.Done():
						return
					}
				}
			}()

			return rr.res, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// raceResult records all the information about a call to Get.
type raceResult struct {
	res  Result
	err  error
	stat *requestStat
}

// raceWithHeadStart races the passed clients to get the Result for the given
// round. It gives the head of the list a 1 second headstart.
func raceWithHeadStart(ctx context.Context, clients []Client, round uint64, headStart time.Duration) <-chan *raceResult {
	results := make(chan *raceResult, len(clients))

	// Expect Get within headStart period and if not, race the others
	ch := get(ctx, clients[0], round)
	var rr *raceResult
	t := time.NewTimer(headStart)

	select {
	case rr, _ = <-ch:
		t.Stop()
	case <-t.C:
	}

	if rr != nil {
		results <- rr
		if rr.err == nil { // done!
			close(results)
			return results
		}
	}

	pending := atomic.NewUint64(uint64(len(clients)))
	forward := func(ch <-chan *raceResult) {
		for rr := range ch {
			results <- rr
		}
		if pending.Dec() == 0 {
			close(results)
		}
	}

	go forward(ch) // forward the result from the first client if it comes

	for _, c := range clients[1:] {
		go forward(get(ctx, c, round))
	}

	return results
}

// get calls Get on the passed client and returns a channel that yields a single
// raceResult when the call completes.
func get(ctx context.Context, client Client, round uint64) <-chan *raceResult {
	ch := make(chan *raceResult, 1)
	go func() {
		start := time.Now()
		res, err := client.Get(ctx, round)
		rtt := time.Now().Sub(start)
		stat := requestStat{client, rtt, start}

		if err != nil {
			ch <- &raceResult{nil, err, &stat}
		} else {
			ch <- &raceResult{res, nil, &stat}
		}
		close(ch)
	}()
	return ch
}

func (oc *optimizingClient) updateStat(sample *requestStat) {
	oc.Lock()
	defer oc.Unlock()

	// update the round trip times with new samples
	for _, curr := range oc.stats {
		if curr.client == sample.client {
			if curr.startTime.Before(sample.startTime) {
				curr.rtt = sample.rtt
				curr.startTime = sample.startTime
			}
			break
		}
	}

	// update expired round trip times
	now := time.Now()
	for _, curr := range oc.stats {
		if now.Sub(curr.startTime) >= oc.rttTTL {
			curr.rtt = 0
			curr.startTime = now
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
