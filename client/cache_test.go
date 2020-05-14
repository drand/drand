package client

import (
	"context"
	"testing"
	"time"

	"github.com/drand/drand/log"
)

func TestCacheGet(t *testing.T) {
	m := MockClientWithResults(1, 6)
	c, err := NewCachingClient(m, 2, log.DefaultLogger)
	if err != nil {
		t.Fatal(err)
	}
	_, e := c.Get(context.Background(), 1)
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.Get(context.Background(), 1)
	if e != nil {
		t.Fatal(e)
	}
	if len(m.(*MockClient).Results) < 4 {
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
	if len(m.(*MockClient).Results) != 1 {
		t.Fatal("unexpected cache size.")
	}
}

func TestCacheWatch(t *testing.T) {
	m := MockClientWithResults(1, 6)
	rc := make(chan Result, 1)
	m.(*MockClient).WatchCh = rc
	c, err := NewCachingClient(m, 2, log.DefaultLogger)
	if err != nil {
		t.Fatal(err)
	}
	ctx, c1 := context.WithCancel(context.Background())
	r1 := c.Watch(ctx)
	rc <- &MockResult{rnd: 1, rand: []byte{1}}
	_, ok := <-r1
	if !ok {
		t.Fatal("results should propagate")
	}

	_, err = c.Get(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.(*MockClient).Results) != 5 {
		t.Fatal("getting should be served by cache.")
	}

	ctx, c2 := context.WithCancel(context.Background())
	r2 := c.Watch(ctx)
	rc <- &MockResult{rnd: 2, rand: []byte{2}}
	if _, ok = <-r1; !ok {
		t.Fatal("should get value")
	}
	if _, ok = <-r2; !ok {
		t.Fatal("should get value from both watchers")
	}
	c1()
	c2()
	// all clients should be gone.
	rc <- &MockResult{rnd: 3, rand: []byte{3}}
	// should now be full. verify by making sure no value is picked up for a bit.
	select {
	case rc <- &MockResult{rnd: 4}:
		t.Fatal("nothing should be draining channel.")
	case <-time.After(15 * time.Millisecond):
	}
}
