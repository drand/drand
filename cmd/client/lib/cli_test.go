package lib

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/drand/drand/client"
	cmock "github.com/drand/drand/client/test/mock"
	"github.com/drand/drand/test/mock"
	"github.com/urfave/cli/v2"
)

var (
	opts         []client.Option
	latestClient client.Client
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

	addr, info, cancel, _ := cmock.NewMockHTTPPublicServer(t, false)
	defer cancel()

	grpcLis, _ := mock.NewMockGRPCPublicServer(":0", false)
	go grpcLis.Start()
	defer grpcLis.Stop(context.Background())

	err = run([]string{"mock-client", "--url", "http://" + addr, "--grpc-connect", grpcLis.Addr(), "--insecure"})
	if err != nil {
		t.Fatal("GRPC should work", err)
	}

	err = run([]string{"mock-client", "--url", "https://" + addr})
	if err == nil {
		t.Fatal("http needs insecure or hash", err)
	}

	err = run([]string{"mock-client", "--url", "http://" + addr, "--hash", hex.EncodeToString(info.Hash())})
	if err != nil {
		t.Fatal("http should construct", err)
	}

	err = run([]string{"mock-client", "--relay", "/ip4/8.8.8.8/tcp/9/p2p/QmSoLju6m7xTh3DuokvT3886QRYqxAzb1kShaanJgW36yx"})
	if err == nil {
		t.Fatal("relays need URL or hash", err)
	}

	err = run([]string{"mock-client", "--relay", "/ip4/8.8.8.8/tcp/9/p2p/QmSoLju6m7xTh3DuokvT3886QRYqxAzb1kShaanJgW36yx", "--hash", hex.EncodeToString(info.Hash())})
	if err != nil {
		t.Fatal(err)
	}
}
