package metrics

import (
	"fmt"
	"net/http"
	"runtime"

	"github.com/drand/drand/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// PrivateMetrics about the internal world (go process, private stuff)
	PrivateMetrics = prometheus.NewRegistry()
	// HTTPMetrics about the public surface area (http requests, cdn stuff)
	HTTPMetrics = prometheus.NewRegistry()
	// GroupMetrics about the group surface (grp, group-member stuff)
	GroupMetrics = prometheus.NewRegistry()

	// APICallCounter (Group) how many grpc calls
	APICallCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "api_call_counter",
		Help: "Number of API calls that we have received",
	}, []string{"api_method"})
	// GroupDialFailures (Group) how manuy failures connecting outbound
	GroupDialFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dial_failures",
		Help: "Number of times there have been network connection issues",
	}, []string{"peer_address"})
	// GroupConnections (Group) how many GrpcClient connections are present
	GroupConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "group_connections",
		Help: "Number of peers with current GrpcClient connections",
	})

	// HTTPCallCounter (HTTP) how many http requests
	HTTPCallCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_call_counter",
		Help: "Number of HTTP calls received",
	}, []string{"code", "method"})
	// HTTPLatency (HTTP) how long http request handling takes
	HTTPLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "http_resopnse_duration",
		Help:        "histogram of request latencies",
		Buckets:     prometheus.DefBuckets,
		ConstLabels: prometheus.Labels{"handler": "http"},
	}, []string{"method"})
	// HTTPInFlight (HTTP) how many http requests exist
	HTTPInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "http_in_flight",
		Help: "A gauge of requests currently being served.",
	})

	metricsBound = false
)

func bindMetrics() {
	if metricsBound {
		return
	}
	metricsBound = true

	// The private go-level metrics live in private.
	PrivateMetrics.Register(prometheus.NewGoCollector())
	PrivateMetrics.Register(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	// Group metrics
	group := []prometheus.Collector{
		APICallCounter,
		GroupDialFailures,
		GroupConnections,
	}
	for _, c := range group {
		GroupMetrics.Register(c)
		PrivateMetrics.Register(c)
	}

	// HTTP metrics
	http := []prometheus.Collector{
		HTTPCallCounter,
		HTTPLatency,
		HTTPInFlight,
	}
	for _, c := range http {
		HTTPMetrics.Register(c)
		PrivateMetrics.Register(c)
	}
}

// Start starts a prometheus metrics server with debug endpoints.
func Start(metricsBind string, pprof http.Handler) {
	log.DefaultLogger.Debug("metrics", "private listener started", "at", metricsBind)
	bindMetrics()

	s := http.Server{Addr: metricsBind}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(PrivateMetrics, promhttp.HandlerOpts{Registry: PrivateMetrics}))

	if pprof != nil {
		mux.Handle("/debug/pprof", pprof)
	}

	mux.HandleFunc("/debug/gc", func(w http.ResponseWriter, req *http.Request) {
		runtime.GC()
		fmt.Fprintf(w, "GC run complete")
	})
	s.Handler = mux
	log.DefaultLogger.Warn("metrics", "listen finished", "err", s.ListenAndServe())
}

// GroupHandler provides metrics shared to other group members
// This HTTP handler, which would typically be mounted at `/metrics` exposes `GroupMetrics`
func GroupHandler() http.Handler {
	return promhttp.HandlerFor(GroupMetrics, promhttp.HandlerOpts{Registry: GroupMetrics})
}
