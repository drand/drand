package net

import (
	"context"
	"testing"
	"time"

	"github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/protobuf/drand"
	"github.com/stretchr/testify/require"
)

type testPeer struct {
	addr string
}

func (t *testPeer) Address() string {
	return t.addr
}

func (t *testPeer) TLS() bool {
	return false
}

type testService struct {
	ts uint64
}

func (t *testService) Public(context.Context, *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	return &drand.PublicRandResponse{Timestamp: t.ts}, nil
}
func (t *testService) Setup(c context.Context, in *dkg.DKGPacket) (*dkg.DKGResponse, error) {
	return &dkg.DKGResponse{}, nil
}
func (t *testService) NewBeacon(c context.Context, in *drand.BeaconPacket) (*drand.BeaconResponse, error) {
	return &drand.BeaconResponse{}, nil
}

func TestGatewa(t *testing.T) {
	addr1 := "127.0.0.1:4000"
	//addr2 := "127.0.0.1:4001"
	lis1 := NewTCPGrpcListener(addr1)
	service1 := &testService{42}
	lis1.RegisterDrandService(service1)
	go lis1.Start()
	defer lis1.Stop()
	time.Sleep(100 * time.Millisecond)
	client := NewGrpcClient()
	resp, err := client.Public(&testPeer{addr1}, &drand.PublicRandRequest{})
	require.Nil(t, err)
	expected := &drand.PublicRandResponse{Timestamp: service1.ts}
	require.Equal(t, expected.GetTimestamp(), resp.GetTimestamp())
}
