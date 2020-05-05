package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

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

	handler := handler{ctx, client, pkt}

	mux := http.NewServeMux()
	//TODO: aggregated bulk round responses.
	mux.HandleFunc("/PublicRand", handler.PublicRand)
	mux.HandleFunc("/Group", handler.Group)
	return mux, nil
}

type handler struct {
	ctx       context.Context
	client    drand.PublicClient
	groupInfo *drand.GroupPacket
}

func (h *handler) PublicRand(w http.ResponseWriter, r *http.Request) {
	// Get the round.
	round, ok := r.URL.Query()["round"]
	if !ok || len(round) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	roundN, err := strconv.ParseUint(round[0], 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	req := drand.PublicRandRequest{Round: roundN}
	resp, err := h.client.PublicRand(h.ctx, &req)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	roundExpectedTime := time.Unix(int64(h.groupInfo.GenesisTime), 0).Add(time.Duration(h.groupInfo.Period) * time.Second)

	http.ServeContent(w, r, "rand.json", roundExpectedTime, bytes.NewReader(data))
}

func (h *handler) Group(w http.ResponseWriter, r *http.Request) {
	data, err := json.Marshal(h.groupInfo)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	http.ServeContent(w, r, "group.json", time.Unix(int64(h.groupInfo.GenesisTime), 0), bytes.NewReader(data))
}
