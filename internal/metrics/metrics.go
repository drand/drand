package metrics

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	common2 "github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/log"
)

var (
	// PrivateMetrics about the internal world (go process, private stuff)
	PrivateMetrics = prometheus.NewRegistry()
	// HTTPMetrics about the public surface area (http requests, cdn stuff)
	HTTPMetrics = prometheus.NewRegistry()
	// GroupMetrics about the group surface (grp, group-member stuff)
	GroupMetrics = prometheus.NewRegistry()
	// ClientMetrics about the drand client requests to servers
	ClientMetrics = prometheus.NewRegistry()

	// APICallCounter (Group) how many grpc calls
	APICallCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "api_call_counter",
		Help: "Number of API calls that we have received",
	}, []string{"api_method"})

	// GroupDialFailures (Group) how many failures connecting outbound
	GroupDialFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dial_failures",
		Help: "Number of times there have been network connection issues",
	}, []string{"peer_address"})

	// OutgoingConnections (Group) how many GrpcClient connections are present
	OutgoingConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "outgoing_group_connections",
		Help: "Number of peers with current outgoing GrpcClient connections",
	})

	GroupSize = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "group_size",
		Help: "Number of peers in the current group",
	}, []string{"beacon_id"})

	GroupThreshold = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "group_threshold",
		Help: "Number of shares needed for beacon reconstruction",
	}, []string{"beacon_id"})

	// BeaconDiscrepancyLatency (Group) millisecond duration between time beacon created and
	// calculated time of round.
	BeaconDiscrepancyLatency = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "beacon_discrepancy_latency",
		Help: "Discrepancy between beacon creation time and calculated round time",
	}, []string{"beacon_id"})

	// LastBeaconRound is the most recent round (as also seen at /health) stored.
	LastBeaconRound = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "last_beacon_round",
		Help: "Last locally stored beacon",
	}, []string{"beacon_id"})

	// HTTPCallCounter (HTTP) how many http requests
	HTTPCallCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_call_counter",
		Help: "Number of HTTP calls received",
	}, []string{"code", "method"})
	// HTTPLatency (HTTP) how long http request handling takes
	HTTPLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "http_response_duration",
		Help:        "histogram of request latencies",
		Buckets:     prometheus.DefBuckets,
		ConstLabels: prometheus.Labels{"handler": "http"},
	}, []string{"method"})
	// HTTPInFlight (HTTP) how many http requests exist
	HTTPInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "http_in_flight",
		Help: "A gauge of requests currently being served.",
	})

	// Client observation metrics

	// ClientWatchLatency measures the latency of the watch channel from the client's perspective.
	ClientWatchLatency = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "client_watch_latency",
		Help: "Duration between time round received and time round expected.",
	})

	// ClientHTTPHeartbeatSuccess measures the success rate of HTTP hearbeat randomness requests.
	ClientHTTPHeartbeatSuccess = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "client_http_heartbeat_success",
		Help: "Number of successful HTTP heartbeats.",
	}, []string{"http_address"})

	// ClientHTTPHeartbeatFailure measures the number of times HTTP heartbeats fail.
	ClientHTTPHeartbeatFailure = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "client_http_heartbeat_failure",
		Help: "Number of unsuccessful HTTP heartbeats.",
	}, []string{"http_address"})

	// ClientHTTPHeartbeatLatency measures the randomness latency of an HTTP source.
	ClientHTTPHeartbeatLatency = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "client_http_heartbeat_latency",
		Help: "Randomness latency of an HTTP source.",
	}, []string{"http_address"})

	// ClientInFlight measures how many active requests have been made
	ClientInFlight = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "client_in_flight",
		Help: "A gauge of in-flight drand client http requests.",
	},
		[]string{"url"},
	)

	// ClientRequests measures how many total requests have been made
	ClientRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "client_api_requests_total",
			Help: "A counter for requests from the drand client.",
		},
		[]string{"code", "method", "url"},
	)

	// ClientDNSLatencyVec tracks the observed DNS resolution times
	ClientDNSLatencyVec = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "client_dns_duration_seconds",
			Help:    "Client drand dns latency histogram.",
			Buckets: []float64{.005, .01, .025, .05},
		},
		[]string{"event", "url"},
	)

	// ClientTLSLatencyVec tracks observed TLS connection times
	ClientTLSLatencyVec = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "client_tls_duration_seconds",
			Help:    "Client drand tls latency histogram.",
			Buckets: []float64{.05, .1, .25, .5},
		},
		[]string{"event", "url"},
	)

	// ClientLatencyVec tracks raw http request latencies
	ClientLatencyVec = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "client_request_duration_seconds",
			Help:    "A histogram of client request latencies.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"url"},
	)

	dkgEpoch = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "dkg_epoch",
			Help: "The epoch of any currently in progress or completed DKGs",
		},
		[]string{"beacon_id"},
	)

	// dkgState (Group) tracks DKG status changes
	dkgState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dkg_state",
		Help: "DKG state: 0-Not Started, 1-Waiting, 2-In Progress, 3-Done, 4-Unknown, 5-Shutdown",
	}, []string{"beacon_id"})

	// DKGStateTimestamp (Group) tracks the time when the reshare status changes
	dkgStateTimestamp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dkg_state_timestamp",
		Help: "Timestamp when the DKG state last changed",
	}, []string{"beacon_id"})

	// dkgLeader (Group) tracks whether this node is the leader during DKG
	dkgLeader = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dkg_leader",
		Help: "Is this node the leader during DKG? 0-false, 1-true",
	}, []string{"beacon_id"})

	// reshareState (Group) tracks reshare status changes
	reshareState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "reshare_state",
		Help: "Reshare state: 0-Idle, 1-Waiting, 2-In Progress, 3-Unknown, 4-Shutdown",
	}, []string{"beacon_id"})

	// reshareStateTimestamp (Group) tracks the time when the reshare status changes
	reshareStateTimestamp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "reshare_state_timestamp",
		Help: "Timestamp when the reshare state last changed",
	}, []string{"beacon_id"})

	// reshareLeader (Group) tracks whether this node is the leader during Reshare
	reshareLeader = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "reshare_leader",
		Help: "Is this node the leader during Reshare? 0-false, 1-true",
	}, []string{"beacon_id"})

	// drandBuildTime (Group) emits the timestamp when the binary was built in Unix time.
	drandBuildTime = prometheus.NewUntypedFunc(prometheus.UntypedOpts{
		Name:        "drand_build_time",
		Help:        "Timestamp when the binary was built in seconds since the Epoch",
		ConstLabels: map[string]string{"build": common2.COMMIT, "version": common2.GetAppVersion().String()},
	}, func() float64 { return float64(getBuildTimestamp(common2.BUILDDATE)) })

	// IsDrandNode (Group) is 1 for drand nodes, 0 for relays
	IsDrandNode = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "is_drand_node",
		Help: "1 for drand nodes, not emitted for relays",
	})

	// DrandStorageBackend reports the database the node is running with
	DrandStorageBackend = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "drand_node_db",
		Help: "The database type the node is running with. 1=bolt-trimmed, 2=postgres, 3=memdb, 4=bolt-untrimmed",
	}, []string{"beaconID", "db_type"})

	// OutgoingConnectionState (Group) tracks the state of an outgoing connection, using the states from
	// https://github.com/grpc/grpc-go/blob/8075dd35d2738b352c4355b4b353dc1e9183bea7/connectivity/connectivity.go#L51-L62
	// Due to the fact that grpc-go doesn't support adding a listener for state tracking, this is
	// emitted only when getting a connection to the remote host. This means that:
	// * If a non-PL host is unable to connect to a PL host, the metric will not be sent to InfluxDB
	// * The state might not be up to date (e.g. the remote host is disconnected but we haven't
	//   tried to connect to it)
	OutgoingConnectionState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "outgoing_connection_state",
		Help: "State of an outgoing connection. 0=Idle, 1=Connecting, 2=Ready, 3=Transient Failure, 4=Shutdown",
	}, []string{"remote_host"})

	// DrandStartTimestamp (group) contains the timestamp in seconds since the epoch of the drand process startup
	DrandStartTimestamp = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "drand_start_timestamp",
		Help: "Timestamp when the drand process started up in seconds since the Epoch",
	})

	ErrorSendingPartialCounter = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "error_sending_partial",
		Help: "Number of errors sending partial beacons to nodes. A good proxy for whether nodes are up or down. " +
			"1 = Error occurred, 0 = No error occurred",
	}, []string{"beaconID", "address"})

	SyncCallbacks = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "sync_total_callbacks",
			Help: "The number of currently active callbacks",
		},
		[]string{"beacon_id"},
	)

	SyncJobs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "sync_total_jobs",
			Help: "The number of currently active jobs",
		},
		[]string{"beacon_id"},
	)

	metricsBound sync.Once
)

