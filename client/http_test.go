package client

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	dhttp "github.com/drand/drand/http"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test/mock"
	"google.golang.org/grpc"
)

func withServer(t *testing.T) (string, []byte, context.CancelFunc) {
	t.Helper()
	l, s := mock.NewMockGRPCPublicServer(":0", false)
	lAddr := l.Addr()
	go l.Start()

	conn, err := grpc.Dial(lAddr, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := drand.NewPublicClient(conn)

	handler, err := dhttp.New(ctx, client, nil)
	if err != nil {
		t.Fatal(err)
	}

	var hash []byte
	for i := 0; i < 3; i++ {
		protoInfo, err := s.ChainInfo(ctx, &drand.ChainInfoRequest{})
		if err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		chainInfo, err := chain.InfoFromProto(protoInfo)
		if err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		hash = chainInfo.Hash()
		break
	}
	if hash == nil {
		t.Fatal("could not use server after 3 attempts.")
	}

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
	if len(result.Randomness()) == 0 {
		t.Fatal("no randomness provided")
	}
	full, ok := (result).(*RandomData)
	if !ok {
		t.Fatal("Should be able to restore concrete type")
	}
	if len(full.Sig) == 0 {
		t.Fatal("no signature provided")
	}

	if _, err := httpClient.Get(ctx, full.Rnd+1); err == nil {
		t.Fatal("round n+1 should have an invalid signature")
	}
}

func TestHTTPWatch(t *testing.T) {
	addr, hash, cancel := withServer(t)
	defer cancel()

	httpClient, err := NewHTTPClient("http://"+addr, hash, &http.Client{})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := httpClient.Watch(ctx)
	first, ok := <-result
	if !ok {
		t.Fatal("Should get a result from watching")
	}
	if len(first.Randomness()) == 0 {
		t.Fatal("should get randomness from watching")
	}
	_, ok = <-result
	if ok {
		// Note. there is a second value polled for by the client, but it will
		// be invalid per the mocked grpc backing server.
		t.Fatal("second result should fail per context timeout")
	}
}
