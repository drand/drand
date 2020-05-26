package client

import (
	"context"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func newMetricController(chainInfo *chain.Info, r prometheus.Registerer) *metricController {
	return &metricController{chainInfo: chainInfo, bridge: r}
}

type metricController struct {
	chainInfo *chain.Info
	bridge    prometheus.Registerer
}

func (mc *metricController) Register(x prometheus.Collector) error {
	return mc.bridge.Register(x)
}

func newWatchLatencyMetricClient(base Client, ctl *metricController) (Client, error) {
	c := &watchLatencyMetricClient{
		Client:       base,
		ctl:          ctl,
		watchLatency: metrics.ClientWatchLatency,
	}
	if err := c.ctl.Register(c.watchLatency); err != nil {
		return nil, err
	}
	go c.startObserve(context.Background())
	return c, nil
}

type watchLatencyMetricClient struct {
	Client
	ctl          *metricController
	watchLatency *prometheus.GaugeVec
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
			actual := time.Now().Unix()
			expected := chain.TimeOfRound(c.ctl.chainInfo.Period, c.ctl.chainInfo.GenesisTime, result.Round())
			// the labels of the gauge vec must already be set at the registerer level
			c.watchLatency.With(nil).Set(float64(expected - actual))
		case <-ctx.Done():
			return
		}
	}
}
