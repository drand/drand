// Package client provides a Drand client which observes latencies and reports them via Prometheus.
package client

import (
	"context"
	"time"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/client"
	"github.com/drand/drand/key"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	watchLatency = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "drand_observe",
		Subsystem: "from_client",
		Name:      "watch_latency",
		Help:      "Duration between time round received and time round expected.",
	})
)

func init() {
	prometheus.MustRegister(watchLatency)
}

// Config configures a client observer node.
type Config struct {
	// Group is the  group key for Drand.
	// Group must be non-nil, so that the observer can compute the expected time of each round.
	Group *key.Group
	// URL is a list of REST endpoint URLs
	URL []string
}

func StartObserving(cfg *Config) error {
	c, err := client.New(
		client.WithGroup(cfg.Group),
		client.WithHTTPEndpoints(cfg.URL),
	)
	if err != nil {
		return err
	}
	XXX // get group key from client
	ctx, cancelCtx := context.WithCancel(context.Background())
	go observeWatch(ctx, cfg, c)
}

func observeWatch(ctx context.Context, cfg *Config, c client.Client) {
	rch := c.Watch(ctx)
	for {
		select {
		case result, ok := <-rch:
			if !ok {
				return
			}
			actual := time.Now().Unix()
			expected := beacon.TimeOfRound(cfg.Group.Period, cfg.Group.GenesisTime, result.Round())
			watchLatency.Set(float64(expected - actual))
		case <-ctx.Done():
			return
		}
	}
}
