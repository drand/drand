package mock

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/core"
	dhttp "github.com/drand/drand/http"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test/mock"
)

// NewMockHTTPPublicServer creates a mock drand HTTP server for testing.
func NewMockHTTPPublicServer(t *testing.T, badSecondRound bool) (string, *chain.Info, context.CancelFunc, func(bool)) {
	t.Helper()

	server := mock.NewMockServer(badSecondRound)
	client := core.Proxy(server)
	ctx, cancel := context.WithCancel(context.Background())

	handler, err := dhttp.New(ctx, client, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	var chainInfo *chain.Info
	for i := 0; i < 3; i++ {
		protoInfo, err := server.ChainInfo(ctx, &drand.ChainInfoRequest{})
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
	httpServer := http.Server{Handler: handler}
	go httpServer.Serve(listener)
	return listener.Addr().String(), chainInfo, func() {
		httpServer.Shutdown(context.Background())
		cancel()
	}, server.(mock.MockService).EmitRand
}
