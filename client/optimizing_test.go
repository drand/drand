package client

import (
	"context"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
)

func expectRound(t *testing.T, res Result, r uint64) {
	t.Helper()
	if res.Round() != r {
		t.Fatalf("expected round %v, got %v", r, res.Round())
	}
}

func TestOptimizingGet(t *testing.T) {
	c0 := MockClientWithResults(0, 3)
	c1 := MockClientWithResults(3, 5)

	c0.Delay = time.Millisecond * 100
	c1.Delay = time.Millisecond

	oc, err := NewOptimizingClient([]Client{c0, c1}, &chain.Info{}, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	expectRound(t, latestResult(t, oc), 0) // round 0 from c0
	expectRound(t, latestResult(t, oc), 3) // round 3 from c1
	expectRound(t, latestResult(t, oc), 4) // round 4 from c1
	expectRound(t, latestResult(t, oc), 1) // c1 error (no results left), round 1 from c0
	expectRound(t, latestResult(t, oc), 2) // round 2 from c0
}

func TestOptimizingWatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c0 := MockClientWithResults(0, 3)
	c1 := MockClientWithResults(3, 5)

	c0.Delay = time.Millisecond * 100
	c1.Delay = time.Millisecond

	oc, err := NewOptimizingClient([]Client{c0, c1}, fakeChainInfo(), 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	ch := oc.Watch(ctx)

	expectRound(t, nextResult(t, ch), 0) // round 0 from c0
	expectRound(t, nextResult(t, ch), 3) // round 3 from c1
	expectRound(t, nextResult(t, ch), 4) // round 4 from c1
	expectRound(t, nextResult(t, ch), 1) // c1 error (no results left), round 1 from c0
	expectRound(t, nextResult(t, ch), 2) // round 2 from c0
}

func TestOptimizingRequiresClients(t *testing.T) {
	_, err := NewOptimizingClient([]Client{}, &chain.Info{}, 0, 0, 0)
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
	oc, err := NewOptimizingClient([]Client{&MockClient{}}, &chain.Info{}, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	lc, ok := oc.(LoggingClient)
	if !ok {
		t.Fatal("expected implements LoggingClient")
	}
	lc.SetLog(log.DefaultLogger)
}

func TestOptimizingRoundAt(t *testing.T) {
	oc, err := NewOptimizingClient([]Client{&MockClient{}}, &chain.Info{}, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	r := oc.RoundAt(time.Now()) // mock client returns 0 always
	if r != 0 {
		t.Fatal("unexpected round", r)
	}
}
