// Package client-observer provides a Drand client which observes latencies and reports them via Prometheus.
package main

import (
	"context"
	"log"
	"time"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/client"
	"github.com/drand/drand/key"
	"github.com/prometheus/client_golang/prometheus"
)

// Config configures a client observer node.
type Config struct {
	// Group is the  group key for Drand.
	// Group must be non-nil, so that the observer can compute the expected time of each round.
	Group *key.Group
	// URL is a list of REST endpoint URLs
	URL []string
	// MetricsAddr is the address where the metrics server binds.
	MetricsAddr string
	// Name is the name under which this node will report metrics.
	Name string
}

// StartObserving listens to incoming randomness and records metrics.
func StartObserving(cfg *Config, watchLatency prometheus.Gauge) {
	c, err := client.New(
		client.WithGroup(cfg.Group),
		client.WithHTTPEndpoints(cfg.URL),
	)
	if err != nil {
		log.Fatalf("drand client init (%v)", err)
	}

	observeWatch(context.Background(), cfg, c, watchLatency)
}

func observeWatch(ctx context.Context, cfg *Config, c client.Client, watchLatency prometheus.Gauge) {
	rch := c.Watch(ctx)
	for {
		select {
		case result, ok := <-rch:
			if !ok {
				return
			}
			// compute the latency metric
			actual := time.Now().Unix()
			expected := beacon.TimeOfRound(cfg.Group.Period, cfg.Group.GenesisTime, result.Round())
			watchLatency.Set(float64(expected - actual))
		case <-ctx.Done():
			return
		}
	}
}
