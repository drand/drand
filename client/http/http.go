package http

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	nhttp "net/http"
	"os"
	"path"
	"strings"
	"time"

	json "github.com/nikkolasg/hexjson"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	client2 "github.com/drand/drand/client"
	"github.com/drand/drand/common"
	chain2 "github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/client"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/internal/chain"
	"github.com/drand/drand/internal/metrics"
)

var errClientClosed = fmt.Errorf("client closed")

const defaultClientExec = "unknown"
const defaultHTTTPTimeout = 60 * time.Second

const httpWaitMaxCounter = 20
const httpWaitInterval = 2 * time.Second
const maxTimeoutHTTPRequest = 5 * time.Second

// New creates a new client pointing to an HTTP endpoint
func New(ctx context.Context, l log.Logger, url string, chainHash []byte, transport nhttp.RoundTripper) (client.Client, error) {
	if transport == nil {
		transport = nhttp.DefaultTransport
	}
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	pn, err := os.Executable()
	if err != nil {
		pn = defaultClientExec
	}
	agent := fmt.Sprintf("drand-client-%s/1.0", path.Base(pn))
	c := &httpClient{
		root:   url,
		client: instrumentClient(url, transport),
		l:      l,
		Agent:  agent,
		done:   make(chan struct{}),
	}

	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	chainInfo, err := c.FetchChainInfo(ctx, chainHash)
	if err != nil {
		return nil, err
	}
	c.chainInfo = chainInfo

	return c, nil
}

// NewWithInfo constructs an http client when the group parameters are already known.
func NewWithInfo(l log.Logger, url string, info *chain2.Info, transport nhttp.RoundTripper) (client.Client, error) {
	if transport == nil {
		transport = nhttp.DefaultTransport
	}
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}

	pn, err := os.Executable()
	if err != nil {
		pn = defaultClientExec
	}
	agent := fmt.Sprintf("drand-client-%s/1.0", path.Base(pn))
	c := &httpClient{
		root:      url,
		chainInfo: info,
		client:    instrumentClient(url, transport),
		l:         l,
		Agent:     agent,
		done:      make(chan struct{}),
	}
	return c, nil
}

// ForURLs provides a shortcut for creating a set of HTTP clients for a set of URLs.
func ForURLs(ctx context.Context, l log.Logger, urls []string, chainHash []byte) []client.Client {
	clients := make([]client.Client, 0)
	var info *chain2.Info
	var skipped []string
	for _, u := range urls {
		if info == nil {
			if c, err := New(ctx, l, u, chainHash, nil); err == nil {
				// Note: this wrapper assumes the current behavior that if `New` succeeds,
				// Info will have been fetched.
				info, _ = c.Info(ctx)
				clients = append(clients, c)
			} else {
				skipped = append(skipped, u)
			}
		} else {
			if c, err := NewWithInfo(l, u, info, nil); err == nil {
				clients = append(clients, c)
			}
		}
	}
	if info != nil {
		for _, u := range skipped {
			if c, err := NewWithInfo(l, u, info, nil); err == nil {
				clients = append(clients, c)
			}
		}
	}
	return clients
}

func Ping(ctx context.Context, root string) error {
	url := fmt.Sprintf("%s/health", root)

	ctx, cancel := context.WithTimeout(ctx, maxTimeoutHTTPRequest)
	defer cancel()

	req, err := nhttp.NewRequestWithContext(ctx, nhttp.MethodGet, url, nhttp.NoBody)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	response, err := nhttp.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	defer response.Body.Close()

	return nil
}

