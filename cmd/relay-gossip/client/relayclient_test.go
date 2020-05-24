package client

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/drand/drand/cmd/drand-gossip-relay/lp2p"
	"github.com/drand/drand/cmd/relay-gossip/node"
	"github.com/drand/drand/test"
	"github.com/drand/drand/test/mock"
	bds "github.com/ipfs/go-ds-badger2"
	ma "github.com/multiformats/go-multiaddr"
)

func TestClient(t *testing.T) {
	// start mock drand node
	grpcLis, grpcSvc := mock.NewMockGRPCPublicServer(":0", false)
	grpcAddr := grpcLis.Addr()
	go grpcLis.Start()
	defer grpcLis.Stop(context.Background())
	_ = grpcSvc

	// start mock relay-node
	cfg := &node.GossipRelayConfig{
		Network:         "test",
		PeerWith:        nil,
		Addr:            "/ip4/0.0.0.0/tcp/" + test.FreePort(),
		DataDir:         path.Join(os.TempDir(), "test-gossip-relay-node-datastore"),
		IdentityPath:    path.Join(os.TempDir(), "test-gossip-relay-node-id"),
		CertPath:        "",
		Insecure:        true,
		DrandPublicGRPC: grpcAddr,
	}
	g, err := node.NewGossipRelayNode(cfg)
	if err != nil {
		t.Fatalf("gossip relay node (%v)", err)
	}
	defer g.Shutdown()

	// start client
	c, err := newTestClient("test-gossip-relay-client", g.Addr(), "test")
	if err != nil {
		t.Fatal(err)
	}

	// test client
	ctx, cancel := context.WithCancel(context.Background())
	ch := c.Watch(ctx)
	for i := 0; i < 3; i++ {
		fmt.Print(<-ch)
	}
	cancel()
	for range ch {
	}
}

func newTestClient(name string, relayMultiaddr []ma.Multiaddr, network string) (*Client, error) {
	ds, err := bds.NewDatastore(path.Join(os.TempDir(), "client-"+name+"-datastore"), nil)
	if err != nil {
		return nil, err
	}
	priv, err := lp2p.LoadOrCreatePrivKey(path.Join(os.TempDir(), "client-"+name+"-id"))
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
	return NewWithPubsub(ps, network)
}
