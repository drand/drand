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
	"github.com/drand/drand/client/basic"
	"github.com/drand/drand/client/grpc"
	cmock "github.com/drand/drand/client/test/mock"
	"github.com/drand/drand/log"
	"github.com/drand/drand/lp2p"
	"github.com/drand/drand/test"
	"github.com/drand/drand/test/mock"

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
	client, err := grpc.New(grpcAddr, "", true)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &lp2p.GossipRelayConfig{
		ChainHash:    hex.EncodeToString(info.Hash()),
		PeerWith:     nil,
		Addr:         "/ip4/0.0.0.0/tcp/" + test.FreePort(),
		DataDir:      dataDir,
		IdentityPath: path.Join(identityDir, "identity.key"),
		Client:       client,
	}
	g, err := lp2p.NewGossipRelayNode(log.DefaultLogger, cfg)
	if err != nil {
		t.Fatalf("gossip relay node (%v)", err)
	}
	defer g.Shutdown()

	// start client
	c, err := newTestClient("test-gossip-relay-client", g.Multiaddrs(), info)
	if err != nil {
		t.Fatal(err)
	}

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
	for range ch {
	}
}

func TestHTTPClient(t *testing.T) {
	addr, chainInfo, stop, emit := cmock.NewMockHTTPPublicServer(t, false)
	defer stop()

	dataDir, err := ioutil.TempDir(os.TempDir(), "test-gossip-relay-node-datastore")
	if err != nil {
		t.Fatal(err)
	}
	identityDir, err := ioutil.TempDir(os.TempDir(), "test-gossip-relay-node-id")
	if err != nil {
		t.Fatal(err)
	}

	client, err := basic.NewHTTPClient("http://"+addr, chainInfo.Hash(), http.DefaultTransport)
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
		Client:       client,
	}
	g, err := lp2p.NewGossipRelayNode(log.DefaultLogger, cfg)
	if err != nil {
		t.Fatalf("gossip relay node (%v)", err)
	}
	defer g.Shutdown()

	c, err := newTestClient("test-http-gossip-relay-client", g.Multiaddrs(), chainInfo)
	if err != nil {
		t.Fatal(err)
	}

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
		case <-time.After(2 * time.Second):
			t.Fatal("timeout.")
		}
	}
	emit(true)
	cancel()
	for range ch {
	}
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
	priv, err := lp2p.LoadOrCreatePrivKey(path.Join(identityDir, "identity.key"), log.DefaultLogger)
	if err != nil {
		return nil, err
	}
	_, ps, err := lp2p.ConstructHost(
		ds,
		priv,
		"/ip4/0.0.0.0/tcp/"+test.FreePort(),
		relayMultiaddr,
		log.DefaultLogger,
	)
	if err != nil {
		return nil, err
	}
	return NewWithPubsub(ps, info, nil)
}
