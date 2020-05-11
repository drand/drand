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
	drand "github.com/drand/drand/protobuf/drand"
)

// HTTPClient is an interface for the exercised methods of an `http.Client`,
// or equivalent alternative.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
	Get(url string) (resp *http.Response, err error)
}

// NewHTTPClient creates a new client pointing to an HTTP endpoint
func NewHTTPClient(url string, groupHash []byte, client HTTPClient) (Client, error) {
	c := &httpClient{
		root:   url,
		client: client,
	}
	group, err := c.FetchGroupInfo(groupHash)
	if err != nil {
		return nil, err
	}
	c.group = group

	return c, nil
}

// NewHTTPClientWithGroup constructs an http client when the group parameters are already known.
func NewHTTPClientWithGroup(url string, group *key.Group, client HTTPClient) (Client, error) {
	c := &httpClient{
		root:   url,
		group:  group,
		client: client,
	}
	return c, nil
}

// httpClient implements Client through http requests to a Drand relay.
type httpClient struct {
	root   string
	client HTTPClient
	group  *key.Group
}

// FetchGroupInfo attempts to initialize an httpClient when
// it does not know the full group paramters for a drand group.
func (h *httpClient) FetchGroupInfo(groupHash []byte) (*key.Group, error) {
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
		return nil, err
	}

	return &randResp, nil
}

// Watch returns new randomness as it becomes available.
func (h *httpClient) Watch(ctx context.Context) <-chan Result {
	ch := make(chan Result, 1)
	r := h.RoundAt(time.Now())
	val, err := h.Get(ctx, r)
	if err != nil {
		close(ch)
		return ch
	}
	ch <- val

	go func() {
		defer close(ch)

		// Initially, wait to synchronize to the round boundary.
		_, nextTime := beacon.NextRound(time.Now().Unix(), h.group.Period, h.group.GenesisTime)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(nextTime-time.Now().Unix()) * time.Second):
		}

		r, err := h.Get(ctx, h.RoundAt(time.Now()))
		if err == nil {
			ch <- r
		}

		// Then tick each period.
		t := time.NewTicker(h.group.Period)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				r, err := h.Get(ctx, h.RoundAt(time.Now()))
				if err == nil {
					ch <- r
				}
				// TODO: keep trying on errors?
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch
}

// RoundAt will return the most recent round of randomness that will be available
// at time for the current client.
func (h *httpClient) RoundAt(time time.Time) uint64 {
	return beacon.CurrentRound(time.Unix(), h.group.Period, h.group.GenesisTime)
}
