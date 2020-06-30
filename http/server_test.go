package http

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/drand/drand/client"
	"github.com/drand/drand/client/grpc"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test/mock"
	"github.com/stretchr/testify/require"

	json "github.com/nikkolasg/hexjson"
)

func withClient(t *testing.T) (c client.Client, emit func(bool)) {
	t.Helper()

	l, s := mock.NewMockGRPCPublicServer(":0", true)
	lAddr := l.Addr()
	go l.Start()

	c, _ = grpc.New(lAddr, "", true)

	return c, s.(mock.MockService).EmitRand
}

func TestHTTPRelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, _ := withClient(t)

	handler, err := New(ctx, c, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	server := http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Shutdown(ctx) }()
	time.Sleep(100 * time.Millisecond)

	getChain := fmt.Sprintf("http://%s/info", listener.Addr().String())
	resp, err := http.Get(getChain)
	require.NoError(t, err)
	cip := new(drand.ChainInfoPacket)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(cip))
	require.NotNil(t, cip.Hash)
	require.NotNil(t, cip.PublicKey)
	require.NoError(t, resp.Body.Close())

	// Test exported interfaces.
	u := fmt.Sprintf("http://%s/public/2", listener.Addr().String())
	resp, err = http.Get(u)
	require.NoError(t, err)
	body := make(map[string]interface{})
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.NoError(t, resp.Body.Close())

	if _, ok := body["signature"]; !ok {
		t.Fatal("expected signature in random response.")
	}

	resp, err = http.Get(fmt.Sprintf("http://%s/public/latest", listener.Addr().String()))
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

	handler, err := New(ctx, c, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	server := http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Shutdown(ctx) }()

	// The first request will trigger background watch. 1 get (1969)
	next, err := http.Get(fmt.Sprintf("http://%s/public/0", listener.Addr().String()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = next.Body.Close() }()

	// 1 watch get will occur (1970 - the bad one)
	push(false)
	done := make(chan time.Time)
	before := time.Now()
	go func() {
		if err = validateEndpoint(listener.Addr().String()+"/public/1971", 1971.0); err != nil {
			done <- time.Unix(0, 0)
			return
		}
		done <- time.Now()
	}()
	time.Sleep(50 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("shouldn't be done.", err)
	default:
	}
	push(false)
	time.Sleep(10 * time.Millisecond)
	var after time.Time
	select {
	case x := <-done:
		if err != nil {
			t.Fatal(err)
		}
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

	handler, err := New(ctx, c, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	server := http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Shutdown(ctx) }()

	// watching sets latest round, future rounds should become inaccessible.
	u := fmt.Sprintf("http://%s/public/2000", listener.Addr().String())
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatal("response should fail on requests in the future")
	}
}

func TestHTTPHealth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, push := withClient(t)

	handler, err := New(ctx, c, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	server := http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Shutdown(ctx) }()

	resp, _ := http.Get(fmt.Sprintf("http://%s/health", listener.Addr().String()))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("newly started server not expected to be synced.")
	}

	resp, _ = http.Get(fmt.Sprintf("http://%s/public/0", listener.Addr().String()))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("startup of the server on 1st request should happen")
	}
	push(false)
	// give some time for http server to get it
	time.Sleep(30 * time.Millisecond)
	resp, _ = http.Get(fmt.Sprintf("http://%s/health", listener.Addr().String()))
	if resp.StatusCode != http.StatusOK {
		var buf [100]byte
		_, _ = resp.Body.Read(buf[:])
		t.Fatalf("after start server expected to be healthy relatively quickly. %v - %v", string(buf[:]), resp.StatusCode)
	}
}