func bindMetrics(l log.Logger) {
	// The private go-level metrics live in private.
	if err := PrivateMetrics.Register(collectors.NewGoCollector()); err != nil {
		l.Errorw("error in bindMetrics", "metrics", "goCollector", "err", err)
		return
	}
	if err := PrivateMetrics.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})); err != nil {
		l.Errorw("error in bindMetrics", "metrics", "processCollector", "err", err)
		return
	}

	// Group metrics
	group := []prometheus.Collector{
		APICallCounter,
		GroupDialFailures,
		OutgoingConnections,
		GroupSize,
		GroupThreshold,
		BeaconDiscrepancyLatency,
		LastBeaconRound,
		drandBuildTime,
		dkgEpoch,
		dkgState,
		dkgStateTimestamp,
		dkgLeader,
		reshareState,
		reshareStateTimestamp,
		reshareLeader,
		OutgoingConnectionState,
		IsDrandNode,
		DrandStartTimestamp,
		DrandStorageBackend,
		ErrorSendingPartialCounter,
		SyncCallbacks,
		SyncJobs,
	}
	for _, c := range group {
		if err := GroupMetrics.Register(c); err != nil {
			l.Errorw("error in bindMetrics", "metrics", "bindMetrics", "err", err)
			return
		}
		if err := PrivateMetrics.Register(c); err != nil {
			l.Errorw("error in bindMetrics", "metrics", "bindMetrics", "err", err)
			return
		}
	}

	// HTTP metrics
	httpMetrics := []prometheus.Collector{
		HTTPCallCounter,
		HTTPLatency,
		HTTPInFlight,
	}
	for _, c := range httpMetrics {
		if err := HTTPMetrics.Register(c); err != nil {
			l.Errorw("error in bindMetrics", "metrics", "bindMetrics", "err", err)
			return
		}
		if err := PrivateMetrics.Register(c); err != nil {
			l.Errorw("error in bindMetrics", "metrics", "bindMetrics", "err", err)
			return
		}
	}

	// Client metrics
	if err := RegisterClientMetrics(ClientMetrics); err != nil {
		l.Errorw("error in bindMetrics", "metrics", "bindMetrics", "err", err)
		return
	}
	if err := RegisterClientMetrics(PrivateMetrics); err != nil {
		l.Errorw("error in bindMetrics", "metrics", "bindMetrics", "err", err)
		return
	}
}

