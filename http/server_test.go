package http

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	json "github.com/nikkolasg/hexjson"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/client"
	"github.com/drand/drand/client/grpc"
	nhttp "github.com/drand/drand/client/http"
	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test/mock"
)

func withClient(t *testing.T) (c client.Client, emit func(bool)) {
	t.Helper()
	sch := scheme.GetSchemeFromEnv()

	l, s := mock.NewMockGRPCPublicServer(":0", true, sch)
	lAddr := l.Addr()
	go l.Start()

	c, _ = grpc.New(lAddr, "", true, []byte(""))

	return c, s.(mock.MockService).EmitRand
}

func getWithCtx(ctx context.Context, url string, t *testing.T) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

//nolint:funlen
func TestHTTPRelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c, _ := withClient(t)

	handler, err := New(ctx, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	info, err := c.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}

	handler.RegisterNewBeaconHandler(c, info.HashString())

	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	server := http.Server{Handler: handler.GetHTTPHandler()}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Shutdown(ctx) }()

	err = nhttp.IsServerReady(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	getChains := fmt.Sprintf("http://%s/chains", listener.Addr().String())
	resp := getWithCtx(ctx, getChains, t)
	if resp.StatusCode != http.StatusOK {
		t.Error("expected http status code 200")
	}
	var chains []string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&chains))
	require.NoError(t, resp.Body.Close())

	if len(chains) != 1 {
		t.Error("expected chain hash qty not valid")
	}
	if chains[0] != info.HashString() {
		t.Error("expected chain hash not valid")
	}

	getChain := fmt.Sprintf("http://%s/%s/info", listener.Addr().String(), info.HashString())
	resp = getWithCtx(ctx, getChain, t)
	cip := new(drand.ChainInfoPacket)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(cip))
	require.NotNil(t, cip.Hash)
	require.NotNil(t, cip.PublicKey)
	require.NoError(t, resp.Body.Close())

	// Test exported interfaces.
	u := fmt.Sprintf("http://%s/%s/public/2", listener.Addr().String(), info.HashString())
	resp = getWithCtx(ctx, u, t)
	body := make(map[string]interface{})
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.NoError(t, resp.Body.Close())

	if _, ok := body["signature"]; !ok {
		t.Fatal("expected signature in random response.")
	}

	u = fmt.Sprintf("http://%s/%s/public/latest", listener.Addr().String(), info.HashString())
	resp, err = http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	body = make(map[string]interface{})

	if err = json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	require.NoError(t, resp.Body.Close())

	if _, ok := body["round"]; !ok {
		t.Fatal("expected signature in latest response.")
	}
}

func validateEndpoint(endpoint string, round float64) error {
	resp, _ := http.Get(fmt.Sprintf("http://%s", endpoint))
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %v", resp.StatusCode)
	}

	body := make(map[string]interface{})
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return err
	}
	if err := resp.Body.Close(); err != nil {
		return err
	}
	if body["round"].(float64) != round {
		return fmt.Errorf("wrong response round number: %v", body)
	}
	return nil
}

func TestHTTPWaiting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c, push := withClient(t)

	handler, err := New(ctx, "", nil)
	require.NoError(t, err)

	info, err := c.Info(ctx)
	require.NoError(t, err)

	handler.RegisterNewBeaconHandler(c, info.HashString())

	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	server := http.Server{Handler: handler.GetHTTPHandler()}
	go func() {
		err := server.Serve(listener)
		if err != nil {
			t.Logf("error while server.Server %v\n", err)
		}
	}()
	defer func() { _ = server.Shutdown(ctx) }()

	err = nhttp.IsServerReady(listener.Addr().String())
	require.NoError(t, err)

	// The first request will trigger background watch. 1 get (1969)
	u := fmt.Sprintf("http://%s/%s/public/1", listener.Addr().String(), info.HashString())
	next := getWithCtx(ctx, u, t)
	defer func() { _ = next.Body.Close() }()

	// 1 watch get will occur (1970 - the bad one)
	push(false)

	done := make(chan time.Time)
	before := time.Now()

	go func() {
		endpoint := listener.Addr().String() + "/" + info.HashString() + "/public/1971"
		if err = validateEndpoint(endpoint, 1971.0); err != nil {
			t.Logf("got validation error: %v\n", err)
			done <- time.Unix(0, 0)
			return
		}
		done <- time.Now()
	}()

	time.Sleep(50 * time.Millisecond)

	select {
	case <-done:
		t.Fatalf("shouldn't be done. err: %v\n", err)
	default:
	}

	// Push the correct round
	push(false)

	time.Sleep(10 * time.Millisecond)

	var after time.Time
	select {
	case x := <-done:
		require.NoError(t, err)
		after = x
	case <-time.After(10 * time.Millisecond):
		t.Fatal("should return after a round")
	}
	// mock grpc server spits out new round every second on streaming interface.
	if after.Sub(before) > time.Second || after.Sub(before) < 10*time.Millisecond {
		t.Fatalf("unexpected timing to receive response: %s", before)
	}
}

func TestHTTPWatchFuture(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, _ := withClient(t)

	handler, err := New(ctx, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	info, err := c.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}

	handler.RegisterNewBeaconHandler(c, info.HashString())

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	server := http.Server{Handler: handler.GetHTTPHandler()}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Shutdown(ctx) }()

	nhttp.IsServerReady(listener.Addr().String())

	// watching sets latest round, future rounds should become inaccessible.
	u := fmt.Sprintf("http://%s/%s/public/2000", listener.Addr().String(), info.HashString())
	resp := getWithCtx(ctx, u, t)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatal("response should fail on requests in the future")
	}
}

func TestHTTPHealth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, push := withClient(t)

	handler, err := New(ctx, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	info, err := c.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}

	handler.RegisterNewBeaconHandler(c, info.HashString())

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	server := http.Server{Handler: handler.GetHTTPHandler()}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Shutdown(ctx) }()

	err = nhttp.IsServerReady(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	resp := getWithCtx(ctx, fmt.Sprintf("http://%s/%s/health", listener.Addr().String(), info.HashString()), t)
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("newly started server not expected to be synced.")
	}
	resp.Body.Close()

	resp = getWithCtx(ctx, fmt.Sprintf("http://%s/%s/public/0", listener.Addr().String(), info.HashString()), t)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("startup of the server on 1st request should happen")
	}
	push(false)
	// give some time for http server to get it
	time.Sleep(30 * time.Millisecond)
	resp.Body.Close()

	resp = getWithCtx(ctx, fmt.Sprintf("http://%s/%s/health", listener.Addr().String(), info.HashString()), t)
	if resp.StatusCode != http.StatusOK {
		var buf [100]byte
		_, _ = resp.Body.Read(buf[:])
		t.Fatalf("after start server expected to be healthy relatively quickly. %v - %v", string(buf[:]), resp.StatusCode)
	}
	resp.Body.Close()
}
