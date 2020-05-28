package client

import (
	"context"
	"testing"

	"github.com/drand/drand/log"
)

func TestCacheGet(t *testing.T) {
	m := MockClientWithResults(1, 6)
	c, err := NewCachingClient(m, 3, log.DefaultLogger)
	if err != nil {
		t.Fatal(err)
	}
	res, e := c.Get(context.Background(), 1)
	if e != nil {
		t.Fatal(e)
	}
	res.(*MockResult).AssertValid(t)

	_, e = c.Get(context.Background(), 1)
	if e != nil {
		t.Fatal(e)
	}
	if len(m.Results) < 4 {
		t.Fatal("multiple gets should cache.")
	}
	_, e = c.Get(context.Background(), 2)
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.Get(context.Background(), 3)
	if e != nil {
		t.Fatal(e)
	}

	_, e = c.Get(context.Background(), 1)
	if e != nil {
		t.Fatal(e)
	}
	if len(m.Results) != 2 {
		t.Fatalf("unexpected cache size. %d", len(m.Results))
	}
}

func TestCacheGetLatest(t *testing.T) {
	m := MockClientWithResults(1, 3)
	c, err := NewCachingClient(m, 2, log.DefaultLogger)
	if err != nil {
		t.Fatal(err)
	}

	r0, e := c.Get(context.Background(), 0)
	if e != nil {
		t.Fatal(e)
	}
	r1, e := c.Get(context.Background(), 0)
	if e != nil {
		t.Fatal(e)
	}

	if r0.Round() == r1.Round() {
		t.Fatal("cached result for latest")
	}
}

func TestCacheWatch(t *testing.T) {
	m := MockClientWithResults(2, 6)
	rc := make(chan Result, 1)
	m.WatchCh = rc
	cache, _ := NewCachingClient(m, 2, log.DefaultLogger)
	c := newWatchAggregator(cache, log.DefaultLogger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r1 := c.Watch(ctx)
	rc <- &MockResult{rnd: 1, rand: []byte{1}}
	_, ok := <-r1
	if !ok {
		t.Fatal("results should propagate")
	}

	_, err := c.Get(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Results) != 4 {
		t.Fatalf("getting should be served by cache.")
	}
}
