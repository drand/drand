package http

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test/mock"
	"github.com/stretchr/testify/require"

	json "github.com/nikkolasg/hexjson"
	"google.golang.org/grpc"
)

func withClient(t *testing.T) drand.PublicClient {
	t.Helper()

	l, _ := mock.NewMockGRPCPublicServer(":0", true)
	lAddr := l.Addr()
	go l.Start()

	conn, err := grpc.Dial(lAddr, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}

	client := drand.NewPublicClient(conn)
	return client
}

func TestHTTPRelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := withClient(t)

	handler, err := New(ctx, client, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	server := http.Server{Handler: handler}
	go server.Serve(listener)
	defer server.Shutdown(ctx)
	time.Sleep(100 * time.Millisecond)

	getChain := fmt.Sprintf("http://%s/info", listener.Addr().String())
	resp, err := http.Get(getChain)
	require.NoError(t, err)
	cip := new(drand.ChainInfoPacket)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(cip))
	require.NotNil(t, cip.Hash)
	require.NotNil(t, cip.PublicKey)

	// Test exported interfaces.
	u := fmt.Sprintf("http://%s/public/2", listener.Addr().String())
	resp, err = http.Get(u)
	require.NoError(t, err)
	body := make(map[string]interface{})
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

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

	if _, ok := body["round"]; !ok {
		t.Fatal("expected signature in latest response.")
	}
}

func TestHTTPWaiting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := withClient(t)

	handler, err := New(ctx, client, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	server := http.Server{Handler: handler}
	go server.Serve(listener)
	defer server.Shutdown(ctx)

	// The first request will trigger background watch. 1 get (1969)
	next, _ := http.Get(fmt.Sprintf("http://%s/public/0", listener.Addr().String()))

	// 1 additional watch get will occur (1970 - the bad one)
	time.Sleep(1200 * time.Millisecond)
	body := make(map[string]interface{})
	before := time.Now()
	next, _ = http.Get(fmt.Sprintf("http://%s/public/1971", listener.Addr().String()))
	after := time.Now()
	if err = json.NewDecoder(next.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["round"].(float64) != 1971.0 {
		t.Fatalf("wrong response round number: %v", body)
	}

	// mock grpc server spits out new round every second on streaming interface.
	if after.Sub(before) > time.Second || after.Sub(before) < 10*time.Millisecond {
		t.Fatalf("unexpected timing to receive %v", body)
	}

	// watching sets latest round, future rounds should become inaccessible.
	u := fmt.Sprintf("http://%s/public/2000", listener.Addr().String())
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatal("response should fail on requests in the future")
	}
}
