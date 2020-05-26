package client

import (
	"context"
	"log"
	"time"

	"github.com/drand/drand/chain"
	"github.com/prometheus/client_golang/prometheus"
)

func newMetricController(p time.Duration, b PrometheusBridge) *metricController {
	return &metricController{period: p, bridge: b}
}

type metricController struct {
	period time.Duration
	bridge PrometheusBridge
}

func (mc *metricController) Start() {
	for {
		time.Sleep(mc.period)
		if err := mc.bridge.Push(); err != nil {
			log.Printf("prometheus gateway push (%v)", err)
		}
	}
}

func (mc *metricController) Register(x prometheus.Collector) error {
	return mc.bridge.Register(x)
}

func newWatchLatencyMetricClient(id string, base Client, ctl *metricController, chainInfo *chain.Info) (Client, error) {
	c := &watchLatencyMetricClient{
		Client:    base,
		ctl:       ctl,
		chainInfo: chainInfo,
		watchLatency: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "drand_client_observation",
			Subsystem: id,
			Name:      "watch_latency",
			Help:      "Duration between time round received and time round expected.",
		}),
	}
	if err := c.ctl.Register(c.watchLatency); err != nil {
		return nil, err
	}
	return c, nil
}

type watchLatencyMetricClient struct {
	Client
	ctl          *metricController
	chainInfo    *chain.Info
	watchLatency prometheus.Gauge
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
			expected := chain.TimeOfRound(c.chainInfo.Period, c.chainInfo.GenesisTime, result.Round())
			c.watchLatency.Set(float64(expected - actual))
		case <-ctx.Done():
			return
		}
	}
}
