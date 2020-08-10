package client

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
	"github.com/hashicorp/go-multierror"
)

const (
	defaultRequestTimeout    = time.Second * 5
	defaultSpeedTestInterval = time.Minute * 5
	// defaultRequestConcurrency controls both how many clients are raced
	// when `Get` is called for on-demand results, and also how many watch
	// clients are spun up (in addition to clients marked as passive) to
	// provide results to `Watch` requests.
	defaultRequestConcurrency = 1
	// defaultWatchRetryInterval is the time after which a closed watch channel
	// is re-open when no context error occurred.
	defaultWatchRetryInterval = time.Second * 30
	defaultChannelBuffer      = 5
)

// newOptimizingClient creates a drand client that measures the speed of clients
// and uses the fastest ones.
//
// Clients passed to the optimizing client are ordered by speed and calls to
// `Get` race the 2 fastest clients (by default) for the result. If a client
// errors then it is moved to the back of the list.
//
// A speed test is performed periodically in the background every 5 minutes (by
// default) to ensure we're still using the fastest clients. A negative speed
// test interval will disable testing.
//
// Calls to `Get` actually iterate over the speed-ordered client list with a
// concurrency of 2 (by default) until a result is retrieved. It means that the
// optimizing client will fallback to using the other slower clients in the
// event of failure(s).
//
// Additionally, calls to Get are given a timeout of 5 seconds (by default) to
// ensure no unbounded blocking occurs.
func newOptimizingClient(
	clients []Client,
	requestTimeout time.Duration,
	requestConcurrency int,
	speedTestInterval,
	watchRetryInterval time.Duration,
) (*optimizingClient, error) {
	if len(clients) == 0 {
		return nil, errors.New("missing clients")
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
	if requestConcurrency <= 0 {
		requestConcurrency = defaultRequestConcurrency
	}
	if speedTestInterval == 0 {
		speedTestInterval = defaultSpeedTestInterval
	}
	if watchRetryInterval == 0 {
		watchRetryInterval = defaultWatchRetryInterval
	}
	oc := &optimizingClient{
		clients:            clients,
		stats:              stats,
		requestTimeout:     requestTimeout,
		requestConcurrency: requestConcurrency,
		speedTestInterval:  speedTestInterval,
		watchRetryInterval: watchRetryInterval,
		log:                log.DefaultLogger(),
		done:               done,
	}
	return oc, nil
}

// Start starts the background speed measurements of the optimizing client.Start
// SetLog should not be called after Start.
func (oc *optimizingClient) Start() {
	if oc.speedTestInterval > 0 {
		go oc.testSpeed()
	}
}

// MarkPassive tags a client as 'passive' - a generalization of the libp2p style gossip client.
// These clients will not participate in the speed test horse race, and will be protected from
// being stopped by the optimized watcher.
// Note: if a client marked as passive closes its results channel from a `watch` call, the
// optimizing client will not re-open it, as would be attempted with non-passive clients.
// MarkPassive must tag clients as passive before `Start` is run.
func (oc *optimizingClient) MarkPassive(c Client) {
	oc.passiveClients = append(oc.passiveClients, c)
}

type optimizingClient struct {
	sync.RWMutex
	clients            []Client
	passiveClients     []Client
	stats              []*requestStat
	requestTimeout     time.Duration
	requestConcurrency int
	speedTestInterval  time.Duration
	watchRetryInterval time.Duration
	log                log.Logger
	done               chan struct{}
}

// String returns the name of this client.
func (oc *optimizingClient) String() string {
	names := make([]string, len(oc.clients))
	for i, c := range oc.clients {
		names[i] = fmt.Sprint(c)
	}
	return fmt.Sprintf("OptimizingClient(%s)", strings.Join(names, ", "))
}

type requestStat struct {
	// client is the client used to make the request.
	client Client
	// rtt is the time it took to make the request.
	rtt time.Duration
	// startTime is the time at which the request was started.
	startTime time.Time
}

type requestResult struct {
	// client is the client used to make the request.
	client Client
	// result is the return value from the call to Get.
	result Result
	// err is the error that occurred from a call to Get (not including context error).
	err error
	// stat is stats from the call to Get.
	stat *requestStat
}

// markedPassive checks if a client should be treated as passive
func (oc *optimizingClient) markedPassive(c Client) bool {
	for _, p := range oc.passiveClients {
		if p == c {
			return true
		}
	}
	return false
}

func (oc *optimizingClient) testSpeed() {
	clients := make([]Client, 0, len(oc.clients))
	for _, c := range oc.clients {
		if !oc.markedPassive(c) {
			clients = append(clients, c)
		}
	}

	for {
		stats := []*requestStat{}
		ctx, cancel := context.WithCancel(context.Background())
		ch := parallelGet(ctx, clients, 1, oc.requestTimeout, oc.requestConcurrency)

	LOOP:
		for {
			select {
			case rr, ok := <-ch:
				if !ok {
					cancel()
					break LOOP
				}
				if rr.err != nil {
					oc.log.Info("optimizing_client", "endpoint down when speed tested", "client", fmt.Sprintf("%s", rr.client), "err", rr.err)
				}
				stats = append(stats, rr.stat)
			case <-oc.done:
				cancel()
				return
			}
		}

		oc.updateStats(stats)

		t := time.NewTimer(oc.speedTestInterval)
		select {
		case <-t.C:
		case <-oc.done:
			t.Stop()
			return
		}
	}
}

// SetLog configures the client log output.
func (oc *optimizingClient) SetLog(l log.Logger) {
	oc.log = l
}

// fastestClients returns a ordered slice of clients - fastest first.
func (oc *optimizingClient) fastestClients() []Client {
	oc.RLock()
	defer oc.RUnlock()
	// copy the current ordered client list so we iterate over a stable slice
	var clients []Client
	for _, s := range oc.stats {
		clients = append(clients, s.client)
	}
	return clients
}

// Get returns the randomness at `round` or an error.
func (oc *optimizingClient) Get(ctx context.Context, round uint64) (res Result, err error) {
	clients := oc.fastestClients()
	stats := []*requestStat{}
	ch := raceGet(ctx, clients, round, oc.requestTimeout, oc.requestConcurrency)
	err = errors.New("no valid clients")

LOOP:
	for {
		select {
		case rr, ok := <-ch:
			if !ok {
				break LOOP
			}
			stats = append(stats, rr.stat)
			res, err = rr.result, rr.err
		case <-ctx.Done():
			oc.updateStats(stats)
			return nil, ctx.Err()
		case <-oc.done:
			oc.updateStats(stats)
			return nil, errors.New("client closed")
		}
	}

	oc.updateStats(stats)
	return
}

// get calls Get on the passed client and returns a requestResult or nil if the context was canceled.
func get(ctx context.Context, client Client, round uint64) *requestResult {
	start := time.Now()
	res, err := client.Get(ctx, round)
	rtt := time.Since(start)
	var stat requestStat

	// client failure, set a large RTT so it is sent to the back of the list
	if err != nil && err != ctx.Err() {
		stat = requestStat{client, math.MaxInt64, start}
		return &requestResult{client, res, err, &stat}
	}

	if ctx.Err() != nil {
		return nil
	}

	stat = requestStat{client, rtt, start}
	return &requestResult{client, res, err, &stat}
}

func raceGet(ctx context.Context, clients []Client, round uint64, timeout time.Duration, concurrency int) <-chan *requestResult {
	results := make(chan *requestResult, len(clients))

	go func() {
		rctx, cancel := context.WithCancel(ctx)
		defer cancel()
		defer close(results)
		ch := parallelGet(rctx, clients, round, timeout, concurrency)

		for {
			select {
			case rr, ok := <-ch:
				if !ok {
					return
				}
				results <- rr
				if rr.err == nil { // race is won
					return
				}
			case <-rctx.Done():
				return
			}
		}
	}()

	return results
}

func parallelGet(ctx context.Context, clients []Client, round uint64, timeout time.Duration, concurrency int) <-chan *requestResult {
	results := make(chan *requestResult, len(clients))
	token := make(chan struct{}, concurrency)

	for i := 0; i < concurrency; i++ {
		token <- struct{}{}
	}

	go func() {
		wg := sync.WaitGroup{}
	LOOP:
		for _, c := range clients {
			select {
			case <-token:
				wg.Add(1)
				go func(c Client) {
					gctx, cancel := context.WithTimeout(ctx, timeout)
					rr := get(gctx, c, round)
					cancel()
					if rr != nil {
						results <- rr
					}
					token <- struct{}{}
					wg.Done()
				}(c)
			case <-ctx.Done():
				break LOOP
			}
		}
		wg.Wait()
		close(results)
	}()

	return results
}

func (oc *optimizingClient) updateStats(stats []*requestStat) {
	oc.Lock()
	defer oc.Unlock()

	// update the round trip times with new samples
	for _, next := range stats {
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

type watchResult struct {
	Result
	Client
}

func (oc *optimizingClient) trackWatchResults(info *chain.Info, in chan watchResult, out chan Result) {
	defer close(out)

	latest := uint64(0)
	for r := range in {
		round := r.Result.Round()
		timeOfRound := time.Unix(chain.TimeOfRound(info.Period, info.GenesisTime, round), 0)
		stat := requestStat{
			client:    r.Client,
			rtt:       time.Since(timeOfRound),
			startTime: timeOfRound,
		}
		oc.updateStats([]*requestStat{&stat})
		if round > latest {
			latest = round
			out <- r.Result
		}
	}
}

// Watch returns new randomness as it becomes available.
func (oc *optimizingClient) Watch(ctx context.Context) <-chan Result {
	outChan := make(chan Result, defaultChannelBuffer)
	inChan := make(chan watchResult, defaultChannelBuffer)

	info, err := oc.Info(ctx)
	if err != nil {
		oc.log.Error("optimizing_client", "failed to learn info", "err", err)
		close(outChan)
		return outChan
	}

	state := watchState{
		ctx:           ctx,
		optimizer:     oc,
		active:        make([]watchingClient, 0),
		protected:     make([]watchingClient, 0),
		failed:        make([]failedClient, 0),
		retryInterval: oc.watchRetryInterval,
	}

	closingClients := make(chan Client, 1)
	for _, c := range oc.passiveClients {
		go state.watchNext(ctx, c, inChan, closingClients)
		state.protected = append(state.protected, watchingClient{c, nil})
	}

	go state.dispatchWatchingClients(inChan, closingClients)
	go oc.trackWatchResults(info, inChan, outChan)
	return outChan
}

type watchingClient struct {
	Client
	context.CancelFunc
}

type failedClient struct {
	Client
	backoffUntil time.Time
}

type watchState struct {
	ctx           context.Context
	optimizer     *optimizingClient
	active        []watchingClient
	protected     []watchingClient
	failed        []failedClient
	retryInterval time.Duration
}

func (ws *watchState) dispatchWatchingClients(resultChan chan watchResult, closingClients chan Client) {
	defer close(resultChan)

	// spin up initial watcher(s)
	ws.tryRepopulate(resultChan, closingClients)

	ticker := time.NewTicker(ws.optimizer.watchRetryInterval)
	defer ticker.Stop()
	for {
		select {
		case c := <-closingClients:
			// replace failed watchers
			ws.done(c)
			if ws.ctx.Err() == nil {
				ws.tryRepopulate(resultChan, closingClients)
			}
			if len(ws.active) == 0 && len(ws.protected) == 0 {
				return
			}
		case <-ticker.C:
			// periodically cycle to fastest client.
			clients := ws.optimizer.fastestClients()
			if len(clients) == 0 {
				continue
			}
			fastest := clients[0]
			if ws.hasActive(fastest) == -1 && ws.hasProtected(fastest) == -1 {
				ws.closeSlowest()
				ws.tryRepopulate(resultChan, closingClients)
			}
		case <-ws.ctx.Done():
			// trigger client close. Will return once len(ws.active) == 0
			for _, c := range ws.active {
				c.CancelFunc()
			}
		}
	}
}

func (ws *watchState) tryRepopulate(results chan watchResult, done chan Client) {
	ws.clean()

	for {
		if len(ws.active) >= ws.optimizer.requestConcurrency {
			return
		}
		c := ws.nextUnwatched()
		if c == nil {
			return
		}
		cctx, cancel := context.WithCancel(ws.ctx)

		ws.active = append(ws.active, watchingClient{c, cancel})
		ws.optimizer.log.Info("optimizing_client", "watching on client", "client", fmt.Sprintf("%s", c))
		go ws.watchNext(cctx, c, results, done)
	}
}

func (ws *watchState) watchNext(ctx context.Context, c Client, out chan watchResult, done chan Client) {
	defer func() { done <- c }()

	resultStream := c.Watch(ctx)
	for r := range resultStream {
		out <- watchResult{r, c}
	}
	ws.optimizer.log.Info("optimizing_client", "watch ended", "client", fmt.Sprintf("%s", c))
}

func (ws *watchState) clean() {
	nf := make([]failedClient, 0, len(ws.failed))
	for _, f := range ws.failed {
		if f.backoffUntil.After(time.Now()) {
			nf = append(nf, f)
		}
	}
	ws.failed = nf
}

func (ws *watchState) close(clientIdx int) {
	ws.active[clientIdx].CancelFunc()
	ws.active[clientIdx] = ws.active[len(ws.active)-1]
	ws.active[len(ws.active)-1] = watchingClient{}
	ws.active = ws.active[:len(ws.active)-1]
}

func (ws *watchState) done(c Client) {
	idx := ws.hasActive(c)
	if idx > -1 {
		ws.close(idx)
		ws.failed = append(ws.failed, failedClient{c, time.Now().Add(ws.retryInterval)})
	} else if i := ws.hasProtected(c); i > -1 {
		ws.protected[i] = ws.protected[len(ws.protected)-1]
		ws.protected = ws.protected[:len(ws.protected)-1]
		return
	}
	// note: it's expected that the client may already not be active.
	// this happens when the optimizing client has closed it via `closeSlowest`
}

func (ws *watchState) hasActive(c Client) int {
	for i, a := range ws.active {
		if a.Client == c {
			return i
		}
	}
	return -1
}

func (ws *watchState) hasProtected(c Client) int {
	for i, p := range ws.protected {
		if p.Client == c {
			return i
		}
	}
	return -1
}

func (ws *watchState) closeSlowest() {
	if len(ws.active) == 0 {
		return
	}
	order := ws.optimizer.fastestClients()
	idxs := make([]int, 0)
	for _, c := range order {
		if i := ws.hasActive(c); i > -1 {
			idxs = append(idxs, i)
		}
	}
	ws.close(idxs[len(idxs)-1])
}

func (ws *watchState) nextUnwatched() Client {
	clients := ws.optimizer.fastestClients()
CLIENT_LOOP:
	for _, c := range clients {
		for _, a := range ws.active {
			if c == a.Client {
				continue CLIENT_LOOP
			}
		}
		for _, f := range ws.failed {
			if c == f.Client {
				continue CLIENT_LOOP
			}
		}
		for _, p := range ws.protected {
			if c == p.Client {
				continue CLIENT_LOOP
			}
		}
		return c
	}
	return nil
}

// Info returns the parameters of the chain this client is connected to.
// The public key, when it started, and how frequently it updates.
func (oc *optimizingClient) Info(ctx context.Context) (chainInfo *chain.Info, err error) {
	clients := oc.fastestClients()
	for _, c := range clients {
		ctx, cancel := context.WithTimeout(context.Background(), oc.requestTimeout)
		chainInfo, err = c.Info(ctx)
		cancel()
		if err == nil {
			return
		}
	}
	return
}

// RoundAt will return the most recent round of randomness that will be available
// at time for the current client.
func (oc *optimizingClient) RoundAt(t time.Time) uint64 {
	return oc.clients[0].RoundAt(t)
}

// Close stops the background speed tests and closes the client and it's
// underlying clients for further use.
func (oc *optimizingClient) Close() error {
	var errs *multierror.Error
	for _, c := range oc.clients {
		errs = multierror.Append(errs, c.Close())
	}
	close(oc.done)
	return errs.ErrorOrNil()
}
