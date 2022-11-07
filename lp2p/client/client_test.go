package client

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"testing"
	"time"

	bds "github.com/ipfs/go-ds-badger2"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/client/grpc"
	dhttp "github.com/drand/drand/client/http"
	httpmock "github.com/drand/drand/client/test/http/mock"
	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/log"
	"github.com/drand/drand/lp2p"
	"github.com/drand/drand/test"
	"github.com/drand/drand/test/mock"
)

func TestGRPCClientTestFunc(t *testing.T) {
	t.Skip("TestGRPCClientTestFunc is flaky")
	// start mock drand node
	sch := scheme.GetSchemeFromEnv()

	grpcLis, svc := mock.NewMockGRPCPublicServer(":0", false, sch)
	grpcAddr := grpcLis.Addr()
	go grpcLis.Start()
	defer grpcLis.Stop(context.Background())

	dataDir := t.TempDir()
	identityDir := t.TempDir()

	infoProto, err := svc.ChainInfo(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := chain.InfoFromProto(infoProto)

	// start mock relay-node
	grpcClient, err := grpc.New(grpcAddr, "", true, []byte(""))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &lp2p.GossipRelayConfig{
		ChainHash:    info.HashString(),
		PeerWith:     nil,
		Addr:         "/ip4/0.0.0.0/tcp/" + test.FreePort(),
		DataDir:      dataDir,
		IdentityPath: path.Join(identityDir, "identity.key"),
		Client:       grpcClient,
	}
	g, err := lp2p.NewGossipRelayNode(log.DefaultLogger(), cfg)
	if err != nil {
		t.Fatalf("gossip relay node (%v)", err)
	}
	defer g.Shutdown()

	// start client
	c, err := newTestClient(t, g.Multiaddrs(), info)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// test client
	ctx, cancel := context.WithCancel(context.Background())
	ch := c.Watch(ctx)
	for i := 0; i < 3; i++ {
		// pub sub polls every 200ms
		time.Sleep(250 * time.Millisecond)
		svc.(mock.MockService).EmitRand(false)
		fmt.Printf("round %d. emitted.\n", i)
		select {
		case r, ok := <-ch:
			if !ok {
				t.Fatal("expected randomness, watch outer channel was closed instead")
			} else {
				t.Log("received", r.Round())
			}
		// the period of the mock servers is 1 second
		case <-time.After(5 * time.Second):
			t.Fatal("timeout.")
		}
	}
	svc.(mock.MockService).EmitRand(true)
	cancel()
	drain(t, ch, 5*time.Second)
}

//nolint:unused
func drain(t *testing.T, ch <-chan client.Result, timeout time.Duration) {
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-time.After(timeout):
			t.Fatal("timed out closing channel.")
		}
	}
}

func TestHTTPClientTestFunc(t *testing.T) {
	t.Skip("TestHTTPClientTestFunc is flaky")
	sch := scheme.GetSchemeFromEnv()

	addr, chainInfo, stop, emit := httpmock.NewMockHTTPPublicServer(t, false, sch)
	defer stop()

	dataDir := t.TempDir()
	identityDir := t.TempDir()

	httpClient, err := dhttp.New("http://"+addr, chainInfo.Hash(), http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &lp2p.GossipRelayConfig{
		ChainHash:    chainInfo.HashString(),
		PeerWith:     nil,
		Addr:         "/ip4/0.0.0.0/tcp/" + test.FreePort(),
		DataDir:      dataDir,
		IdentityPath: path.Join(identityDir, "identity.key"),
		Client:       httpClient,
	}
	g, err := lp2p.NewGossipRelayNode(log.DefaultLogger(), cfg)
	if err != nil {
		t.Fatalf("gossip relay node (%v)", err)
	}
	defer g.Shutdown()

	c, err := newTestClient(t, g.Multiaddrs(), chainInfo)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	emit(false)
	ch := c.Watch(ctx)
	for i := 0; i < 3; i++ {
		// pub sub polls every 200ms, but the other http one polls every period
		time.Sleep(1250 * time.Millisecond)
		emit(false)
		select {
		case r, ok := <-ch:
			if !ok {
				t.Fatal("expected randomness")
			} else {
				t.Log("received randomness", r.Round())
			}
		case <-time.After(8 * time.Second):
			t.Fatal("timeout.")
		}
	}
	emit(true)
	cancel()
	drain(t, ch, 5*time.Second)
}

//nolint:unused
func newTestClient(t *testing.T, relayMultiaddr []ma.Multiaddr, info *chain.Info) (*Client, error) {
	dataDir := t.TempDir()
	identityDir := t.TempDir()
	ds, err := bds.NewDatastore(dataDir, nil)
	if err != nil {
		return nil, err
	}
	priv, err := lp2p.LoadOrCreatePrivKey(path.Join(identityDir, "identity.key"), log.DefaultLogger())
	if err != nil {
		return nil, err
	}
	h, ps, err := lp2p.ConstructHost(
		ds,
		priv,
		"/ip4/0.0.0.0/tcp/"+test.FreePort(),
		relayMultiaddr,
		log.DefaultLogger(),
	)
	if err != nil {
		return nil, err
	}
	relayPeerID, err := peerIDFromMultiaddr(relayMultiaddr[0])
	if err != nil {
		return nil, err
	}
	err = waitForConnection(h, relayPeerID, time.Minute)
	if err != nil {
		return nil, err
	}
	c, err := NewWithPubsub(ps, info, nil)
	if err != nil {
		return nil, err
	}
	c.SetLog(log.DefaultLogger())
	return c, nil
}

//nolint:unused
func peerIDFromMultiaddr(addr ma.Multiaddr) (peer.ID, error) {
	ai, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return "", err
	}
	return ai.ID, nil
}

//nolint:unused
func waitForConnection(h host.Host, id peer.ID, timeout time.Duration) error {
	t := time.NewTimer(timeout)
	for {
		if len(h.Network().ConnsToPeer(id)) > 0 {
			t.Stop()
			return nil
		}
		select {
		case <-t.C:
			return fmt.Errorf("timed out waiting to be connected the relay @ %v", id)
		default:
		}
		time.Sleep(time.Millisecond * 100)
	}
}
