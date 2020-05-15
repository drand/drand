package metrics

import (
	"fmt"
	"net/http"
	_ "net/http/pprof" // adds default pprof endpoint at /debug/pprof
	"runtime"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	APICallCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "api_call_counter",
		Help: "Number of API calls that we have received",
	}, []string{"api_method"})
	DialFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dial_failures",
		Help: "Number of times there have been network connection issues",
	}, []string{"peer_index"})
)

// Register metrics and custom debug endpoints.
func init() {
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/debug/gc", func(w http.ResponseWriter, req *http.Request) {
		runtime.GC()
		fmt.Fprintf(w, "GC run complete")
	})
}

// Start starts a prometheus metrics server with debug endpoints.
func Start(metricsPort int) {
	fmt.Printf("drand: starting metrics server on port %v", metricsPort)
	fmt.Printf("%v", http.ListenAndServe(":"+strconv.Itoa(metricsPort), nil))
}
