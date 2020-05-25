package client

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"

	json "github.com/nikkolasg/hexjson"
)

// HTTPGetter is an interface for the exercised methods of an `http.Client`,
// or equivalent alternative.
type HTTPGetter interface {
	Do(req *http.Request) (*http.Response, error)
	Get(url string) (resp *http.Response, err error)
}

// NewHTTPClient creates a new client pointing to an HTTP endpoint
func NewHTTPClient(url string, chainHash []byte, client HTTPGetter) (Client, error) {
	if client == nil {
		client = &http.Client{}
	}
	c := &httpClient{
		root:   url,
		client: client,
		l:      log.DefaultLogger,
	}
	chainInfo, err := c.FetchChainInfo(chainHash)
	if err != nil {
		return nil, err
	}
	c.chainInfo = chainInfo

	return c, nil
}

// NewHTTPClientWithGroup constructs an http client when the group parameters are already known.
func NewHTTPClientWithInfo(url string, info *chain.Info, client HTTPGetter) (Client, error) {
	if client == nil {
		client = &http.Client{}
	}
	c := &httpClient{
		root:      url,
		chainInfo: info,
		client:    client,
		l:         log.DefaultLogger,
	}
	return c, nil
}

// httpClient implements Client through http requests to a Drand relay.
type httpClient struct {
	root      string
	client    HTTPGetter
	chainInfo *chain.Info
	l         log.Logger
}

// FetchGroupInfo attempts to initialize an httpClient when
// it does not know the full group paramters for a drand group. The chain hash
// is the hash of the chain info.
func (h *httpClient) FetchChainInfo(chainHash []byte) (*chain.Info, error) {
	if h.chainInfo != nil {
		return h.chainInfo, nil
	}

	infoBody, err := h.client.Get(fmt.Sprintf("%s/info", h.root))
	if err != nil {
		return nil, err
	}
	defer infoBody.Body.Close()

	chainInfo, err := chain.InfoFromJSON(infoBody.Body)
	if err != nil {
		return nil, err
	}

	if chainInfo.PublicKey == nil {
		return nil, fmt.Errorf("Group does not have a valid key for validation")
	}

	if chainHash == nil {
		h.l.Warn("http_client", "instantiated without trustroot", "groupHash", hex.EncodeToString(chainInfo.Hash()))
	}
	if chainHash != nil && !bytes.Equal(chainInfo.Hash(), chainHash) {
		return nil, fmt.Errorf("%s does not advertise the expected drand group (%x vs %x)", h.root, chainInfo.Hash(), chainHash)
	}
	return chainInfo, nil
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
	Sig               []byte `json:"signature,omitempty"`
	PreviousSignature []byte `json:"previous_signature,omitempty"`
}

// Round provides access to the round associatted with this random data.
func (r *RandomData) Round() uint64 {
	return r.Rnd
}

func (r *RandomData) Signature() []byte {
	return r.Sig
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
	defer randResponse.Body.Close()

	randResp := RandomData{}
	if err := json.NewDecoder(randResponse.Body).Decode(&randResp); err != nil {
		return nil, err
	}
	if len(randResp.Sig) == 0 || len(randResp.PreviousSignature) == 0 {
		return nil, fmt.Errorf("insufficent response")
	}

	b := chain.Beacon{
		PreviousSig: randResp.PreviousSignature,
		Round:       randResp.Rnd,
		Signature:   randResp.Sig,
	}
	if err := chain.VerifyBeacon(h.chainInfo.PublicKey, &b); err != nil {
		h.l.Warn("http_client", "failed to verify value", "err", err)
		return nil, err
	}
	randResp.Random = chain.RandomnessFromSignature(randResp.Sig)

	return &randResp, nil
}

// Watch returns new randomness as it becomes available.
func (h *httpClient) Watch(ctx context.Context) <-chan Result {
	return pollingWatcher(ctx, h, h.chainInfo, h.l)
}

// RoundAt will return the most recent round of randomness that will be available
// at time for the current client.
func (h *httpClient) RoundAt(time time.Time) uint64 {
	return chain.CurrentRound(time.Unix(), h.chainInfo.Period, h.chainInfo.GenesisTime)
}
