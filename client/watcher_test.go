package client

import (
	"context"
	"sync"
	"testing"
	"time"

	clientMock "github.com/drand/drand/client/mock"
	"github.com/drand/drand/client/test/result/mock"
	"github.com/drand/drand/common/client"
)

func TestWatcherWatch(t *testing.T) {
	results := []mock.Result{
		{Rnd: 1, Rand: []byte{1}},
		{Rnd: 2, Rand: []byte{2}},
	}

	ch := make(chan client.Result, len(results))
	for i := range results {
		ch <- &results[i]
	}
	close(ch)

	w := watcherClient{nil, &clientMock.Client{WatchCh: ch}}

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

	c := &clientMock.Client{Results: cr}

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
	c := &clientMock.Client{}

	w := watcherClient{c, c}

	if w.RoundAt(time.Now()) != 0 {
		t.Fatal("unexpected RoundAt value")
	}
}

func TestWatcherClose(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(2)

	closeF := func() error {
		wg.Done()
		return nil
	}

	w := &clientMock.Client{CloseF: closeF}
	c := &clientMock.Client{CloseF: closeF}

	wc := &watcherClient{c, w}
	err := wc.Close() // should close the underlying client AND watcher
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait() // wait for underlying client AND watcher to close
}
