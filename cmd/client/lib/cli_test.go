package lib

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/drand/drand/client"
	httpmock "github.com/drand/drand/client/test/http/mock"
	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/test/mock"
	"github.com/urfave/cli/v2"
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

func run(args []string) error {
	app := cli.NewApp()
	app.Name = "mock-client"
	app.Flags = ClientFlags
	app.Action = mockAction

	return app.Run(args)
}

func TestClientLib(t *testing.T) {
	opts = []client.Option{}
	err := run([]string{"mock-client"})
	if err == nil {
		t.Fatal("need to specify a connection method.", err)
	}

	sch := scheme.GetSchemeFromEnv()

	addr, info, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false, sch)
	defer cancel()

	grpcLis, _ := mock.NewMockGRPCPublicServer(":0", false, sch)
	go grpcLis.Start()
	defer grpcLis.Stop(context.Background())

	args := []string{"mock-client", "--url", "http://" + addr, "--grpc-connect", grpcLis.Addr(), "--insecure"}

	fmt.Printf("%+v", args)
	err = run(args)
	if err != nil {
		t.Fatal("GRPC should work", err)
	}

	args = []string{"mock-client", "--url", "https://" + addr}

	err = run(args)
	if err == nil {
		t.Fatal("http needs insecure or hash", err)
	}

	args = []string{"mock-client", "--url", "http://" + addr, "--hash", hex.EncodeToString(info.Hash())}

	err = run(args)
	if err != nil {
		t.Fatal("http should construct", err)
	}

	args = []string{"mock-client", "--relay", fakeGossipRelayAddr}

	err = run(args)
	if err == nil {
		t.Fatal("relays need URL or hash", err)
	}

	args = []string{"mock-client", "--relay", fakeGossipRelayAddr, "--hash", hex.EncodeToString(info.Hash())}

	err = run(args)
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientLibGroupConfTOML(t *testing.T) {
	err := run([]string{"mock-client", "--relay", fakeGossipRelayAddr, "--group-conf", groupTOMLPath()})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientLibGroupConfJSON(t *testing.T) {
	sch := scheme.GetSchemeFromEnv()

	addr, info, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false, sch)
	defer cancel()

	var b bytes.Buffer
	info.ToJSON(&b, nil)

	tmpDir, err := os.MkdirTemp(os.TempDir(), "drand")
	if err != nil {
		t.Fatal(err)
	}

	infoPath := filepath.Join(tmpDir, "info.json")

	err = os.WriteFile(infoPath, b.Bytes(), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = run([]string{"mock-client", "--url", "http://" + addr, "--group-conf", infoPath})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientLibChainHashOverrideError(t *testing.T) {
	err := run([]string{
		"mock-client",
		"--relay",
		fakeGossipRelayAddr,
		"--group-conf",
		groupTOMLPath(),
		"--hash",
		fakeChainHash,
	})
	if err == nil {
		t.Fatal("expected error from mismatched chain hashes")
	}
	fmt.Println(err)
}

func TestClientLibListenPort(t *testing.T) {
	err := run([]string{"mock-client", "--relay", fakeGossipRelayAddr, "--port", "0.0.0.0:0", "--group-conf", groupTOMLPath()})
	if err != nil {
		t.Fatal(err)
	}
}

func groupTOMLPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "deploy", "latest", "group.toml")
}
