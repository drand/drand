package http

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi"
	json "github.com/nikkolasg/hexjson"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/common"
	"github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
)

const (
	watchConnectBackoff = 300 * time.Millisecond
	catchupExpiryFactor = 2
	roundNumBase        = 10
	roundNumSize        = 64
	chainHashParamKey   = "chainHash"
	roundParamKey       = "round"
)

var (
	// Timeout for how long to wait for the drand.PublicClient before timing out
	reqTimeout = 5 * time.Second
)

// DrandHandler keeps the reference to the real http handler used by the server
// to attend to new request, as well as the slice of logic handlers used to process
// a request. Each handler will attend to requests for one beacon process. The chain hash
// is used as key.
type DrandHandler struct {
	httpHandler http.Handler
	beacons     map[string]*BeaconHandler

	timeout time.Duration
	context context.Context
	log     log.Logger
	version string
	state   sync.RWMutex
}

type BeaconHandler struct {
	// NOTE: should only be accessed via getChainInfo
	chainInfo   *chain.Info
	chainInfoLk sync.RWMutex
	log         log.Logger

	// Client to handle beacon
	client client.Client

	// synchronization for blocking writes until randomness available.
	pendingLk   sync.RWMutex
	startOnce   sync.Once
	pending     []chan []byte
	context     context.Context
	latestRound uint64
	version     string
}

// New creates an HTTP handler for the public Drand API
func New(ctx context.Context, version string) (*DrandHandler, error) {
	logger := log.FromContextOrDefault(ctx)

	handler := &DrandHandler{
		timeout: reqTimeout,
		log:     logger,
		context: ctx,
		version: version,
		beacons: make(map[string]*BeaconHandler),
	}

	instrument := func(h http.HandlerFunc, name string) http.HandlerFunc {
		return withCommonHeaders(
			version,
			otelhttp.NewHandler(h, name).ServeHTTP,
		)
	}

	mux := chi.NewMux()

	mux.HandleFunc(
		"/{"+chainHashParamKey+"}/public/latest",
		instrument(handler.LatestRand, chainHashParamKey+".LatestRand"),
	)
	mux.HandleFunc(
		"/{"+chainHashParamKey+"}/public/{"+roundParamKey+"}",
		instrument(handler.PublicRand, chainHashParamKey+".PublicRand"),
	)
	mux.HandleFunc(
		"/{"+chainHashParamKey+"}/info",
		instrument(handler.ChainInfo, chainHashParamKey+".ChainInfo"),
	)
	mux.HandleFunc(
		"/{"+chainHashParamKey+"}/health",
		instrument(handler.Health, chainHashParamKey+".Health"),
	)

	mux.HandleFunc(
		"/public/latest",
		instrument(handler.LatestRand, "LatestRand"),
	)
	mux.HandleFunc(
		"/public/{"+roundParamKey+"}",
		instrument(handler.PublicRand, roundParamKey+".PublicRand"),
	)
	mux.HandleFunc(
		"/info",
		instrument(handler.ChainInfo, "ChainInfo"),
	)
	mux.HandleFunc(
		"/health",
		instrument(handler.Health, "Health"),
	)
	mux.HandleFunc(
		"/chains",
		instrument(handler.ChainHashes, "ChainHashes"),
	)

	handler.httpHandler = promhttp.InstrumentHandlerCounter(
		metrics.HTTPCallCounter,
		promhttp.InstrumentHandlerDuration(
			metrics.HTTPLatency,
			promhttp.InstrumentHandlerInFlight(
				metrics.HTTPInFlight,
				mux)))

	return handler, nil
}

// RegisterNewBeaconHandler add a new handler for a beacon process using its chain hash
func (h *DrandHandler) RegisterNewBeaconHandler(c client.Client, chainHash string) *BeaconHandler {
	h.state.Lock()
	defer h.state.Unlock()

	bh := &BeaconHandler{
		context:     h.context,
		client:      c,
		latestRound: 0,
		pending:     nil,
		chainInfo:   nil,
		version:     h.version,
		log:         h.log,
	}

	h.beacons[chainHash] = bh
	h.log.Infow("New beacon handler registered", "chainHash", chainHash)

	return bh
}

