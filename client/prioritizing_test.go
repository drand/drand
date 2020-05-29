package client

import (
	"context"
	"testing"

	"github.com/drand/drand/log"
)

func TestPrioritizingGet(t *testing.T) {
	c := MockClientWithResults(0, 5)
	c2 := MockClientWithResults(6, 10)

	p, err := NewPrioritizingClient([]Client{c, c2}, nil, nil, log.DefaultLogger)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		r, err := p.Get(context.Background(), 0)
		if err != nil {
			t.Fatal("should get from first client")
		}
		if r.Round() >= 5 {
			t.Fatal("wrong client prioritized")
		}
		r.(*MockResult).AssertValid(t)
	}

	r, err := p.Get(context.Background(), 0)
	if err != nil {
		t.Fatal("should not error even when one client does")
	}
	if r.Round() != 6 {
		t.Fatal("failed to switch priority")
	}
	r.(*MockResult).AssertValid(t)

	c.Results = []MockResult{NewMockResult(50)}

	r, err = p.Get(context.Background(), 0)
	if err != nil {
		t.Fatal("should not error")
	}
	if r.Round() != 7 {
		t.Fatal("failed client should remain deprioritized")
	}
	r.(*MockResult).AssertValid(t)
}

func TestPrioritizingWatch(t *testing.T) {
	c := MockClientWithResults(0, 5)
	c2 := MockClientWithResults(6, 10)

	p, _ := NewPrioritizingClient([]Client{c, c2}, nil, nil, log.DefaultLogger)
	ch := p.Watch(context.Background())
	r, ok := <-ch
	if r != nil || ok {
		t.Fatal("watch should fail without group provided")
	}

	p, _ = NewPrioritizingClient([]Client{c, c2}, nil, fakeChainInfo(), log.DefaultLogger)
	ch = p.Watch(context.Background())
	r, ok = <-ch
	if r == nil || !ok {
		t.Fatal("watch should succeed with group for timing")
	}
	if r.Round() != 0 {
		t.Fatal("wrong client prioritized")
	}
}

func TestPrioritizingWatchFromClient(t *testing.T) {
	c := MockClientWithResults(0, 5)
	c2, _ := NewHTTPClientWithInfo("", fakeChainInfo(), nil)

	p, _ := NewPrioritizingClient([]Client{c, c2}, nil, nil, log.DefaultLogger)
	ch := p.Watch(context.Background())
	r, ok := <-ch
	if r == nil || !ok {
		t.Fatal("watch should succeed if http client provided")
	}
	if r.Round() != 0 {
		t.Fatal("wrong client prioritized")
	}
}
