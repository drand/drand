package mock

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	dhttp "github.com/drand/drand/http"
	"github.com/drand/drand/protobuf/drand"
	"google.golang.org/grpc"
)

// NewMockHTTPPublicServer creates a mock drand HTTP server for testing.
func NewMockHTTPPublicServer(t *testing.T, badSecondRound bool) (string, *chain.Info, context.CancelFunc) {
	t.Helper()
	l, s := NewMockGRPCPublicServer(":0", badSecondRound)
	lAddr := l.Addr()
	go l.Start()

	conn, err := grpc.Dial(lAddr, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	client := drand.NewPublicClient(conn)

	handler, err := dhttp.New(ctx, client, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	var chainInfo *chain.Info
	for i := 0; i < 3; i++ {
		protoInfo, err := s.ChainInfo(ctx, &drand.ChainInfoRequest{})
		if err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		chainInfo, err = chain.InfoFromProto(protoInfo)
		if err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		break
	}
	if chainInfo == nil {
		t.Fatal("could not use server after 3 attempts.")
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	server := http.Server{Handler: handler}
	go server.Serve(listener)
	return listener.Addr().String(), chainInfo, func() {
		server.Shutdown(ctx)
		cancel()
	}
}
