package main

import (
	"context"
	"encoding/hex"
	"fmt"

	json "github.com/nikkolasg/hexjson"

	"github.com/drand/drand/crypto"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test/mock"
)

const serve = "localhost:1969"

type TestJSON struct {
	Public string
	API    *drand.PublicRandResponse
}

func main() {
	sch, err := crypto.GetSchemeFromEnv()
	if err != nil {
		panic(err)
	}
	listener, server := mock.NewMockGRPCPublicServer(nil, serve, true, sch)
	resp, err := server.PublicRand(context.TODO(), &drand.PublicRandRequest{})
	if err != nil {
		panic(err)
	}
	ci, err := server.ChainInfo(context.TODO(), &drand.ChainInfoRequest{})
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
