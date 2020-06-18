package http

import (
	"context"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// MeasureHeartbeats periodically tracks latency observed on a set of HTTP clients
func MeasureHeartbeats(ctx context.Context, c []client.Client) *HealthMetrics {
	m := &HealthMetrics{
		next:    0,
		clients: c,
	}
	if len(c) > 0 {
		go m.startObserve(ctx)
	}
	return m
}

// HealthMetrics is a measurement task around HTTP clients
type HealthMetrics struct {
	next    int
	clients []client.Client
}

// HTTPHeartbeatInterval is the duration between liveness heartbeats sent to an HTTP API.
const HTTPHeartbeatInterval = 10 * time.Second

func (c *HealthMetrics) startObserve(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		time.Sleep(time.Duration(int64(HTTPHeartbeatInterval) / int64(len(c.clients))))
		n := c.next % len(c.clients)

		httpClient, ok := c.clients[n].(*httpClient)
		if !ok {
			c.next++
			continue
		}

		result, err := c.clients[n].Get(ctx, c.clients[n].RoundAt(time.Now())+1)
		if err != nil {
			metrics.ClientHTTPHeartbeatFailure.With(prometheus.Labels{"http_address": httpClient.root}).Inc()
			continue
		} else {
			metrics.ClientHTTPHeartbeatSuccess.With(prometheus.Labels{"http_address": httpClient.root}).Inc()
		}
		// compute the latency metric
		actual := time.Now().UnixNano()
		expected := chain.TimeOfRound(httpClient.chainInfo.Period, httpClient.chainInfo.GenesisTime, result.Round()) * 1e9
		// the labels of the gauge vec must already be set at the registerer level
		metrics.ClientHTTPHeartbeatLatency.With(prometheus.Labels{"http_address": httpClient.root}).
			Set(float64(actual-expected) / float64(time.Millisecond))
		c.next++
	}
}
