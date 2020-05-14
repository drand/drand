package main

import (
	"context"
	"encoding/hex"
	"fmt"

	json "github.com/nikkolasg/hexjson"

	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test/mock"
)

const serve = "localhost:1969"

type TestJSON struct {
	Public string
	API    *drand.PublicRandResponse
}

func main() {
	listener, server := mock.NewMockGRPCPublicServer(serve)
	resp, err := server.PublicRand(context.TODO(), &drand.PublicRandRequest{})
	if err != nil {
		panic(err)
	}
	dk, err := server.DistKey(context.TODO(), &drand.DistKeyRequest{})

	tjson := &TestJSON{
		Public: hex.EncodeToString(dk.Key),
		API:    resp,
	}
	s, _ := json.MarshalIndent(tjson, "", "    ")
	fmt.Println(string(s))

	fmt.Println("server will listen on ", serve)
	listener.Start()
}
