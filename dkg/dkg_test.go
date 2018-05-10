package dkg

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	"github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/protobuf/drand"
	"github.com/dedis/drand/test"
	sdkg "github.com/dedis/kyber/share/dkg/pedersen"
	"github.com/nikkolasg/slog"
	"github.com/stretchr/testify/require"
)

// testService implements a barebone service to be plugged in a net.Gateway
type testService struct {
	h *Handler
}

func (t *testService) Public(context.Context, *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	return &drand.PublicRandResponse{}, nil
}
func (t *testService) Setup(c context.Context, in *dkg.DKGPacket) (*dkg.DKGResponse, error) {
	t.h.Process(c, in)
	return &dkg.DKGResponse{}, nil
}

func (t *testService) NewBeacon(c context.Context, in *drand.BeaconRequest) (*drand.BeaconResponse, error) {
	return &drand.BeaconResponse{}, nil
}

// testNet implements the network interface that the dkg Handler expects
type testNet struct {
	net.Client
}

func (t *testNet) Send(p net.Peer, d *dkg.DKGPacket) error {
	_, err := t.Client.Setup(p, d)
	return err
}

func testNets(n int) []*testNet {
	nets := make([]*testNet, n, n)
	for i := 0; i < n; i++ {
		nets[i] = &testNet{net.NewGrpcClient()}
	}
	return nets
}

func TestDKG(t *testing.T) {
	slog.Level = slog.LevelDebug

	n := 5
	thr := n/2 + 1
	privs := test.GenerateIDs(n)
	pubs := test.ListFromPrivates(privs)
	nets := testNets(n)
	conf := &Config{
		Suite: key.G2.(sdkg.Suite),
		Group: key.NewGroup(pubs, thr),
	}
	conf.Group.Threshold = thr
	handlers := make([]*Handler, n, n)
	listeners := make([]net.Listener, n, n)
	var err error
	for i := 0; i < n; i++ {
		handlers[i], err = NewHandler(privs[i], conf, nets[i])
		require.NoError(t, err)
		listeners[i] = net.NewTCPGrpcListener(privs[i].Public.Addr, &testService{handlers[i]})
		go listeners[i].Start()
	}
	defer func() {
		fmt.Println("defer")
		for i := 0; i < n; i++ {
			listeners[i].Stop()
		}
	}()
	go handlers[0].Start()
	select {
	case <-handlers[0].WaitShare():
		return
	case err := <-handlers[0].WaitError():
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		fmt.Println("timeout")
		t.Fatal("not finished in time")
	}
}
