package http

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	clock "github.com/jonboulle/clockwork"
	json "github.com/nikkolasg/hexjson"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/common/client"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/common/testlogger"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/internal/test"
	"github.com/drand/drand/internal/test/mock"
)

func withClient(t *testing.T, clk clock.Clock) (c client.Client, emit func(bool)) {
	t.Helper()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	lg := testlogger.New(t)
	l, s := mock.NewMockGRPCPublicServer(t, lg, "127.0.0.1:0", true, sch, clk)
	go l.Start()

	c = mock.NewGrpcClient(s.(*mock.Server))
	require.NoError(t, err)

	return c, s.(mock.Service).EmitRand
}

func getWithCtx(ctx context.Context, url string, t *testing.T) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
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
	if rep, ok := body["Round"].(float64); !ok || rep != round {
		return fmt.Errorf("wrong response round number (!%f): %v", round, body)
	}
	return nil
}

func TestHTTPWaiting(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.Skip("test is flacky in CI")
	}

	lg := testlogger.New(t)
	ctx := log.ToContext(context.Background(), lg)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	test.Tracer(t, ctx)

	clk := clock.NewFakeClockAt(time.Now())
	c, push := withClient(t, clk)

	handler, err := New(ctx, "")
	require.NoError(t, err)

	info, err := c.Info(ctx)
	require.NoError(t, err)

	handler.RegisterNewBeaconHandler(c, info.HashString())

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := http.Server{Handler: handler.GetHTTPHandler()}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Shutdown(ctx) }()

	time.Sleep(50 * time.Millisecond)

	// The first request will trigger background watch. 1 get (1969)
	u := fmt.Sprintf("http://%s/%s/public/1", listener.Addr().String(), info.HashString())
	next := getWithCtx(ctx, u, t)
	_ = next.Body.Close()

	// 1 watch get will occur (1970 - the bad one)
	push(false)

	// Wait a bit after we send this request since DrandHandler.getRand() might not contain
	// the expected beacon from above due to lock contention on bh.pendingLk.
	// Note: Removing this sleep will cause the test to randomly break.
	time.Sleep(100 * time.Millisecond)

	done := make(chan time.Time)
	before := clk.Now()
	go func() {
		endpoint := listener.Addr().String() + "/" + info.HashString() + "/public/1971"
		if err = validateEndpoint(endpoint, 1971.0); err != nil {
			done <- time.Unix(0, 0)
			return
		}
		done <- clk.Now()
	}()
	time.Sleep(100 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("shouldn't be done.", err)
	default:
	}
	push(false)

	var after time.Time
	select {
	case x := <-done:
		require.NoError(t, err)
		after = x
	case <-time.After(10 * time.Millisecond):
		t.Fatal("should return after a round")
	}

	t.Logf("comparing values: before: %s after: %s\n", before, after)

	// mock grpc server spits out new round every second on streaming interface.
	if after.Sub(before) > time.Second || after.Sub(before) < 10*time.Millisecond {
		t.Fatalf("unexpected timing to receive response: before: %s after: %s", before, after)
	}
}

func TestHTTPWatchFuture(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	lg := testlogger.New(t)
	ctx := log.ToContext(context.Background(), lg)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	clk := clock.NewFakeClockAt(time.Now())
	c, _ := withClient(t, clk)

	test.Tracer(t, ctx)

	handler, err := New(ctx, "")
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

	time.Sleep(50 * time.Millisecond)

	// watching sets latest round, future rounds should become inaccessible.
	u := fmt.Sprintf("http://%s/%s/public/2000", listener.Addr().String(), info.HashString())
	resp := getWithCtx(ctx, u, t)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatal("response should fail on requests in the future")
	}
}

func TestHTTPHealth(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.Skip("test is flacky in CI")
	}

	lg := testlogger.New(t)
	ctx := log.ToContext(context.Background(), lg)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	test.Tracer(t, ctx)

	clk := clock.NewFakeClockAt(time.Now())
	c, push := withClient(t, clk)

	handler, err := New(ctx, "")
	require.NoError(t, err)

	info, err := c.Info(ctx)
	require.NoError(t, err)

	handler.RegisterNewBeaconHandler(c, info.HashString())

	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	server := http.Server{Handler: handler.GetHTTPHandler()}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Shutdown(ctx) }()

	time.Sleep(50 * time.Millisecond)

	resp := getWithCtx(ctx, fmt.Sprintf("http://%s/%s/health", listener.Addr().String(), info.HashString()), t)
	require.NotEqual(t, http.StatusOK, resp.StatusCode, "newly started server not expected to be synced.")

	resp.Body.Close()

	resp = getWithCtx(ctx, fmt.Sprintf("http://%s/%s/public/0", listener.Addr().String(), info.HashString()), t)
	require.Equal(t, http.StatusOK, resp.StatusCode, "startup of the server on 1st request should happen")

	push(false)
	// Give some time for http server to get it
	// Note: Removing this sleep will cause the test to randomly break.
	time.Sleep(50 * time.Millisecond)
	resp.Body.Close()

	resp = getWithCtx(ctx, fmt.Sprintf("http://%s/%s/health", listener.Addr().String(), info.HashString()), t)
	buf, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	//nolint:lll // This is correct
	require.Equalf(t, http.StatusOK, resp.StatusCode, "after start server expected to be healthy relatively quickly. %v - %v", string(buf), resp.StatusCode)
	resp.Body.Close()
}

func TestHTTP404(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c, _ := withClient(t, clock.NewFakeClock())

	handler, err := New(ctx, "")
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

	// wait to know we're ready to serve
	time.Sleep(50 * time.Millisecond)

	u := fmt.Sprintf("http://%s/deadbeef/public/latest", listener.Addr().String())
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatal("response should 404 on beacon hash that doesn't exist")
	}

	u = fmt.Sprintf("http://%s/deadbeef/public/1", listener.Addr().String())
	resp, err = http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatal("response should 404 on beacon hash that doesn't exist")
	}
}
