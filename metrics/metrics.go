package metrics

import (
	"fmt"
	"net/http"
	_ "net/http/pprof" // adds default pprof endpoint at /debug/pprof
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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

// Start starts a prometheus metrics server with debug endpoints.
func Start(metricsPort int) {
	fmt.Printf("drand: starting metrics server on port %v", metricsPort)
	fmt.Printf("%v", http.ListenAndServe(":"+strconv.Itoa(metricsPort), nil))
}
