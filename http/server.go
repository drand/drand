package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
)

var (
	// Timeout for how long to wait for the drand.PublicClient before timing out
	reqTimeout = 5 * time.Second
)

// New creates an HTTP handler for the public Drand API
func New(ctx context.Context, client drand.PublicClient, logger log.Logger) (http.Handler, error) {
	pkt, err := client.Group(ctx, &drand.GroupRequest{})
	if err != nil {
		return nil, err
	}
	if pkt == nil {
		return nil, fmt.Errorf("Failed to retrieve valid GroupPacket")
	}
	parsedPkt, err := key.GroupFromProto(pkt)
	if err != nil {
		return nil, err
	}

	if logger == nil {
		logger = log.DefaultLogger
	}
	handler := handler{reqTimeout, client, parsedPkt, logger}

	mux := http.NewServeMux()
	//TODO: aggregated bulk round responses.
	mux.HandleFunc("/public/latest", handler.LatestRand)
	mux.HandleFunc("/public/", handler.PublicRand)
	mux.HandleFunc("/group", handler.Group)
	return mux, nil
}

type handler struct {
	timeout   time.Duration
	client    drand.PublicClient
	groupInfo *key.Group
	log       log.Logger
}

func (h *handler) getRand(round uint64) ([]byte, error) {
	req := drand.PublicRandRequest{Round: round}
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()
	resp, err := h.client.PublicRand(ctx, &req)

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

	data, err := h.getRand(roundN)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warn("http_server", "failed to get randomness", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "err", err)
		return
	}

	roundExpectedTime := time.Unix(h.groupInfo.GenesisTime, 0).Add(h.groupInfo.Period * time.Duration(roundN))

	http.ServeContent(w, r, "rand.json", roundExpectedTime, bytes.NewReader(data))
}

func (h *handler) LatestRand(w http.ResponseWriter, r *http.Request) {
	req := drand.PublicRandRequest{Round: 0}
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()

	resp, err := h.client.PublicRand(ctx, &req)

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

	roundTime := time.Unix(h.groupInfo.GenesisTime, 0).Add(h.groupInfo.Period * time.Duration(resp.Round))

	nextTime := roundTime.Add(h.groupInfo.Period)

	remaining := nextTime.Sub(time.Now())
	if remaining > 0 && remaining < h.groupInfo.Period {
		seconds := int(math.Ceil(remaining.Seconds()))
		w.Header().Set("Cache-Control", fmt.Sprintf("max-age:%d, public", seconds))
	} else {
		h.log.Warn("http_server", "latest rand in the past", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "remaining", remaining)
	}

	w.Header().Set("Content-Type", "text/json")
	w.Header().Set("Expires", nextTime.Format(http.TimeFormat))
	w.Header().Set("Last-Modified", roundTime.Format(http.TimeFormat))
	w.Write(data)
}

func (h *handler) Group(w http.ResponseWriter, r *http.Request) {
	data, err := json.Marshal(h.groupInfo.ToProto())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warn("http_server", "failed to marshal group", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "err", err)
		return
	}

	http.ServeContent(w, r, "group.json", time.Unix(h.groupInfo.GenesisTime, 0), bytes.NewReader(data))
}
