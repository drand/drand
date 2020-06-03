package basic

import (
	"context"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
)

func TestWatcherWatch(t *testing.T) {
	results := []MockResult{
		{rnd: 1, rand: []byte{1}},
		{rnd: 2, rand: []byte{2}},
	}

	ch := make(chan client.Result, len(results))
	for i := range results {
		ch <- &results[i]
	}
	close(ch)

	ctor := func(chainInfo *chain.Info, _ Cache) (Watcher, error) {
		return &MockClient{WatchCh: ch}, nil
	}

	watcher, err := ctor(fakeChainInfo(), nil)
	if err != nil {
		t.Fatal(err)
	}
	w := watcherClient{nil, watcher}

	i := 0
	for r := range w.Watch(context.Background()) {
		compareResults(t, r, &results[i])
		i++
	}
}

func TestWatcherGet(t *testing.T) {
	results := []MockResult{
		{rnd: 1, rand: []byte{1}},
		{rnd: 2, rand: []byte{2}},
	}

	cr := make([]MockResult, len(results))
	copy(cr, results)

	c := &MockClient{Results: cr}
	ctor := func(chainInfo *chain.Info, _ Cache) (Watcher, error) {
		return c, nil
	}

	watcher, err := ctor(fakeChainInfo(), nil)
	if err != nil {
		t.Fatal(err)
	}
	w := watcherClient{c, watcher}

	for _, result := range results {
		r, err := w.Get(context.Background(), 0)
		if err != nil {
			t.Fatal(err)
		}
		compareResults(t, r, &result)
	}
}

func TestWatcherRoundAt(t *testing.T) {
	c := &MockClient{}
	ctor := func(chainInfo *chain.Info, _ Cache) (Watcher, error) {
		return c, nil
	}

	watcher, err := ctor(fakeChainInfo(), nil)
	if err != nil {
		t.Fatal(err)
	}
	w := watcherClient{c, watcher}

	if w.RoundAt(time.Now()) != 0 {
		t.Fatal("unexpected RoundAt value")
	}
}
