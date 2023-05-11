package client

import (
	"sync"
	"testing"

	clientMock "github.com/drand/drand/client/mock"
	"github.com/drand/drand/common/client"
)

func TestMetricClose(t *testing.T) {
	chainInfo := fakeChainInfo(t)
	wg := sync.WaitGroup{}
	wg.Add(1)

	c := &clientMock.Client{
		WatchCh: make(chan client.Result),
		CloseF: func() error {
			wg.Done()
			return nil
		},
	}

	mc := newWatchLatencyMetricClient(c, chainInfo)

	err := mc.Close() // should close the underlying client
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait() // wait for underlying client to close
}
