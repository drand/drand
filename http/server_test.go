package http

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"

	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test/mock"

	json "github.com/nikkolasg/hexjson"
	"google.golang.org/grpc"
)

func TestHTTPRelay(t *testing.T) {
	l, _ := mock.NewMockGRPCPublicServer(":0")
	lAddr := l.Addr()
	go l.Start()

	conn, err := grpc.Dial(lAddr, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := drand.NewPublicClient(conn)

	handler, err := New(ctx, client, nil)
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

	// Test exported interfaces.
	u := fmt.Sprintf("http://%s/public/1", listener.Addr().String())
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	body := make(map[string]interface{})

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

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
