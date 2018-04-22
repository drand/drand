package net

import (
	"context"
	"testing"
	"time"

	"github.com/dedis/drand/protobuf/beacon"
	"github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/protobuf/external"
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

type testService struct{}

func (t *testService) Public(context.Context, *external.PublicRandRequest) (*external.PublicRandResponse, error) {
	return &external.PublicRandResponse{}, nil
}
func (t *testService) Setup(c context.Context, in *dkg.DKGPacket) (*dkg.DKGResponse, error) {
	return &dkg.DKGResponse{}, nil
}
func (t *testService) NewBeacon(c context.Context, in *beacon.BeaconPacket) (*beacon.BeaconResponse, error) {
	return &beacon.BeaconResponse{}, nil
}

func TestGateway(t *testing.T) {
	addr1 := "127.0.0.1:4000"
	//addr2 := "127.0.0.1:4001"
	lis1 := NewTCPGrpcListener(addr1)
	service1 := new(testService)
	lis1.RegisterDrandService(service1)
	go lis1.Start()
	time.Sleep(100 * time.Millisecond)
	client := NewGrpcClient()
	resp, err := client.Public(&testPeer{addr1}, &external.PublicRandRequest{})
	require.Nil(t, err)
	expected := &external.PublicRandResponse{}
	require.Equal(t, resp.GetTimestamp(), expected.GetTimestamp())
}