func (h *DrandHandler) GetHTTPHandler() http.Handler {
	return h.httpHandler
}

func (h *DrandHandler) SetHTTPHandler(newHandler http.Handler) {
	h.httpHandler = newHandler
}

func (h *DrandHandler) RemoveBeaconHandler(chainHash string) {
	h.state.Lock()
	defer h.state.Unlock()

	delete(h.beacons, chainHash)
}

func (h *DrandHandler) RegisterDefaultBeaconHandler(bh *BeaconHandler) {
	h.state.Lock()
	defer h.state.Unlock()

	h.beacons[common.DefaultChainHash] = bh
	h.log.Infow("New default beacon handler registered")
}

func withCommonHeaders(version string, h func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", version)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		h(w, r)
	}
}

func (h *DrandHandler) start(bh *BeaconHandler) {
	bh.pendingLk.Lock()
	defer bh.pendingLk.Unlock()

	bh.pending = make([]chan []byte, 0)
	ready := make(chan bool)
	go h.Watch(bh, ready)

	<-ready
}

func (h *DrandHandler) Watch(bh *BeaconHandler, ready chan bool) {
	for {
		select {
		case <-bh.context.Done():
			return
		default:
		}
		h.watchWithTimeout(bh, ready)
	}
}

func (h *DrandHandler) watchWithTimeout(bh *BeaconHandler, ready chan bool) {
	watchCtx, cncl := context.WithCancel(bh.context)
	defer cncl()
	stream := bh.client.Watch(watchCtx)

	// signal that the watch is ready
	select {
	case ready <- true:
	default:
	}

	expectedRoundDelayBackoff := time.Minute
	bh.chainInfoLk.RLock()
	if bh.chainInfo != nil {
		expectedRoundDelayBackoff = bh.chainInfo.Period * 2
	}
	bh.chainInfoLk.RUnlock()
	for {
		var next client.Result
		var ok bool
		select {
		case <-bh.context.Done():
			return
		case next, ok = <-stream:
		case <-time.After(expectedRoundDelayBackoff):
			return
		}
		if !ok {
			h.log.Warnw("", "http_server", "random stream round failed")
			bh.pendingLk.Lock()
			bh.latestRound = 0
			bh.pendingLk.Unlock()
			// backoff on failures a bit to not fall into a tight loop.
			// TODO: tuning.
			time.Sleep(watchConnectBackoff)
			return
		}

		b, _ := json.Marshal(next)

		bh.pendingLk.Lock()
		if bh.latestRound+1 != next.Round() && bh.latestRound != 0 {
			// we missed a round, or similar. don't send bad data to peers.
			h.log.Warnw("", "http_server", "unexpected round for watch", "err", fmt.Sprintf("expected %d, saw %d", bh.latestRound+1, next.Round()))
			b = []byte{}
		}
		bh.latestRound = next.Round()
		pending := bh.pending
		bh.pending = make([]chan []byte, 0)

		for _, waiter := range pending {
			waiter <- b
		}
		bh.pendingLk.Unlock()
	}
}

func (h *DrandHandler) getChainInfo(ctx context.Context, chainHash []byte) (*chain.Info, error) {
	bh, err := h.getBeaconHandler(chainHash)
	if err != nil {
		return nil, err
	}

	bh.chainInfoLk.RLock()
	if bh.chainInfo != nil {
		info := bh.chainInfo
		bh.chainInfoLk.RUnlock()
		return info, nil
	}
	bh.chainInfoLk.RUnlock()

	bh.chainInfoLk.Lock()
	defer bh.chainInfoLk.Unlock()

	if bh.chainInfo != nil {
		return bh.chainInfo, nil
	}

	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	info, err := bh.client.Info(ctx)
	if err != nil {
		h.log.Warnw("", "msg", "chain info fetch failed", "err", err)
		return nil, err
	}
	if info == nil {
		h.log.Warnw("", "msg", "chain info fetch didn't return group info")
		return nil, fmt.Errorf("chain info fetch didn't return group info")
	}
	bh.chainInfo = info
	return info, nil
}

