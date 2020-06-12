package client

import (
	"context"
	"testing"
	"time"

	"github.com/drand/drand/client/test/mock"
)

func TestWatcherWatch(t *testing.T) {
	results := []mock.Result{
		{Rnd: 1, Rand: []byte{1}},
		{Rnd: 2, Rand: []byte{2}},
	}

	ch := make(chan Result, len(results))
	for i := range results {
		ch <- &results[i]
	}
	close(ch)

	w := watcherClient{nil, &MockClient{WatchCh: ch}}

	i := 0
	for r := range w.Watch(context.Background()) {
		compareResults(t, r, &results[i])
		i++
	}
}

func TestWatcherGet(t *testing.T) {
	results := []mock.Result{
		{Rnd: 1, Rand: []byte{1}},
		{Rnd: 2, Rand: []byte{2}},
	}

	cr := make([]mock.Result, len(results))
	copy(cr, results)

	c := &MockClient{Results: cr}

	w := watcherClient{c, c}

	for i := range results {
		r, err := w.Get(context.Background(), 0)
		if err != nil {
			t.Fatal(err)
		}
		compareResults(t, r, &results[i])
	}
}

func TestWatcherRoundAt(t *testing.T) {
	c := &MockClient{}

	w := watcherClient{c, c}

	if w.RoundAt(time.Now()) != 0 {
		t.Fatal("unexpected RoundAt value")
	}
}
