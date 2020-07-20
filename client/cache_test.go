package client

import (
	"context"
	"sync"
	"testing"

	"github.com/drand/drand/client/test/result/mock"
)

func TestCacheGet(t *testing.T) {
	m := MockClientWithResults(1, 6)
	cache, err := makeCache(3)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewCachingClient(m, cache)
	if err != nil {
		t.Fatal(err)
	}
	res, e := c.Get(context.Background(), 1)
	if e != nil {
		t.Fatal(e)
	}
	res.(*mock.Result).AssertValid(t)

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
	cache, err := makeCache(3)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewCachingClient(m, cache)
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
	arcCache, err := makeCache(3)
	if err != nil {
		t.Fatal(err)
	}
	cache, _ := NewCachingClient(m, arcCache)
	c := newWatchAggregator(cache, false, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r1 := c.Watch(ctx)
	rc <- &mock.Result{Rnd: 1, Rand: []byte{1}}
	_, ok := <-r1
	if !ok {
		t.Fatal("results should propagate")
	}

	_, err = c.Get(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Results) != 4 {
		t.Fatalf("getting should be served by cache.")
	}
}

func TestCacheClose(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(1)

	c := &MockClient{
		WatchCh: make(chan Result),
		CloseF: func() error {
			wg.Done()
			return nil
		},
	}

	cache, err := makeCache(1)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := NewCachingClient(c, cache)
	if err != nil {
		t.Fatal(err)
	}

	err = ca.Close() // should close the underlying client
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait() // wait for underlying client to close
}