func (h *DrandHandler) getRand(ctx context.Context, chainHash []byte, info *chain.Info, round uint64) ([]byte, error) {
	bh, err := h.getBeaconHandler(chainHash)
	if err != nil {
		return nil, err
	}

	bh.startOnce.Do(func() {
		h.start(bh)
	})

	// First see if we should get on the synchronized 'wait for next release' bandwagon.
	var block bool
	bh.pendingLk.RLock()
	block = (bh.latestRound+1 == round) && bh.latestRound != 0
	bh.pendingLk.RUnlock()
	// If so, prepare, and if we're still sync'd, add ourselves to the list of waiters.
	if block {
		ch := make(chan []byte, 1)
		defer close(ch)
		bh.pendingLk.Lock()
		block = (bh.latestRound+1 == round) && bh.latestRound != 0
		if block {
			bh.pending = append(bh.pending, ch)
		}
		bh.pendingLk.Unlock()

		// If that was successful, we can now block until we're notified.
		if block {
			select {
			case r := <-ch:
				return r, nil
			case <-ctx.Done():
				bh.pendingLk.Lock()
				defer bh.pendingLk.Unlock()
				for i, c := range bh.pending {
					if c == ch {
						bh.pending = append(bh.pending[:i], bh.pending[i+1:]...)
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
		h.log.Warnw("requested round is in the future")
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	resp, err := bh.client.Get(ctx, round)

	if err != nil {
		return nil, err
	}

	return json.Marshal(resp)
}

func (h *DrandHandler) PublicRand(w http.ResponseWriter, r *http.Request) {
	// Get the round.
	roundN, err := readRound(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		h.log.Warnw("", "http_server", "failed to parse client round", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path))
		return
	}

	if roundN == 0 {
		h.LatestRand(w, r)
		return
	}

	chainHashHex, err := readChainHash(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, err = h.getBeaconHandler(chainHashHex)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	info, err := h.getChainInfo(r.Context(), chainHashHex)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warnw("", "http_server", "failed to get randomness", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "err", err)
		return
	}

	roundExpectedTime := time.Unix(chain.TimeOfRound(info.Period, info.GenesisTime, roundN), 0)

	if roundExpectedTime.After(time.Now().Add(info.Period)) {
		timeToExpected := int(time.Until(roundExpectedTime).Seconds())
		w.Header().Set("Cache-Control", fmt.Sprintf("public, must-revalidate, max-age=%d", timeToExpected))
		w.WriteHeader(http.StatusNotFound)
		h.log.Warnw("", "http_server", "request in the future", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path))
		return
	}

	data, err := h.getRand(r.Context(), chainHashHex, info, roundN)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warnw("", "http_server", "failed to get randomness", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "err", err)
		return
	}
	if data == nil {
		w.Header().Set("Cache-Control", "must-revalidate, no-cache, max-age=0")
		w.WriteHeader(http.StatusNotFound)
		h.log.Warnw("", "http_server", "request in the future", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path))
		return
	}

	// Headers per recommendation for static assets at
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Cache-Control
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	w.Header().Set("Expires", time.Now().Add(7*24*time.Hour).Format(http.TimeFormat))
	http.ServeContent(w, r, "rand.json", roundExpectedTime, bytes.NewReader(data))
}

func (h *DrandHandler) LatestRand(w http.ResponseWriter, r *http.Request) {
	chainHashHex, err := readChainHash(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	bh, err := h.getBeaconHandler(chainHashHex)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	resp, err := bh.client.Get(ctx, 0)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warnw("", "http_server", "failed to get randomness", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "err", err)
		return
	}

	data, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warnw("", "http_server", "failed to marshal randomness", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "err", err)
		return
	}

	info, err := h.getChainInfo(r.Context(), chainHashHex)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warn(
			"http_server", "unable to get info from chainhash",
			"chainHashHex", chainHashHex,
			"client", r.RemoteAddr,
			"req", url.PathEscape(r.URL.Path),
			"err", err,
		)
		return
	}

	roundTime := time.Unix(chain.TimeOfRound(info.Period, info.GenesisTime, resp.Round()), 0)
	nextTime := time.Now()

	next := time.Unix(chain.TimeOfRound(info.Period, info.GenesisTime, resp.Round()+1), 0)
	if next.After(nextTime) {
		nextTime = next
	} else {
		nextTime = nextTime.Add(info.Period / catchupExpiryFactor)
	}

	remaining := time.Until(nextTime)
	if remaining > 0 && remaining < info.Period {
		seconds := int(math.Ceil(remaining.Seconds()))
		w.Header().Set("Cache-Control", fmt.Sprintf("max-age:%d, public", seconds))
	} else {
		h.log.Warnw("", "http_server", "latest rand in the past",
			"client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "remaining", remaining)
	}

	w.Header().Set("Expires", nextTime.Format(http.TimeFormat))
	w.Header().Set("Last-Modified", roundTime.Format(http.TimeFormat))
	_, _ = w.Write(data)
}

