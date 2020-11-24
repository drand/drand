package http

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	json "github.com/nikkolasg/hexjson"
)

const (
	watchConnectBackoff = 300 * time.Millisecond
	catchupExpiryFactor = 2
)

var (
	// Timeout for how long to wait for the drand.PublicClient before timing out
	reqTimeout = 5 * time.Second
)

// New creates an HTTP handler for the public Drand API
func New(ctx context.Context, c client.Client, version string, logger log.Logger) (http.Handler, error) {
	if logger == nil {
		logger = log.DefaultLogger()
	}
	handler := handler{
		timeout:     reqTimeout,
		client:      c,
		chainInfo:   nil,
		log:         logger,
		pending:     nil,
		context:     ctx,
		latestRound: 0,
		version:     version,
	}

	mux := http.NewServeMux()
	//TODO: aggregated bulk round responses.
	mux.HandleFunc("/public/latest", withCommonHeaders(version, handler.LatestRand))
	mux.HandleFunc("/public/", withCommonHeaders(version, handler.PublicRand))
	mux.HandleFunc("/info", withCommonHeaders(version, handler.ChainInfo))
	mux.HandleFunc("/health", withCommonHeaders(version, handler.Health))

	instrumented := promhttp.InstrumentHandlerCounter(
		metrics.HTTPCallCounter,
		promhttp.InstrumentHandlerDuration(
			metrics.HTTPLatency,
			promhttp.InstrumentHandlerInFlight(
				metrics.HTTPInFlight,
				mux)))
	return instrumented, nil
}

func withCommonHeaders(version string, h func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", version)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		h(w, r)
	}
}

type handler struct {
	timeout time.Duration
	client  client.Client
	// NOTE: should only be accessed via getChainInfo
	chainInfo   *chain.Info
	chainInfoLk sync.RWMutex
	log         log.Logger

	// synchronization for blocking writes until randomness available.
	pendingLk   sync.RWMutex
	startOnce   sync.Once
	pending     []chan []byte
	context     context.Context
	latestRound uint64
	version     string
}

func (h *handler) start() {
	h.pendingLk.Lock()
	defer h.pendingLk.Unlock()
	h.pending = make([]chan []byte, 0)
	ready := make(chan bool)
	go h.Watch(h.context, ready)
	<-ready
}

func (h *handler) Watch(ctx context.Context, ready chan bool) {
RESET:
	stream := h.client.Watch(context.Background())

	// signal that the watch is ready
	select {
	case ready <- true:
	default:
	}

	for {
		next, ok := <-stream
		if !ok {
			h.log.Warn("http_server", "random stream round failed")
			h.pendingLk.Lock()
			h.latestRound = 0
			h.pendingLk.Unlock()
			// backoff on failures a bit to not fall into a tight loop.
			// TODO: tuning.
			time.Sleep(watchConnectBackoff)
			goto RESET
		}

		b, _ := json.Marshal(next)

		h.pendingLk.Lock()
		if h.latestRound+1 != next.Round() && h.latestRound != 0 {
			// we missed a round, or similar. don't send bad data to peers.
			h.log.Warn("http_server", "unexpected round for watch", "err", fmt.Sprintf("expected %d, saw %d", h.latestRound+1, next.Round()))
			b = []byte{}
		}
		h.latestRound = next.Round()
		pending := h.pending
		h.pending = make([]chan []byte, 0)

		for _, waiter := range pending {
			waiter <- b
		}
		h.pendingLk.Unlock()

		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func (h *handler) getChainInfo(ctx context.Context) *chain.Info {
	h.chainInfoLk.RLock()
	if h.chainInfo != nil {
		info := h.chainInfo
		h.chainInfoLk.RUnlock()
		return info
	}
	h.chainInfoLk.RUnlock()

	h.chainInfoLk.Lock()
	defer h.chainInfoLk.Unlock()
	if h.chainInfo != nil {
		return h.chainInfo
	}

	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	info, err := h.client.Info(ctx)
	if err != nil {
		h.log.Warn("msg", "chain info fetch failed", "err", err)
		return nil
	}
	if info == nil {
		h.log.Warn("msg", "chain info fetch didn't return group info")
		return nil
	}
	h.chainInfo = info
	return info
}

func (h *handler) getRand(ctx context.Context, info *chain.Info, round uint64) ([]byte, error) {
	h.startOnce.Do(h.start)
	// First see if we should get on the synchronized 'wait for next release' bandwagon.
	block := false
	h.pendingLk.RLock()
	block = (h.latestRound+1 == round) && h.latestRound != 0
	h.pendingLk.RUnlock()
	// If so, prepare, and if we're still sync'd, add ourselves to the list of waiters.
	if block {
		ch := make(chan []byte, 1)
		defer close(ch)
		h.pendingLk.Lock()
		block = (h.latestRound+1 == round) && h.latestRound != 0
		if block {
			h.pending = append(h.pending, ch)
		}
		h.pendingLk.Unlock()
		// If that was successful, we can now block until we're notified.
		if block {
			select {
			case r := <-ch:
				return r, nil
			case <-ctx.Done():
				h.pendingLk.Lock()
				defer h.pendingLk.Unlock()
				for i, c := range h.pending {
					if c == ch {
						h.pending = append(h.pending[:i], h.pending[i+1:]...)
						break
					}
				}
				select {
				case <-ch:
				default:
				}
				return nil, ctx.Err()
			}
		}
	}

	// make sure we aren't going to ask for a round that doesn't exist yet.
	if time.Unix(chain.TimeOfRound(info.Period, info.GenesisTime, round), 0).After(time.Now()) {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	resp, err := h.client.Get(ctx, round)

	if err != nil {
		return nil, err
	}

	return json.Marshal(resp)
}

func (h *handler) PublicRand(w http.ResponseWriter, r *http.Request) {
	// Get the round.
	round := strings.Replace(r.URL.Path, "/public/", "", 1)
	roundN, err := strconv.ParseUint(round, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		h.log.Warn("http_server", "failed to parse client round", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path))
		return
	}

	info := h.getChainInfo(r.Context())
	roundExpectedTime := time.Now()
	if info == nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warn("http_server", "failed to get randomness", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "err", err)
		return
	}

	roundExpectedTime = time.Unix(chain.TimeOfRound(info.Period, info.GenesisTime, roundN), 0)

	if roundExpectedTime.After(time.Now().Add(info.Period)) {
		timeToExpected := int(time.Until(roundExpectedTime).Seconds())
		w.WriteHeader(http.StatusNotFound)
		w.Header().Set("Cache-Control", fmt.Sprintf("public, must-revalidate, max-age=%d", timeToExpected))
		h.log.Warn("http_server", "request in the future", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path))
		return
	}

	data, err := h.getRand(r.Context(), info, roundN)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warn("http_server", "failed to get randomness", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "err", err)
		return
	}
	if data == nil {
		w.WriteHeader(http.StatusNotFound)
		w.Header().Set("Cache-Control", "must-revalidate, no-cache, max-age=0")
		h.log.Warn("http_server", "request in the future", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path))
		return
	}

	// Headers per recommendation for static assets at
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Cache-Control
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	w.Header().Set("Expires", time.Now().Add(7*24*time.Hour).Format(http.TimeFormat))
	http.ServeContent(w, r, "rand.json", roundExpectedTime, bytes.NewReader(data))
}

