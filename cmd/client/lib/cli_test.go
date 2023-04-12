package lib

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	clock "github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	"github.com/drand/drand/client"
	httpmock "github.com/drand/drand/client/test/http/mock"
	commonutils "github.com/drand/drand/common"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/log"
	"github.com/drand/drand/test/mock"
	"github.com/drand/drand/test/testlogger"
)

var (
	opts         []client.Option
	latestClient client.Client
)

const (
	fakeGossipRelayAddr = "/ip4/8.8.8.8/tcp/9/p2p/QmSoLju6m7xTh3DuokvT3886QRYqxAzb1kShaanJgW36yx"
	fakeChainHash       = "6093f9e4320c285ac4aab50ba821cd5678ec7c5015d3d9d11ef89e2a99741e83"
)

func mockAction(c *cli.Context) error {
	res, err := Create(c, false, opts...)
	latestClient = res
	return err
}

func run(l log.Logger, args []string) error {
	app := cli.NewApp()
	app.Name = "mock-client"
	app.Flags = ClientFlags
	app.Action = func(c *cli.Context) error {
		c.Context = log.ToContext(c.Context, l)
		return mockAction(c)
	}

	return app.Run(args)
}

func TestClientLib(t *testing.T) {
	opts = []client.Option{}
	lg := testlogger.New(t)
	err := run(lg, []string{"mock-client"})
	if err == nil {
		t.Fatal("need to specify a connection method.", err)
	}

	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr, info, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	grpcLis, _ := mock.NewMockGRPCPublicServer(t, lg, ":0", false, sch, clk)
	go grpcLis.Start()
	defer grpcLis.Stop(context.Background())

	args := []string{"mock-client", "--url", "http://" + addr, "--grpc-connect", grpcLis.Addr(), "--insecure"}
	err = run(lg, args)
	if err != nil {
		t.Fatal("GRPC should work", err)
	}

	args = []string{"mock-client", "--url", "https://" + addr}
	err = run(lg, args)
	if err == nil {
		t.Fatal("http needs insecure or hash", err)
	}

	args = []string{"mock-client", "--url", "http://" + addr, "--hash", hex.EncodeToString(info.Hash())}
	err = run(lg, args)
	if err != nil {
		t.Fatal("http should construct", err)
	}

	args = []string{"mock-client", "--relay", fakeGossipRelayAddr}
	err = run(lg, args)
	if err == nil {
		t.Fatal("relays need URL to get chain info and hash", err)
	}

	args = []string{"mock-client", "--relay", fakeGossipRelayAddr, "--hash", hex.EncodeToString(info.Hash())}
	err = run(lg, args)
	if err == nil {
		t.Fatal("relays need URL to get chain info and hash", err)
	}

	args = []string{"mock-client", "--url", "http://" + addr, "--relay", fakeGossipRelayAddr, "--hash", hex.EncodeToString(info.Hash())}
	err = run(lg, args)
	if err != nil {
		t.Fatal("unable to get relay to work", err)
	}
}

func TestClientLibGroupConfTOML(t *testing.T) {
	lg := testlogger.New(t)
	err := run(lg, []string{"mock-client", "--relay", fakeGossipRelayAddr, "--group-conf", groupTOMLPath()})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientLibGroupConfJSON(t *testing.T) {
	lg := testlogger.New(t)
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr, info, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	var b bytes.Buffer
	require.NoError(t, info.ToJSON(&b, nil))

	infoPath := filepath.Join(t.TempDir(), "info.json")

	err = os.WriteFile(infoPath, b.Bytes(), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = run(lg, []string{"mock-client", "--url", "http://" + addr, "--group-conf", infoPath})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientLibChainHashOverrideError(t *testing.T) {
	lg := testlogger.New(t)
	err := run(lg, []string{
		"mock-client",
		"--relay",
		fakeGossipRelayAddr,
		"--group-conf",
		groupTOMLPath(),
		"--hash",
		fakeChainHash,
	})
	if !errors.Is(err, commonutils.ErrInvalidChainHash) {
		t.Fatal("expected error from mismatched chain hashes. Got: ", err)
	}
}

func TestClientLibListenPort(t *testing.T) {
	lg := testlogger.New(t)
	err := run(lg, []string{"mock-client", "--relay", fakeGossipRelayAddr, "--port", "0.0.0.0:0", "--group-conf", groupTOMLPath()})
	if err != nil {
		t.Fatal(err)
	}
}

func groupTOMLPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "test", "default.toml")
}
