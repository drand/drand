package client

import (
	"io"
	"sync"
	"testing"
)

func TestMetricClose(t *testing.T) {
	chainInfo := fakeChainInfo()
	wg := sync.WaitGroup{}
	wg.Add(1)

	c := &MockClient{
		WatchCh: make(chan Result),
		CloseF: func() error {
			wg.Done()
			return nil
		},
	}

	mc := newWatchLatencyMetricClient(c, chainInfo)
	cc, ok := mc.(io.Closer)
	if !ok {
		t.Fatal("not an io.Closer")
	}

	err := cc.Close() // should close the underlying client
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait() // wait for underlying client to close
}
