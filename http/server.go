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

	"github.com/ipfs/go-log/v2"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/core"
	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
)

// New creates an HTTP handler for the public Drand API
func New(ctx context.Context, client drand.PublicClient) (http.Handler, error) {
	pkt, err := client.Group(ctx, &drand.GroupRequest{})
	if err != nil {
		return nil, err
	}
	if pkt == nil {
		return nil, fmt.Errorf("Failed to retrieve valid GroupPacket")
	}
	parsedPkt, err := core.ProtoToGroup(pkt)
	if err != nil {
		return nil, err
	}

	handler := handler{ctx, client, parsedPkt, log.Logger("http")}

	mux := http.NewServeMux()
	//TODO: aggregated bulk round responses.
	mux.HandleFunc("/public/latest", handler.LatestRand)
	mux.HandleFunc("/public/", handler.PublicRand)
	mux.HandleFunc("/group", handler.Group)
	return mux, nil
}

type handler struct {
	ctx       context.Context
	client    drand.PublicClient
	groupInfo *key.Group
	log       log.StandardLogger
}

func (h *handler) getRand(round uint64) ([]byte, error) {
	req := drand.PublicRandRequest{Round: round}
	resp, err := h.client.PublicRand(h.ctx, &req)

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
		h.log.Warnf("%s %d - %s", r.RemoteAddr, http.StatusBadRequest, url.PathEscape(r.URL.Path))
		return
	}

	data, err := h.getRand(roundN)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warnf("%s %d - %s %v", r.RemoteAddr, http.StatusInternalServerError, url.PathEscape(r.URL.Path), err)
		return
	}

	roundExpectedTime := beacon.TimeOfRound(h.groupInfo.Period, h.groupInfo.GenesisTime, roundN)

	http.ServeContent(w, r, "rand.json", time.Unix(roundExpectedTime, 0), bytes.NewReader(data))
	h.log.Infof("%s %d - %s", r.RemoteAddr, http.StatusOK, url.PathEscape(r.URL.Path))
}

func (h *handler) LatestRand(w http.ResponseWriter, r *http.Request) {
	req := drand.PublicRandRequest{Round: 0}
	resp, err := h.client.PublicRand(h.ctx, &req)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warnf("%s %d - %s %v", r.RemoteAddr, http.StatusInternalServerError, url.PathEscape(r.URL.Path), err)
		return
	}

	data, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warnf("%s %d - %s %v", r.RemoteAddr, http.StatusInternalServerError, url.PathEscape(r.URL.Path), err)
		return
	}

	roundTime := time.Unix(beacon.TimeOfRound(h.groupInfo.Period, h.groupInfo.GenesisTime, resp.Round), 0)
	nextTime := roundTime.Add(h.groupInfo.Period)
	remaining := nextTime.Sub(time.Now())
	if remaining > 0 && remaining < h.groupInfo.Period {
		seconds := int(math.Ceil(remaining.Seconds()))
		w.Header().Set("Cache-Control", fmt.Sprintf("max-age:%d, public", seconds))
		h.log.Infof("%s %d - %s", r.RemoteAddr, http.StatusOK, url.PathEscape(r.URL.Path))
	} else {
		h.log.Warnf("%s %d - %s %v", r.RemoteAddr, http.StatusPartialContent, url.PathEscape(r.URL.Path), remaining)
	}

	w.Header().Set("Content-Type", "text/json")
	w.Header().Set("Expires", nextTime.Format(http.TimeFormat))
	w.Header().Set("Last-Modified", roundTime.Format(http.TimeFormat))
	w.Write(data)
}

func (h *handler) Group(w http.ResponseWriter, r *http.Request) {
	data, err := json.Marshal(h.groupInfo)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warnf("%s %d - %s %v", r.RemoteAddr, http.StatusInternalServerError, url.PathEscape(r.URL.Path), err)
		return
	}

	http.ServeContent(w, r, "group.json", time.Unix(h.groupInfo.GenesisTime, 0), bytes.NewReader(data))
	h.log.Infof("%s %d - %s", r.RemoteAddr, http.StatusOK, url.PathEscape(r.URL.Path))
}
