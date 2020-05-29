package client

import (
	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/drand/drand/chain"
	cmock "github.com/drand/drand/client/test/mock"
	"github.com/drand/drand/cmd/relay-gossip/lp2p"
	"github.com/drand/drand/cmd/relay-gossip/node"
	dlog "github.com/drand/drand/log"
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

	// start mock relay-node
	cfg := &node.GossipRelayConfig{
		ChainHash:       hex.EncodeToString(info.Hash()),
		PeerWith:        nil,
		Addr:            "/ip4/0.0.0.0/tcp/" + test.FreePort(),
		DataDir:         dataDir,
		IdentityPath:    path.Join(identityDir, "identity.key"),
		CertPath:        "",
		Insecure:        true,
		DrandPublicGRPC: grpcAddr,
	}
	g, err := node.NewGossipRelayNode(dlog.DefaultLogger, cfg)
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
	ch := c.Watch(ctx)
	for i := 0; i < 3; i++ {
		if _, ok := <-ch; !ok {
			t.Fatal("expected randomness")
		}
		fmt.Print(<-ch)
	}
	cancel()
	for range ch {
	}
}

func TestHTTPClient(t *testing.T) {
	addr, chainInfo, stop := cmock.NewMockHTTPPublicServer(t, false)
	defer stop()

	dataDir, err := ioutil.TempDir(os.TempDir(), "test-gossip-relay-node-datastore")
	if err != nil {
		t.Fatal(err)
	}
	identityDir, err := ioutil.TempDir(os.TempDir(), "test-gossip-relay-node-id")
	if err != nil {
		t.Fatal(err)
	}

	cfg := &node.GossipRelayConfig{
		ChainHash:       hex.EncodeToString(chainInfo.Hash()),
		PeerWith:        nil,
		Addr:            "/ip4/0.0.0.0/tcp/" + test.FreePort(),
		DataDir:         dataDir,
		IdentityPath:    path.Join(identityDir, "identity.key"),
		DrandPublicHTTP: []string{"http://" + addr},
	}
	g, err := node.NewGossipRelayNode(dlog.DefaultLogger, cfg)
	if err != nil {
		t.Fatalf("gossip relay node (%v)", err)
	}
	defer g.Shutdown()

	c, err := newTestClient("test-http-gossip-relay-client", g.Multiaddrs(), chainInfo)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch := c.Watch(ctx)
	for i := 0; i < 3; i++ {
		r, ok := <-ch
		if !ok {
			t.Fatal("expected randomness")
		}
		fmt.Printf("%+v\n", r)
	}
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
	priv, err := lp2p.LoadOrCreatePrivKey(path.Join(identityDir, "identity.key"))
	if err != nil {
		return nil, err
	}
	_, ps, err := lp2p.ConstructHost(
		ds,
		priv,
		"/ip4/0.0.0.0/tcp/"+test.FreePort(),
		relayMultiaddr,
	)
	if err != nil {
		return nil, err
	}
	return NewWithPubsub(ps, info, nil)
}
