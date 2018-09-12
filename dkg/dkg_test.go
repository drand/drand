package dkg

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	"github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/test"
	sdkg "github.com/dedis/kyber/share/dkg/pedersen"
	"github.com/nikkolasg/slog"
	"github.com/stretchr/testify/require"
)

// testDKGServer implements a barebone service to be plugged in a net.DefaultService
type testDKGServer struct {
	h *Handler
}

func (t *testDKGServer) Setup(c context.Context, in *dkg.DKGPacket) (*dkg.DKGResponse, error) {
	t.h.Process(c, in)
	return &dkg.DKGResponse{}, nil
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
	slog.Level = slog.LevelDebug
	n := 5
	thr := key.DefaultThreshold(n)
	privs := test.GenerateIDs(n)
	pubs := test.ListFromPrivates(privs)
	nets := testNets(n)
	handlers := make([]*Handler, n, n)
	listeners := make([]net.Listener, n, n)
	var err error

	group := key.NewGroup(pubs, thr)
	for i := 0; i < n; i++ {
		dkgConf := sdkg.NewDKGConfig(key.G2.(sdkg.Suite),
			privs[i].Key,
			group.Points())
		dkgConf.Threshold = thr
		conf := &Config{
			DKG:      dkgConf,
			Key:      privs[i],
			NewNodes: group,
		}

		handlers[i], err = NewHandler(nets[i], conf)
		require.NoError(t, err)
		dkgServer := testDKGServer{h: handlers[i]}
		listeners[i] = net.NewTCPGrpcListener(privs[i].Public.Addr, &net.DefaultService{D: &dkgServer})
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
