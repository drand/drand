package http

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	nhttp "net/http"
	"strings"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	json "github.com/nikkolasg/hexjson"
)

var errClientClosed = fmt.Errorf("client closed")

// New creates a new client pointing to an HTTP endpoint
func New(url string, chainHash []byte, transport nhttp.RoundTripper) (client.Client, error) {
	if transport == nil {
		transport = nhttp.DefaultTransport
	}
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	c := &httpClient{
		root:   url,
		client: instrumentClient(url, transport),
		l:      log.DefaultLogger(),
		done:   make(chan struct{}),
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
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}

	c := &httpClient{
		root:      url,
		chainInfo: info,
		client:    instrumentClient(url, transport),
		l:         log.DefaultLogger(),
		done:      make(chan struct{}),
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
	hc := nhttp.Client{}
	hc.Timeout = nhttp.DefaultClient.Timeout
	hc.Jar = nhttp.DefaultClient.Jar
	hc.CheckRedirect = nhttp.DefaultClient.CheckRedirect
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

	hc.Transport = transport

	return &hc
}

// httpClient implements Client through http requests to a Drand relay.
type httpClient struct {
	root      string
	client    *nhttp.Client
	chainInfo *chain.Info
	l         log.Logger
	done      chan struct{}
}

// SetLog configures the client log output
func (h *httpClient) SetLog(l log.Logger) {
	h.l = l
}

// String returns the name of this client.
func (h *httpClient) String() string {
	return fmt.Sprintf("HTTP(%q)", h.root)
}

type httpInfoResponse struct {
	chainInfo *chain.Info
	err       error
}

// FetchGroupInfo attempts to initialize an httpClient when
// it does not know the full group parameters for a drand group. The chain hash
// is the hash of the chain info.
func (h *httpClient) FetchChainInfo(chainHash []byte) (*chain.Info, error) {
	if h.chainInfo != nil {
		return h.chainInfo, nil
	}

	resC := make(chan httpInfoResponse, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		req, err := nhttp.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%sinfo", h.root), nil)
		if err != nil {
			resC <- httpInfoResponse{nil, fmt.Errorf("creating request: %w", err)}
			return
		}

		infoBody, err := h.client.Do(req)
		if err != nil {
			resC <- httpInfoResponse{nil, fmt.Errorf("doing request: %w", err)}
			return
		}
		defer infoBody.Body.Close()

		chainInfo, err := chain.InfoFromJSON(infoBody.Body)
		if err != nil {
			resC <- httpInfoResponse{nil, fmt.Errorf("decoding response: %w", err)}
			return
		}

		if chainInfo.PublicKey == nil {
			resC <- httpInfoResponse{nil, fmt.Errorf("group does not have a valid key for validation")}
			return
		}

		if chainHash == nil {
			h.l.Warn("http_client", "instantiated without trustroot", "chainHash", hex.EncodeToString(chainInfo.Hash()))
		}
		if chainHash != nil && !bytes.Equal(chainInfo.Hash(), chainHash) {
			err := fmt.Errorf("%s does not advertise the expected drand group (%x vs %x)", h.root, chainInfo.Hash(), chainHash)
			resC <- httpInfoResponse{nil, err}
			return
		}
		resC <- httpInfoResponse{chainInfo, nil}
	}()

	select {
	case res := <-resC:
		if res.err != nil {
			return nil, res.err
		}
		return res.chainInfo, nil
	case <-h.done:
		return nil, errClientClosed
	}
}

// Implement textMarshaller
func (h *httpClient) MarshalText() ([]byte, error) {
	return json.Marshal(h)
}

type httpGetResponse struct {
	result client.Result
	err    error
}

// Get returns a the randomness at `round` or an error.
func (h *httpClient) Get(ctx context.Context, round uint64) (client.Result, error) {
	var url string
	if round == 0 {
		url = fmt.Sprintf("%spublic/latest", h.root)
	} else {
		url = fmt.Sprintf("%spublic/%d", h.root, round)
	}

	resC := make(chan httpGetResponse, 1)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		req, err := nhttp.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			resC <- httpGetResponse{nil, fmt.Errorf("creating request: %w", err)}
			return
		}

		randResponse, err := h.client.Do(req)
		if err != nil {
			resC <- httpGetResponse{nil, fmt.Errorf("doing request: %w", err)}
			return
		}
		defer randResponse.Body.Close()

		randResp := client.RandomData{}
		if err := json.NewDecoder(randResponse.Body).Decode(&randResp); err != nil {
			resC <- httpGetResponse{nil, fmt.Errorf("decoding response: %w", err)}
			return
		}
		if len(randResp.Sig) == 0 || len(randResp.PreviousSignature) == 0 {
			resC <- httpGetResponse{nil, fmt.Errorf("insufficient response")}
			return
		}

		resC <- httpGetResponse{&randResp, nil}
	}()

	select {
	case res := <-resC:
		if res.err != nil {
			return nil, res.err
		}
		return res.result, nil
	case <-h.done:
		return nil, errClientClosed
	}
}

// Watch returns new randomness as it becomes available.
func (h *httpClient) Watch(ctx context.Context) <-chan client.Result {
	out := make(chan client.Result)
	go func() {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		defer close(out)

		in := client.PollingWatcher(ctx, h, h.chainInfo, h.l)
		for {
			select {
			case res, ok := <-in:
				if !ok {
					return
				}
				out <- res
			case <-h.done:
				return
			}
		}
	}()
	return out
}

// Info returns information about the chain.
func (h *httpClient) Info(ctx context.Context) (*chain.Info, error) {
	return h.chainInfo, nil
}

// RoundAt will return the most recent round of randomness that will be available
// at time for the current client.
func (h *httpClient) RoundAt(t time.Time) uint64 {
	return chain.CurrentRound(t.Unix(), h.chainInfo.Period, h.chainInfo.GenesisTime)
}

func (h *httpClient) Close() error {
	close(h.done)
	h.client.CloseIdleConnections()
	return nil
}
