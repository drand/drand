package client

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/drand/drand/client/test/result/mock"
	"github.com/drand/drand/log"
)

// waitForSpeedTest waits until all clients have had their initial speed test.
func waitForSpeedTest(t *testing.T, c Client, timeout time.Duration) {
	t.Helper()
	oc, ok := c.(*optimizingClient)
	if !ok {
		t.Fatal("client is not an optimizing client")
	}

	timedOut := time.NewTimer(timeout)
	defer timedOut.Stop()
	for {
		oc.RLock()
		tested := true
		for _, s := range oc.stats {
			// all RTT's are zero until a speed test has been done
			if s.rtt == 0 {
				tested = false
				break
			}
		}
		oc.RUnlock()

		if tested {
			return
		}

		// try again in a bit...
		zzz := time.NewTimer(time.Millisecond * 100)
		select {
		case <-zzz.C:
		case <-timedOut.C:
			zzz.Stop()
			t.Fatal("timed out waiting for initial speed test to complete")
		}
	}
}

func expectRound(t *testing.T, res Result, r uint64) {
	t.Helper()
	if res.Round() != r {
		t.Fatalf("expected round %v, got %v", r, res.Round())
	}
}

func closeClient(t *testing.T, c Client) {
	t.Helper()
	err := c.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestOptimizingGet(t *testing.T) {
	c0 := MockClientWithResults(0, 5)
	c1 := MockClientWithResults(5, 8)

	c0.Delay = time.Millisecond * 100
	c1.Delay = time.Millisecond

	oc, err := newOptimizingClient([]Client{c0, c1}, time.Second*5, 2, time.Minute*5, 0)
	if err != nil {
		t.Fatal(err)
	}
	oc.Start()
	defer closeClient(t, oc)

	waitForSpeedTest(t, oc, 10*time.Second)

	// speed test will consume round 0 and 5 from c0 and c1
	// then c1 will be used because it's faster
	expectRound(t, latestResult(t, oc), 6) // round 6 from c1 and round 1 from c0 (discarded)
	expectRound(t, latestResult(t, oc), 7) // round 7 from c1 and round 2 from c0 (discarded)
	expectRound(t, latestResult(t, oc), 3) // c1 error (no results left), round 3 from c0
	expectRound(t, latestResult(t, oc), 4) // round 4 from c0
}

func TestOptimizingWatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c0 := MockClientWithResults(0, 5)
	c1 := MockClientWithResults(5, 8)
	c2 := MockClientWithInfo(fakeChainInfo())

	wc1 := make(chan Result, 5)
	c1.WatchCh = wc1

	c0.Delay = time.Millisecond

	oc, err := newOptimizingClient([]Client{c0, c1, c2}, time.Second*5, 2, time.Minute*5, 0)
	if err != nil {
		t.Fatal(err)
	}
	oc.Start()
	defer closeClient(t, oc)

	waitForSpeedTest(t, oc, time.Minute)

	ch := oc.Watch(ctx)

	expectRound(t, nextResult(t, ch), 1) // round 1 from c0 (after 100ms)
	wc1 <- &mock.Result{Rnd: 2}
	expectRound(t, nextResult(t, ch), 2) // round 2 from c1 and round 2 from c0 (discarded)
	select {
	case <-ch:
		t.Fatal("should not get another watched result at this point")
	case <-time.After(50 * time.Millisecond):
	}
	wc1 <- &mock.Result{Rnd: 6}
	expectRound(t, nextResult(t, ch), 6) // round 6 from c1
}

func TestOptimizingWatchRetryOnClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var rnd uint64
	c := &MockClient{
		// a single result for the speed test
		Results: []mock.Result{mock.NewMockResult(0)},
		// return a watch channel that yields one result then closes
		WatchF: func(context.Context) <-chan Result {
			ch := make(chan Result, 1)
			r := mock.NewMockResult(rnd)
			rnd++
			ch <- &r
			close(ch)
			return ch
		},
	}

	oc, err := newOptimizingClient([]Client{c}, 0, 0, 0, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	oc.Start()
	defer closeClient(t, oc)

	waitForSpeedTest(t, oc, time.Minute)

	ch := oc.Watch(ctx)

	var i uint64
	for r := range ch {
		if r.Round() != i {
			t.Fatal("unexpected round number")
		}
		i++
		if i > 2 {
			break
		}
	}
}

func TestOptimizingWatchFailover(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	chainInfo := fakeChainInfo()

	var rndlk sync.Mutex
	var rnd uint64 = 1
	wf := func(context.Context) <-chan Result {
		rndlk.Lock()
		defer rndlk.Unlock()
		ch := make(chan Result, 1)
		r := mock.NewMockResult(rnd)
		rnd++
		if rnd < 5 {
			ch <- &r
		}
		close(ch)
		return ch
	}
	c1 := &MockClient{
		Results: []mock.Result{mock.NewMockResult(0)},
		WatchF:  wf,
	}
	c2 := &MockClient{
		Results: []mock.Result{mock.NewMockResult(0)},
		WatchF:  wf,
	}

	oc, err := newOptimizingClient([]Client{MockClientWithInfo(chainInfo), c1, c2}, 0, 0, 0, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	oc.Start()
	defer closeClient(t, oc)

	waitForSpeedTest(t, oc, time.Minute)

	ch := oc.Watch(ctx)

	var i uint64 = 1
	for r := range ch {
		if r.Round() != i {
			t.Fatalf("unexpected round number %d vs %d", r.Round(), i)
		}
		i++
		if i > 5 {
			t.Fatal("there are a total of 4 rounds possible")
			break
		}
	}
	if i < 3 {
		t.Fatalf("watching didn't flip / yield expected rounds. %d", i)
	}
}

func TestOptimizingRequiresClients(t *testing.T) {
	_, err := newOptimizingClient([]Client{}, 0, 0, 0, 0)
	if err.Error() != "missing clients" {
		t.Fatal("unexpected error", err)
	}
}

func TestOptimizingIsLogging(t *testing.T) {
	oc, err := newOptimizingClient([]Client{&MockClient{}}, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	oc.SetLog(log.DefaultLogger())
}

func TestOptimizingIsCloser(t *testing.T) {
	oc, err := newOptimizingClient([]Client{&MockClient{}}, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	oc.Start()
	err = oc.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestOptimizingInfo(t *testing.T) {
	chainInfo := fakeChainInfo()
	oc, err := newOptimizingClient([]Client{MockClientWithInfo(chainInfo)}, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	oc.Start()
	i, err := oc.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if i != chainInfo {
		t.Fatal("wrong chain info", i)
	}
}

func TestOptimizingRoundAt(t *testing.T) {
	oc, err := newOptimizingClient([]Client{&MockClient{}}, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	oc.Start()
	r := oc.RoundAt(time.Now()) // mock client returns 0 always
	if r != 0 {
		t.Fatal("unexpected round", r)
	}
}

func TestOptimizingClose(t *testing.T) {
	wg := sync.WaitGroup{}

	closeF := func() error {
		wg.Done()
		return nil
	}

	clients := []Client{
		&MockClient{WatchCh: make(chan Result), CloseF: closeF},
		&MockClient{WatchCh: make(chan Result), CloseF: closeF},
	}

	wg.Add(len(clients))

	oc, err := newOptimizingClient(clients, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	err = oc.Close() // should close the underlying clients
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait() // wait for underlying clients to close
}
