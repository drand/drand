package mock

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	chainCommon "github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/internal/core"
	dhttp "github.com/drand/drand/internal/http"
	"github.com/drand/drand/internal/test/mock"
	"github.com/drand/drand/internal/test/testlogger"
	"github.com/drand/drand/protobuf/drand"
	clock "github.com/jonboulle/clockwork"
)

// NewMockHTTPPublicServer creates a mock drand HTTP server for testing.
func NewMockHTTPPublicServer(t *testing.T, badSecondRound bool, sch *crypto.Scheme, clk clock.Clock) (string, *chainCommon.Info, context.CancelFunc, func(bool)) {
	t.Helper()
	ctx := context.Background()

	server := mock.NewMockServer(t, badSecondRound, sch, clk)
	client := core.Proxy(server)

	lg := testlogger.New(t)
	lctx := log.ToContext(context.Background(), lg)
	ctx, cancel := context.WithCancel(lctx)

	handler, err := dhttp.New(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	var chainInfo *chainCommon.Info
	for i := 0; i < 3; i++ {
		protoInfo, err := server.ChainInfo(ctx, &drand.ChainInfoRequest{})
		if err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		chainInfo, err = chainCommon.InfoFromProto(protoInfo)
		if err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		break
	}
	if chainInfo == nil {
		t.Fatal("could not use server after 3 attempts.")
	}

	handler.RegisterNewBeaconHandler(client, chainInfo.HashString())

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	httpServer := http.Server{Handler: handler.GetHTTPHandler(), ReadHeaderTimeout: 3 * time.Second}
	go httpServer.Serve(listener)

	return listener.Addr().String(), chainInfo, func() {
		httpServer.Shutdown(ctx)
		cancel()
	}, server.(mock.Service).EmitRand
}
