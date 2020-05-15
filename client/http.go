package client

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	drand "github.com/drand/drand/protobuf/drand"

	json "github.com/nikkolasg/hexjson"
)

// HTTPGetter is an interface for the exercised methods of an `http.Client`,
// or equivalent alternative.
type HTTPGetter interface {
	Do(req *http.Request) (*http.Response, error)
	Get(url string) (resp *http.Response, err error)
}

// NewHTTPClient creates a new client pointing to an HTTP endpoint
func NewHTTPClient(url string, groupHash []byte, client HTTPGetter) (Client, error) {
	if client == nil {
		client = &http.Client{}
	}
	c := &httpClient{
		root:   url,
		client: client,
		l:      log.DefaultLogger,
	}
	group, err := c.FetchGroupInfo(groupHash)
	if err != nil {
		return nil, err
	}
	c.group = group

	return c, nil
}

// NewHTTPClientWithGroup constructs an http client when the group parameters are already known.
func NewHTTPClientWithGroup(url string, group *key.Group, client HTTPGetter) (Client, error) {
	if client == nil {
		client = &http.Client{}
	}
	c := &httpClient{
		root:   url,
		group:  group,
		client: client,
		l:      log.DefaultLogger,
	}
	return c, nil
}

// httpClient implements Client through http requests to a Drand relay.
type httpClient struct {
	root   string
	client HTTPGetter
	group  *key.Group
	l      log.Logger
}

// FetchGroupInfo attempts to initialize an httpClient when
// it does not know the full group paramters for a drand group.
func (h *httpClient) FetchGroupInfo(groupHash []byte) (*key.Group, error) {
	if h.group != nil {
		return h.group, nil
	}

	// fetch the `Group` to validate connectivity.
	groupResp, err := h.client.Get(fmt.Sprintf("%s/group", h.root))
	if err != nil {
		return nil, err
	}

	protoGrp := drand.GroupPacket{}
	if err := json.NewDecoder(groupResp.Body).Decode(&protoGrp); err != nil {
		return nil, err
	}
	grp, err := key.GroupFromProto(&protoGrp)
	if err != nil {
		return nil, err
	}

	if grp.PublicKey == nil {
		return nil, fmt.Errorf("Group does not have a valid key for validation")
	}

	if groupHash == nil {
		h.l.Warn("http_client", "instantiated without trustroot", "groupHash", hex.EncodeToString(grp.Hash()))
	}
	if groupHash != nil && !bytes.Equal(grp.Hash(), groupHash) {
		return nil, fmt.Errorf("%s does not advertise the expected drand group", h.root)
	}
	return grp, nil
}

// Implement textMarshaller
func (h *httpClient) MarshalText() ([]byte, error) {
	return json.Marshal(h)
}

// RandomData holds the full random response from the server, including data needed
// for validation.
type RandomData struct {
	Rnd               uint64 `json:"round,omitempty"`
	Random            []byte `json:"randomness,omitempty"`
	Signature         []byte `json:"signature,omitempty"`
	PreviousSignature []byte `json:"previous_signature,omitempty"`
}

// Round provides access to the round associatted with this random data.
func (r *RandomData) Round() uint64 {
	return r.Rnd
}

// Randomness exports the randomness
func (r *RandomData) Randomness() []byte {
	return r.Random
}

// Get returns a the randomness at `round` or an error.
func (h *httpClient) Get(ctx context.Context, round uint64) (Result, error) {
	randResponse, err := h.client.Get(fmt.Sprintf("%s/public/%d", h.root, round))
	if err != nil {
		return nil, err
	}

	randResp := RandomData{}
	if err := json.NewDecoder(randResponse.Body).Decode(&randResp); err != nil {
		return nil, err
	}
	if len(randResp.Signature) == 0 || len(randResp.PreviousSignature) == 0 {
		return nil, fmt.Errorf("insufficent response")
	}

	b := beacon.Beacon{
		PreviousSig: randResp.PreviousSignature,
		Round:       randResp.Rnd,
		Signature:   randResp.Signature,
	}
	if err := beacon.VerifyBeacon(h.group.PublicKey.Key(), &b); err != nil {
		h.l.Warn("http_client", "failed to verify value", "err", err)
		return nil, err
	}

	return &randResp, nil
}

// Watch returns new randomness as it becomes available.
func (h *httpClient) Watch(ctx context.Context) <-chan Result {
	return pollingWatcher(ctx, h, h.group, h.l)
}

// RoundAt will return the most recent round of randomness that will be available
// at time for the current client.
func (h *httpClient) RoundAt(time time.Time) uint64 {
	return beacon.CurrentRound(time.Unix(), h.group.Period, h.group.GenesisTime)
}
