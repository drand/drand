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
	"github.com/stretchr/testify/require"
)

// testService implements a barebone service to be plugged in a net.Gateway
type testService struct {
	h *Handler
}

func (t *testService) Public(context.Context, *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	return &drand.PublicRandResponse{}, nil
}
func (t *testService) Private(context.Context, *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	return &drand.PrivateRandResponse{}, nil
}
func (t *testService) DistKey(context.Context, *drand.DistKeyRequest) (*drand.DistKeyResponse, error) {
	return &drand.DistKeyResponse{}, nil
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
	net.InternalClient
}

func (t *testNet) Send(p net.Peer, d *dkg.DKGPacket) error {
	_, err := t.InternalClient.Setup(p, d)
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
	//slog.Level = slog.LevelDebug

	n := 5
	thr := key.DefaultThreshold(n)
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
		for i := 0; i < n; i++ {
			listeners[i].Stop()
		}
	}()

	finished := make(chan int, n)
	goDkg := func(idx int) {
		if idx == 0 {
			go handlers[idx].Start()
		}
		shareCh := handlers[idx].WaitShare()
		errCh := handlers[idx].WaitError()
		select {
		case <-shareCh:
			finished <- idx
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(3 * time.Second):
			fmt.Println("timeout")
			t.Fatal("not finished in time")
		}
	}

	for i := 0; i < n; i++ {
		go goDkg(i)
	}
	for i := 0; i < n; i++ {
		<-finished
	}
}