func (h *DrandHandler) ChainInfo(w http.ResponseWriter, r *http.Request) {
	chainHashHex, err := readChainHash(r)
	if err != nil {
		h.log.Warnw("", "http_server", "failed to read chain hash", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	info, err := h.getChainInfo(r.Context(), chainHashHex)
	if err != nil {
		h.log.Warnw("", "http_server", "failed to serve group", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path))
		http.Error(w, "group not found", http.StatusNotFound)
		return
	}

	var chainBuff bytes.Buffer
	err = info.ToJSON(&chainBuff, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warnw("", "http_server", "failed to marshal group", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "err", err)
		return
	}

	// Headers per recommendation for static assets at
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Cache-Control
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	w.Header().Set("Expires", time.Now().Add(7*24*time.Hour).Format(http.TimeFormat))
	http.ServeContent(w, r, "info.json", time.Unix(info.GenesisTime, 0), bytes.NewReader(chainBuff.Bytes()))
}

func (h *DrandHandler) Health(w http.ResponseWriter, r *http.Request) {
	chainHashHex, err := readChainHash(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	bh, err := h.getBeaconHandler(chainHashHex)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	bh.startOnce.Do(func() {
		h.start(bh)
	})

	bh.pendingLk.RLock()
	lastSeen := bh.latestRound
	bh.pendingLk.RUnlock()

	info, err := h.getChainInfo(r.Context(), chainHashHex)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	resp := make(map[string]uint64)
	resp["current"] = lastSeen
	resp["expected"] = 0
	var b []byte

	if err != nil {
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

func (h *DrandHandler) ChainHashes(w http.ResponseWriter, _ *http.Request) {
	chainHashes := make([]string, 0)
	for chainHash := range h.beacons {
		if chainHash != common.DefaultChainHash {
			chainHashes = append(chainHashes, chainHash)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "max-age=300")

	w.WriteHeader(http.StatusOK)
	b, _ := json.Marshal(chainHashes)
	_, _ = w.Write(b)
}

func readChainHash(r *http.Request) ([]byte, error) {
	var err error
	chainHashHex := make([]byte, 0)

	chainHash := chi.URLParam(r, chainHashParamKey)
	if chainHash != "" {
		chainHashHex, err = hex.DecodeString(chainHash)
		if err != nil {
			return nil, fmt.Errorf("unable to decode chain hash %s: %w", chainHash, err)
		}
	}

	return chainHashHex, nil
}

func readRound(r *http.Request) (uint64, error) {
	round := chi.URLParam(r, roundParamKey)
	return strconv.ParseUint(round, roundNumBase, roundNumSize)
}

func (h *DrandHandler) getBeaconHandler(chainHash []byte) (*BeaconHandler, error) {
	chainHashStr := fmt.Sprintf("%x", chainHash)
	if chainHashStr == "" {
		chainHashStr = common.DefaultChainHash
	}

	h.state.RLock()
	defer h.state.RUnlock()

	bh, exists := h.beacons[chainHashStr]

	if !exists {
		return nil, fmt.Errorf("there is no BeaconHandler for beaconHash [%s] in our beacons [%v]. "+
			"Is the chain hash correct?. Please check it", chainHashStr, h.beacons)
	}

	return bh, nil
}
