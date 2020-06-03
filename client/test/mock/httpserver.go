package mock

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	grpcc "github.com/drand/drand/client/grpc"
	dhttp "github.com/drand/drand/http"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test/mock"
)

// NewMockHTTPPublicServer creates a mock drand HTTP server for testing.
func NewMockHTTPPublicServer(t *testing.T, badSecondRound bool) (string, *chain.Info, context.CancelFunc, func(bool)) {
	t.Helper()
	l, s := mock.NewMockGRPCPublicServer(":0", badSecondRound)
	lAddr := l.Addr()
	go l.Start()

	ctx, cancel := context.WithCancel(context.Background())
	client, err := grpcc.New(lAddr, "", true)
	if err != nil {
		t.Fatal(err)
	}

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
	}, s.(mock.MockService).EmitRand
}
