package client

import (
	"context"
	"io"
	"testing"
	"time"

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
	cl, ok := c.(io.Closer)
	if !ok {
		t.Fatal("client is not an io.Closer")
	}
	err := cl.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestOptimizingGet(t *testing.T) {
	c0 := MockClientWithResults(0, 5)
	c1 := MockClientWithResults(5, 8)

	c0.Delay = time.Millisecond * 100
	c1.Delay = time.Millisecond

	oc, err := NewOptimizingClient([]Client{c0, c1}, fakeChainInfo(), time.Second*5, 2, time.Minute*5)
	if err != nil {
		t.Fatal(err)
	}
	defer closeClient(t, oc)

	waitForSpeedTest(t, oc, time.Minute)

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

	c0.Delay = time.Millisecond * 100
	c1.Delay = time.Millisecond

	oc, err := NewOptimizingClient([]Client{c0, c1}, fakeChainInfo(), time.Second*5, 2, time.Minute*5)
	if err != nil {
		t.Fatal(err)
	}
	defer closeClient(t, oc)

	waitForSpeedTest(t, oc, time.Minute)

	ch := oc.Watch(ctx)

	// speed test will consume round 0 and 5 from c0 and c1
	// then c1 will be used because it's faster
	expectRound(t, nextResult(t, ch), 6) // round 6 from c1 and round 1 from c0 (discarded)
	expectRound(t, nextResult(t, ch), 7) // round 7 from c1 and round 2 from c0 (discarded)
	expectRound(t, nextResult(t, ch), 3) // c1 error (no results left), round 3 from c0
	expectRound(t, nextResult(t, ch), 4) // round 4 from c0
}

func TestOptimizingRequiresClients(t *testing.T) {
	_, err := NewOptimizingClient([]Client{}, fakeChainInfo(), 0, 0, 0)
	if err.Error() != "missing clients" {
		t.Fatal("unexpected error", err)
	}
}

func TestOptimizingRequiresChainInfo(t *testing.T) {
	_, err := NewOptimizingClient([]Client{&MockClient{}}, nil, 0, 0, 0)
	if err.Error() != "missing chain info" {
		t.Fatal("unexpected error", err)
	}
}

func TestOptimizingIsLogging(t *testing.T) {
	oc, err := NewOptimizingClient([]Client{&MockClient{}}, fakeChainInfo(), 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	lc, ok := oc.(LoggingClient)
	if !ok {
		t.Fatal("expected implements LoggingClient")
	}
	lc.SetLog(log.DefaultLogger)
}

func TestOptimizingIsCloser(t *testing.T) {
	oc, err := NewOptimizingClient([]Client{&MockClient{}}, fakeChainInfo(), 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	cc, ok := oc.(io.Closer)
	if !ok {
		t.Fatal("expected implements io.Closer")
	}
	cc.Close()
}

func TestOptimizingRoundAt(t *testing.T) {
	oc, err := NewOptimizingClient([]Client{&MockClient{}}, fakeChainInfo(), 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	r := oc.RoundAt(time.Now()) // mock client returns 0 always
	if r != 0 {
		t.Fatal("unexpected round", r)
	}
}
