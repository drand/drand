package main

import (
	"context"
	"encoding/hex"
	"fmt"

	clock "github.com/jonboulle/clockwork"
	json "github.com/nikkolasg/hexjson"

	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/protobuf/drand"
	"github.com/drand/drand/v2/test/mock"
)

const serve = "127.0.0.1:1969"

type TestJSON struct {
	Public string
	API    *drand.PublicRandResponse
}

func main() {
	ctx := context.Background()
	lg := log.New(nil, log.DebugLevel, false)
	sch, err := crypto.GetSchemeFromEnv()
	if err != nil {
		panic(err)
	}
	clk := clock.NewRealClock()
	listener, server := mock.NewMockGRPCPublicServer(nil, lg, serve, true, sch, clk)
	resp, err := server.PublicRand(ctx, &drand.PublicRandRequest{})
	if err != nil {
		panic(err)
	}
	ci, err := server.ChainInfo(ctx, &drand.ChainInfoRequest{})
	if err != nil {
		panic(err)
	}

	tjson := &TestJSON{
		Public: hex.EncodeToString(ci.PublicKey),
		API:    resp,
	}
	s, _ := json.MarshalIndent(tjson, "", "    ")
	fmt.Println(string(s))

	fmt.Println("server will listen on ", serve)
	listener.Start()
}
