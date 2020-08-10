package client

import (
	"sync"
	"testing"
)

func TestAggregatorClose(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(1)

	c := &MockClient{
		WatchCh: make(chan Result),
		CloseF: func() error {
			wg.Done()
			return nil
		},
	}

	ac := newWatchAggregator(c, true, 0)

	err := ac.Close() // should cancel the autoWatch and close the underlying client
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait() // wait for underlying client to close
}