// Instruments an HTTP client around a transport
func instrumentClient(url string, transport nhttp.RoundTripper) *nhttp.Client {
	hc := nhttp.Client{}
	hc.Timeout = defaultHTTTPTimeout
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

func IsServerReady(ctx context.Context, addr string) (er error) {
	counter := 0
	for {
		// Ping is wrapping its context with a Timeout on maxTimeoutHTTPRequest anyway.
		err := Ping(ctx, "http://"+addr)
		if err == nil {
			return nil
		}

		counter++
		if counter == httpWaitMaxCounter {
			return fmt.Errorf("timeout waiting http server to be ready")
		}

		time.Sleep(httpWaitInterval)
	}
}

// httpClient implements Client through http requests to a Drand relay.
type httpClient struct {
	root      string
	client    *nhttp.Client
	Agent     string
	chainInfo *chain2.Info
	l         log.Logger
	done      chan struct{}
}

// SetLog configures the client log output
func (h *httpClient) SetLog(l log.Logger) {
	h.l = l
}

// SetUserAgent sets the user agent used by the client
func (h *httpClient) SetUserAgent(ua string) {
	h.Agent = ua
}

// String returns the name of this client.
func (h *httpClient) String() string {
	return fmt.Sprintf("HTTP(%q)", h.root)
}

// MarshalText implements encoding.TextMarshaller interface
func (h *httpClient) MarshalText() ([]byte, error) {
	return json.Marshal(h.String())
}

type httpInfoResponse struct {
	chainInfo *chain2.Info
	err       error
}

// FetchChainInfo attempts to initialize an httpClient when
// it does not know the full group parameters for a drand group. The chain hash
// is the hash of the chain info.
func (h *httpClient) FetchChainInfo(ctx context.Context, chainHash []byte) (*chain2.Info, error) {
	if h.chainInfo != nil {
		return h.chainInfo, nil
	}

	resC := make(chan httpInfoResponse, 1)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		var url string
		if len(chainHash) > 0 {
			url = fmt.Sprintf("%s%x/info", h.root, chainHash)
		} else {
			url = fmt.Sprintf("%sinfo", h.root)
		}

		req, err := nhttp.NewRequestWithContext(ctx, nhttp.MethodGet, url, nhttp.NoBody)
		if err != nil {
			resC <- httpInfoResponse{nil, fmt.Errorf("creating request: %w", err)}
			return
		}
		req.Header.Set("User-Agent", h.Agent)

		infoBody, err := h.client.Do(req)
		if err != nil {
			resC <- httpInfoResponse{nil, fmt.Errorf("doing request: %w", err)}
			return
		}
		defer infoBody.Body.Close()

		chainInfo, err := chain2.InfoFromJSON(infoBody.Body)
		if err != nil {
			resC <- httpInfoResponse{nil, fmt.Errorf("decoding response: %w", err)}
			return
		}

		if chainInfo.PublicKey == nil {
			resC <- httpInfoResponse{nil, fmt.Errorf("group does not have a valid key for validation")}
			return
		}

		if len(chainHash) == 0 {
			h.l.Warnw("", "http_client", "instantiated without trustroot", "chainHash", hex.EncodeToString(chainInfo.Hash()))
			if !common.IsDefaultBeaconID(chainInfo.ID) {
				err := fmt.Errorf("%s does not advertise the default drand for the default chainHash (got %x)", h.root, chainInfo.Hash())
				resC <- httpInfoResponse{nil, err}
				return
			}
		} else if !bytes.Equal(chainInfo.Hash(), chainHash) {
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

type httpGetResponse struct {
	result client.Result
	err    error
}

// Get returns the randomness at `round` or an error.
func (h *httpClient) Get(ctx context.Context, round uint64) (client.Result, error) {
	var url string
	if round == 0 {
		url = fmt.Sprintf("%s%x/public/latest", h.root, h.chainInfo.Hash())
	} else {
		url = fmt.Sprintf("%s%x/public/%d", h.root, h.chainInfo.Hash(), round)
	}

	resC := make(chan httpGetResponse, 1)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		req, err := nhttp.NewRequestWithContext(ctx, nhttp.MethodGet, url, nhttp.NoBody)
		if err != nil {
			resC <- httpGetResponse{nil, fmt.Errorf("creating request: %w", err)}
			return
		}
		req.Header.Set("User-Agent", h.Agent)

		randResponse, err := h.client.Do(req)
		if err != nil {
			resC <- httpGetResponse{nil, fmt.Errorf("doing request: %w", err)}
			return
		}
		defer randResponse.Body.Close()

		randResp := client2.RandomData{}
		if err := json.NewDecoder(randResponse.Body).Decode(&randResp); err != nil {
			resC <- httpGetResponse{nil, fmt.Errorf("decoding response: %w", err)}
			return
		}

		if len(randResp.Sig) == 0 {
			resC <- httpGetResponse{nil, fmt.Errorf("insufficient response - signature is not present")}
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

		in := client2.PollingWatcher(ctx, h, h.chainInfo, h.l)
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
func (h *httpClient) Info(_ context.Context) (*chain2.Info, error) {
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