func (h *handler) LatestRand(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	resp, err := h.client.Get(ctx, 0)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warn("http_server", "failed to get randomness", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "err", err)
		return
	}

	data, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warn("http_server", "failed to marshal randomness", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "err", err)
		return
	}

	info := h.getChainInfo(r.Context())
	roundTime := time.Now()
	nextTime := time.Now()
	if info != nil {
		roundTime = time.Unix(chain.TimeOfRound(info.Period, info.GenesisTime, resp.Round()), 0)
		next := time.Unix(chain.TimeOfRound(info.Period, info.GenesisTime, resp.Round()+1), 0)
		if next.After(nextTime) {
			nextTime = next
		} else {
			nextTime = nextTime.Add(info.Period / catchupExpiryFactor)
		}
	}

	remaining := time.Until(nextTime)
	if remaining > 0 && remaining < info.Period {
		seconds := int(math.Ceil(remaining.Seconds()))
		w.Header().Set("Cache-Control", fmt.Sprintf("max-age:%d, public", seconds))
	} else {
		h.log.Warn("http_server", "latest rand in the past", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "remaining", remaining)
	}

	w.Header().Set("Expires", nextTime.Format(http.TimeFormat))
	w.Header().Set("Last-Modified", roundTime.Format(http.TimeFormat))
	_, _ = w.Write(data)
}

func (h *handler) ChainInfo(w http.ResponseWriter, r *http.Request) {
	info := h.getChainInfo(r.Context())
	if info == nil {
		w.WriteHeader(http.StatusNoContent)
		h.log.Warn("http_server", "failed to serve group", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path))
		return
	}
	var chainBuff bytes.Buffer
	err := info.ToJSON(&chainBuff)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warn("http_server", "failed to marshal group", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "err", err)
		return
	}

	// Headers per recommendation for static assets at
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Cache-Control
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	w.Header().Set("Expires", time.Now().Add(7*24*time.Hour).Format(http.TimeFormat))
	http.ServeContent(w, r, "info.json", time.Unix(info.GenesisTime, 0), bytes.NewReader(chainBuff.Bytes()))
}

func (h *handler) Health(w http.ResponseWriter, r *http.Request) {
	h.startOnce.Do(h.start)

	h.pendingLk.RLock()
	lastSeen := h.latestRound
	h.pendingLk.RUnlock()

	info := h.getChainInfo(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	resp := make(map[string]uint64)
	resp["current"] = lastSeen
	resp["expected"] = 0
	var b []byte

	if info == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		expected := chain.CurrentRound(time.Now().Unix(), info.Period, info.GenesisTime)
		resp["expected"] = expected
		if lastSeen == expected || lastSeen+1 == expected {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}

	b, _ = json.Marshal(resp)
	_, _ = w.Write(b)
}