// RegisterClientMetrics registers drand client metrics with the given registry
func RegisterClientMetrics(r prometheus.Registerer) error {
	// Client metrics
	client := []prometheus.Collector{
		ClientDNSLatencyVec,
		ClientInFlight,
		ClientLatencyVec,
		ClientRequests,
		ClientTLSLatencyVec,
		ClientWatchLatency,
		ClientHTTPHeartbeatSuccess,
		ClientHTTPHeartbeatFailure,
		ClientHTTPHeartbeatLatency,
	}
	for _, c := range client {
		if err := r.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// Handler abstracts a helper for relaying http requests to a group peer
type Handler func(ctx context.Context, addr string) (http.Handler, error)

// Client is the same as net.MetricsClient but avoids cyclic dependencies in our metric and net code.
type Client interface {
	GetMetrics(ctx context.Context, p string) (string, error)
}

// Start starts a prometheus metrics server with debug endpoints. If metricsBind is 0 it will use an available port.
func Start(logger log.Logger, metricsBind string, pprof http.Handler, cli Client) net.Listener {
	logger.Infow("metrics starting", "desired_port", metricsBind)

	metricsBound.Do(func() {
		bindMetrics(logger)
	})

	// handle metricsBind being just a port value
	if !strings.Contains(metricsBind, ":") {
		metricsBind = "127.0.0.1:" + metricsBind
	}
	//nolint:noctx
	l, err := net.Listen("tcp", metricsBind)
	if err != nil {
		logger.Warnw("", "metrics", "listen failed", "err", err)
		return nil
	}
	logger.Infow("metric listener started", "addr", l.Addr())

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(PrivateMetrics, promhttp.HandlerOpts{Registry: PrivateMetrics}))

	mux.Handle("/peer/", newRemotePeerHandler(logger, cli))

	if pprof != nil {
		mux.Handle("/debug/pprof/", pprof)
	}

	mux.HandleFunc("/debug/gc", func(w http.ResponseWriter, _ *http.Request) {
		runtime.GC()
		fmt.Fprintf(w, "GC run complete")
	})

	s := http.Server{Addr: l.Addr().String(), ReadHeaderTimeout: 3 * time.Second, Handler: mux}
	go func() {
		logger.Warnw("", "metrics", "listen finished", "err", s.Serve(l))
	}()
	return l
}

// remotePeerHandler is a structure that handles all peers that
// this node is connected to regardless of which group they are a part of.
type remotePeerHandler struct {
	log    log.Logger
	client Client
}

// newRemotePeerHandler creates a new remotePeerHandler from a MetricsClient
func newRemotePeerHandler(logger log.Logger, cli Client) *remotePeerHandler {
	return &remotePeerHandler{
		log:    logger,
		client: cli,
	}
}

// ServeHTTP serves the metrics for the peer whose address is given in the URI.
// It assumes that the URI is in the form /peer/<peer_address>
func (l *remotePeerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	addr := strings.Replace(r.URL.Path, "/peer/", "", 1)
	if index := strings.Index(addr, "/"); index != -1 {
		addr = addr[:index]
	}

	metrics, err := l.client.GetMetrics(r.Context(), addr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}

	l.log.Debugw("Received metrics through GRPC", "from", addr, "err", err)

	_, err = w.Write([]byte(metrics))
	if err != nil {
		l.log.Errorw("Error serving remote metrics for peer", "addr", addr)
	}
}

func getBuildTimestamp(buildDate string) int64 {
	if buildDate == "" {
		return 0
	}

	layout := "02/01/2006@15:04:05"
	t, err := time.Parse(layout, buildDate)
	if err != nil {
		return 0
	}
	return t.Unix()
}

// DKGStateChange emits appropriate dkgState, dkgStateTimestamp and dkgLeader metrics
func DKGStateChange(beaconID string, epoch uint32, leader bool, state uint32) {
	leading := 0.0
	if leader {
		leading = 1.0
	}
	dkgEpoch.WithLabelValues(beaconID).Set(float64(epoch))
	dkgState.WithLabelValues(beaconID).Set(float64(state))
	dkgStateTimestamp.WithLabelValues(beaconID).SetToCurrentTime()
	dkgLeader.WithLabelValues(beaconID).Set(leading)
}

func ErrorSendingPartial(beaconID, address string) {
	ErrorSendingPartialCounter.WithLabelValues(beaconID, address).Set(1)
}

func SuccessfulPartial(beaconID, address string) {
	ErrorSendingPartialCounter.WithLabelValues(beaconID, address).Set(0)
}
