package basic

import (
	"context"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func newHTTPHealthMetrics(httpAddrs []string, clients []client.Client, info *chain.Info) *httpHealthMetrics {
	if len(clients) != len(httpAddrs) {
		panic("client/address count mismatch")
	}
	if len(clients) == 0 {
		return nil
	}
	c := &httpHealthMetrics{
		next:      0,
		httpAddrs: httpAddrs,
		clients:   clients,
		chainInfo: info,
	}
	go c.startObserve(context.Background())
	return c
}

type httpHealthMetrics struct {
	next      int
	httpAddrs []string
	clients   []client.Client
	chainInfo *chain.Info
}

// HTTPHeartbeatInterval is the duration between liveness heartbeats sent to an HTTP API.
const HTTPHeartbeatInterval = 10 * time.Second

func (c *httpHealthMetrics) startObserve(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		time.Sleep(time.Duration(int64(HTTPHeartbeatInterval) / int64(len(c.clients))))
		n := c.next % len(c.clients)
		result, err := c.clients[n].Get(ctx, 0)
		if err != nil {
			metrics.ClientHTTPHeartbeatFailure.With(prometheus.Labels{"http_address": c.httpAddrs[n]}).Inc()
			continue
		} else {
			metrics.ClientHTTPHeartbeatSuccess.With(prometheus.Labels{"http_address": c.httpAddrs[n]}).Inc()
		}
		// compute the latency metric
		actual := time.Now().UnixNano()
		expected := chain.TimeOfRound(c.chainInfo.Period, c.chainInfo.GenesisTime, result.Round()) * 1e9
		// the labels of the gauge vec must already be set at the registerer level
		metrics.ClientHTTPHeartbeatLatency.With(prometheus.Labels{"http_address": c.httpAddrs[n]}).
			Set(float64(actual-expected) / 1e6) // latency in milliseconds
		c.next++
	}
}

func newWatchLatencyMetricClient(base client.Client, info *chain.Info) (client.Client, error) {
	c := &watchLatencyMetricClient{
		Client:    base,
		chainInfo: info,
	}
	go c.startObserve(context.Background())
	return c, nil
}

type watchLatencyMetricClient struct {
	client.Client
	chainInfo *chain.Info
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
			// the labels of the gauge vec must already be set at the registerer level
			metrics.ClientWatchLatency.Set(float64(expected - actual))
		case <-ctx.Done():
			return
		}
	}
}
