package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/key"
)

// NewHTTPClient creates a new client pointing to an HTTP endpoint
func NewHTTPClient(url string, groupHash []byte, client *http.Client) (Client, error) {
	c := &httpClient{
		root:   url,
		client: client,
	}
	// fetch the `Group` to validate connectivity.
	groupResp, err := client.Get(fmt.Sprintf("%s/group", url))
	if err != nil {
		return nil, err
	}

	grp := key.Group{}
	if err := json.NewDecoder(groupResp.Body).Decode(&grp); err != nil {
		return nil, err
	}
	c.group = &grp

	if !bytes.Equal(grp.PublicKey.Hash(), groupHash) {
		return nil, fmt.Errorf("%s does not advertise the expected drand group", url)
	}

	return c, nil
}

// httpClient implements Client through http requests to a Drand relay.
type httpClient struct {
	root   string
	client *http.Client
	group  *key.Group
}

// Implement textMarshaller
func (h *httpClient) MarshalText() ([]byte, error) {
	return json.Marshal(h)
}

type randData struct {
	Round             uint64 `json:"round,omitempty"`
	Signature         []byte `json:"signature,omitempty"`
	PreviousSignature []byte `json:"previous_signature,omitempty"`
	Randomness        []byte `json:"randomness,omitempty"`
}

// Get returns a the randomness at `round` or an error.
func (h *httpClient) Get(ctx context.Context, round uint64) (Result, error) {
	randResponse, err := h.client.Get(fmt.Sprintf("%s/public/%d", h.root, round))
	if err != nil {
		return Result{}, err
	}

	randResp := randData{}
	if err := json.NewDecoder(randResponse.Body).Decode(&randResp); err != nil {
		return Result{}, err
	}

	b := beacon.Beacon{
		PreviousSig: randResp.PreviousSignature,
		Round:       randResp.Round,
		Signature:   randResp.Signature,
	}
	if err := beacon.VerifyBeacon(h.group.PublicKey.Key(), &b); err != nil {
		return Result{}, err
	}

	// XXX: is this the right transition?
	return Result{Round: randResp.Round, Signature: randResp.Randomness}, nil
}

// RoundAt will return the most recent round of randomness that will be available
// at time for the current client.
func (h *httpClient) RoundAt(time time.Time) uint64 {
	return beacon.CurrentRound(time.Unix(), h.group.Period, h.group.GenesisTime)
}
