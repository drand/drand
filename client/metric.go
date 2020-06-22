package client

import (
	"context"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/metrics"
)

func newWatchLatencyMetricClient(base Client, info *chain.Info) Client {
	ctx, cancel := context.WithCancel(context.Background())
	c := &watchLatencyMetricClient{
		Client:    base,
		chainInfo: info,
		cancel:    cancel,
	}
	go c.startObserve(ctx)
	return c
}

type watchLatencyMetricClient struct {
	Client
	chainInfo *chain.Info
	cancel    context.CancelFunc
}

func (c *watchLatencyMetricClient) startObserve(ctx context.Context) {
	rch := c.Watch(ctx)
	for {
		select {
		case result, ok := <-rch:
			if !ok {
				return
			}
			// compute the latency metric
			actual := time.Now().UnixNano()
			expected := chain.TimeOfRound(c.chainInfo.Period, c.chainInfo.GenesisTime, result.Round()) * 1e9
			// the labels of the gauge vec must already be set at the registerer level
			metrics.ClientWatchLatency.Set(float64(actual-expected) / float64(time.Millisecond))
		case <-ctx.Done():
			return
		}
	}
}

func (c *watchLatencyMetricClient) Close() error {
	err := c.Client.Close()
	c.cancel()
	return err
}
