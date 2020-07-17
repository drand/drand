package client

import (
	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/client/grpc"
	dhttp "github.com/drand/drand/client/http"
	httpmock "github.com/drand/drand/client/test/http/mock"
	"github.com/drand/drand/log"
	"github.com/drand/drand/lp2p"
	"github.com/drand/drand/test"
	"github.com/drand/drand/test/mock"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"

	bds "github.com/ipfs/go-ds-badger2"
	ma "github.com/multiformats/go-multiaddr"
)

func TestGRPCClient(t *testing.T) {
	// start mock drand node
	grpcLis, svc := mock.NewMockGRPCPublicServer(":0", false)
	grpcAddr := grpcLis.Addr()
	go grpcLis.Start()
	defer grpcLis.Stop(context.Background())

	dataDir, err := ioutil.TempDir(os.TempDir(), "test-gossip-relay-node-datastore")
	if err != nil {
		t.Fatal(err)
	}
	identityDir, err := ioutil.TempDir(os.TempDir(), "test-gossip-relay-node-id")
	if err != nil {
		t.Fatal(err)
	}

	infoProto, err := svc.ChainInfo(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := chain.InfoFromProto(infoProto)
	info.GenesisTime -= 10

	// start mock relay-node
	grpcClient, err := grpc.New(grpcAddr, "", true)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &lp2p.GossipRelayConfig{
		ChainHash:    hex.EncodeToString(info.Hash()),
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
	c, err := newTestClient("test-gossip-relay-client", g.Multiaddrs(), info)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// test client
	ctx, cancel := context.WithCancel(context.Background())
	// for the initial 'get' to sync the chain
	svc.(mock.MockService).EmitRand(false)
	ch := c.Watch(ctx)
	time.Sleep(100 * time.Millisecond)
	for i := 0; i < 3; i++ {
		svc.(mock.MockService).EmitRand(false)
		fmt.Printf("round %d. emitting.\n", i)
		select {
		case r, ok := <-ch:
			if !ok {
				t.Fatal("expected randomness")
			} else {
				fmt.Print(r)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("timeout.")
		}
	}
	svc.(mock.MockService).EmitRand(true)
	cancel()
	drain(t, ch, 10*time.Second)
}

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

func TestHTTPClient(t *testing.T) {
	addr, chainInfo, stop, emit := httpmock.NewMockHTTPPublicServer(t, false)
	defer stop()

	dataDir, err := ioutil.TempDir(os.TempDir(), "test-gossip-relay-node-datastore")
	if err != nil {
		t.Fatal(err)
	}
	identityDir, err := ioutil.TempDir(os.TempDir(), "test-gossip-relay-node-id")
	if err != nil {
		t.Fatal(err)
	}

	httpClient, err := dhttp.New("http://"+addr, chainInfo.Hash(), http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}
	chainInfo.GenesisTime -= 10
	cfg := &lp2p.GossipRelayConfig{
		ChainHash:    hex.EncodeToString(chainInfo.Hash()),
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

	c, err := newTestClient("test-http-gossip-relay-client", g.Multiaddrs(), chainInfo)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	emit(false)
	ch := c.Watch(ctx)
	time.Sleep(5 * time.Millisecond)
	for i := 0; i < 3; i++ {
		emit(false)
		select {
		case r, ok := <-ch:
			if !ok {
				t.Fatal("expected randomness")
			} else {
				fmt.Print(r)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("timeout.")
		}
	}
	emit(true)
	cancel()
	drain(t, ch, 10*time.Second)
}

func newTestClient(name string, relayMultiaddr []ma.Multiaddr, info *chain.Info) (*Client, error) {
	dataDir, err := ioutil.TempDir(os.TempDir(), "client-"+name+"-datastore")
	if err != nil {
		return nil, err
	}
	identityDir, err := ioutil.TempDir(os.TempDir(), "client-"+name+"-id")
	if err != nil {
		return nil, err
	}
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

func peerIDFromMultiaddr(addr ma.Multiaddr) (peer.ID, error) {
	ai, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return "", err
	}
	return ai.ID, nil
}

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
