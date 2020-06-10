package http

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	nhttp "net/http"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	json "github.com/nikkolasg/hexjson"
)

// New creates a new client pointing to an HTTP endpoint
func New(url string, chainHash []byte, transport nhttp.RoundTripper) (client.Client, error) {
	if transport == nil {
		transport = nhttp.DefaultTransport
	}
	c := &httpClient{
		root:   url,
		client: instrumentClient(url, transport),
		l:      log.DefaultLogger,
	}
	chainInfo, err := c.FetchChainInfo(chainHash)
	if err != nil {
		return nil, err
	}
	c.chainInfo = chainInfo

	return c, nil
}

// NewWithInfo constructs an http client when the group parameters are already known.
func NewWithInfo(url string, info *chain.Info, transport nhttp.RoundTripper) (client.Client, error) {
	if transport == nil {
		transport = nhttp.DefaultTransport
	}

	c := &httpClient{
		root:      url,
		chainInfo: info,
		client:    instrumentClient(url, transport),
		l:         log.DefaultLogger,
	}
	return c, nil
}

// ForURLs provides a shortcut for creating a set of HTTP clients for a set of URLs.
func ForURLs(urls []string, chainHash []byte) []client.Client {
	clients := make([]client.Client, 0)
	var info *chain.Info
	skipped := []string{}
	for _, u := range urls {
		if info == nil {
			if c, err := New(u, chainHash, nil); err == nil {
				// Note: this wrapper assumes the current behavior that if `New` succeeds,
				// Info will have been fetched.
				info, _ = c.Info(context.Background())
				clients = append(clients, c)
			} else {
				skipped = append(skipped, u)
			}
		} else {
			if c, err := NewWithInfo(u, info, nil); err == nil {
				clients = append(clients, c)
			}
		}
	}
	if info != nil {
		for _, u := range skipped {
			if c, err := NewWithInfo(u, info, nil); err == nil {
				clients = append(clients, c)
			}
		}
	}
	return clients
}

// Instruments an HTTP client around a transport
func instrumentClient(url string, transport nhttp.RoundTripper) *nhttp.Client {
	client := &nhttp.Client{}
	urlLabel := prometheus.Labels{"url": url}

	trace := &promhttp.InstrumentTrace{
		DNSStart: func(t float64) {
			metrics.ClientDNSLatencyVec.MustCurryWith(urlLabel).WithLabelValues("dns_start").Observe(t)
		},
		DNSDone: func(t float64) {
			metrics.ClientDNSLatencyVec.MustCurryWith(urlLabel).WithLabelValues("dns_done").Observe(t)
		},
		TLSHandshakeStart: func(t float64) {
			metrics.ClientTLSLatencyVec.MustCurryWith(urlLabel).WithLabelValues("tls_handshake_start").Observe(t)
		},
		TLSHandshakeDone: func(t float64) {
			metrics.ClientTLSLatencyVec.MustCurryWith(urlLabel).WithLabelValues("tls_handshake_done").Observe(t)
		},
	}

	transport = promhttp.InstrumentRoundTripperInFlight(metrics.ClientInFlight.With(urlLabel),
		promhttp.InstrumentRoundTripperCounter(metrics.ClientRequests.MustCurryWith(urlLabel),
			promhttp.InstrumentRoundTripperTrace(trace,
				promhttp.InstrumentRoundTripperDuration(metrics.ClientLatencyVec.MustCurryWith(urlLabel),
					transport))))

	client.Transport = transport

	return client
}

// httpClient implements Client through http requests to a Drand relay.
type httpClient struct {
	root      string
	client    *nhttp.Client
	chainInfo *chain.Info
	l         log.Logger
}

// SetLog configures the client log output
func (h *httpClient) SetLog(l log.Logger) {
	h.l = l
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
		h.l.Warn("http_client", "instantiated without trustroot", "chainHash", hex.EncodeToString(chainInfo.Hash()))
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

// Get returns a the randomness at `round` or an error.
func (h *httpClient) Get(ctx context.Context, round uint64) (client.Result, error) {
	var url string
	if round == 0 {
		url = fmt.Sprintf("%s/public/latest", h.root)
	} else {
		url = fmt.Sprintf("%s/public/%d", h.root, round)
	}

	randResponse, err := h.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer randResponse.Body.Close()

	randResp := client.RandomData{}
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
func (h *httpClient) Watch(ctx context.Context) <-chan client.Result {
	return client.PollingWatcher(ctx, h, h.chainInfo, h.l)
}

// Info returns information about the chain.
func (h *httpClient) Info(ctx context.Context) (*chain.Info, error) {
	return h.chainInfo, nil
}

// RoundAt will return the most recent round of randomness that will be available
// at time for the current client.
func (h *httpClient) RoundAt(time time.Time) uint64 {
	return chain.CurrentRound(time.Unix(), h.chainInfo.Period, h.chainInfo.GenesisTime)
}
