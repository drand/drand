package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/drand/drand/key"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/urfave/cli/v2"
)

var urlFlag = &cli.StringFlag{
	Name:  "url",
	Usage: "Root URL for fetching randomness.",
}

var groupKeyFlag = &cli.StringFlag{
	Name:  "group-key",
	Usage: "Path to TOML file containing the group key.",
}

var metricsFlag = &cli.StringFlag{
	Name:  "metrics",
	Usage: "Server address for Prometheus metrics.",
	Value: ":8080",
}

var gatewayFlag = &cli.StringFlag{
	Name:  "gateway",
	Usage: "Push gateway for Prometheus metrics.",
}

var nameFlag = &cli.StringFlag{
	Name:  "name",
	Usage: "The name of this observer node in the metrics system.",
}

func main() {
	app := &cli.App{
		Name:   "observe",
		Usage:  "Drand client for observing metrics",
		Flags:  []cli.Flag{urlFlag, groupKeyFlag, metricsFlag, nameFlag, gatewayFlag},
		Action: Observe,
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

// Observe connects to Drand's distribution network and records metrics from the client's point of view.
func Observe(c *cli.Context) error {
	// set URLs
	cfg := &Config{}
	if c.IsSet(urlFlag.Name) {
		cfg.URL = []string{c.String(urlFlag.Name)}
	}
	// read group key from file
	if !c.IsSet(groupKeyFlag.Name) {
		return fmt.Errorf("group key file is not specified")
	}
	var groupKey key.Group
	if err := key.Load(c.String(groupKeyFlag.Name), &groupKey); err != nil {
		return fmt.Errorf("reading group file (%v)", err)
	}
	cfg.Group = &groupKey
	// read metrics bind address
	if c.IsSet(metricsFlag.Name) {
		cfg.MetricsAddr = c.String(metricsFlag.Name)
	} else {
		cfg.MetricsAddr = ":8080"
	}
	// read metrics push gateay address
	if c.IsSet(gatewayFlag.Name) {
		cfg.MetricsGateway = c.String(gatewayFlag.Name)
	}
	// read name
	if !c.IsSet(nameFlag.Name) {
		return fmt.Errorf("observer node name not set")
	}
	cfg.Name = c.String(nameFlag.Name)

	// register prometheus metrics
	watchLatency := promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "drand_client_observation",
		Subsystem: cfg.Name,
		Name:      "watch_latency",
		Help:      "Duration between time round received and time round expected.",
	})

	go StartObserving(cfg, watchLatency)

	if cfg.MetricsGateway != "" {
		go pushObservations(cfg, watchLatency)
	}
	if cfg.MetricsAddr != "" {
		http.Handle("/metrics", promhttp.Handler())
		log.Fatal(http.ListenAndServe(cfg.MetricsAddr, nil))
	}
	<-(chan int)(nil)
	return nil
}

func pushObservations(cfg *Config, watchLatency prometheus.Gauge) {
	p := push.New(cfg.MetricsGateway, "drand_client_observations_push").Collector(watchLatency)
	for {
		time.Sleep(cfg.Group.Period)
		if err := p.Push(); err != nil {
			log.Printf("prometheus gateway push (%v)", err)
		}
	}
}
