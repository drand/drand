package client

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	dhttp "github.com/drand/drand/http"
	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test/mock"
	"google.golang.org/grpc"
)

func withServer(t *testing.T) (string, []byte, context.CancelFunc) {
	t.Helper()
	l, _ := mock.NewGRPCPublicServer(":0")
	lAddr := l.Addr()
	go l.Start()

	conn, err := grpc.Dial(lAddr, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := drand.NewPublicClient(conn)

	handler, err := dhttp.New(ctx, client)
	if err != nil {
		t.Fatal(err)
	}

	protoGroup, _ := l.Group(ctx, &drand.GroupRequest{})
	realGroup, _ := key.GroupFromProto(protoGroup)
	hash := realGroup.Hash()

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	server := http.Server{Handler: handler}
	go server.Serve(listener)
	return listener.Addr().String(), hash, func() {
		server.Shutdown(ctx)
		cancel()
	}
}
func TestHTTPClient(t *testing.T) {
	addr, hash, cancel := withServer(t)
	defer cancel()

	httpClient, err := NewHTTPClient("http://"+addr, hash, &http.Client{})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := httpClient.Get(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Signature) == 0 {
		t.Fatal("no signature provided.")
	}

	if _, err := httpClient.Get(ctx, result.Round+1); err == nil {
		t.Fatal("round n+1 should have an invalid signature")
	}
}
