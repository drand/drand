package client

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path"
	"testing"
	"time"

	bds "github.com/ipfs/go-ds-badger2"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

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
	info.GenesisTime -= 10

	// start mock relay-node
	grpcClient, err := grpc.New(grpcAddr, "", true, []byte(""))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &lp2p.GossipRelayConfig{
		ChainHash:    hex.EncodeToString(info.Hash()),
		PeerWith:     nil,
		Addr:         "/ip4/127.0.0.1/tcp/" + test.FreePort(),
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
	service := svc.(mock.MockService)
	// for the initial 'get' to sync the chain
	ch := c.Watch(ctx)
	service.EmitRand(t, false)
	r, ok := <-ch
	require.True(t, ok, "expected randomness")
	require.NotNil(t, r)
	t.Logf("received message %#v on ch\n", r)

	for i := 0; i < 3; i++ {
		service.EmitRand(t, false)
		t.Logf("round %d. emitting.\n", i)

		select {
		case r, ok := <-ch:
			if !ok {
				t.Fatal("expected randomness")
			}

			t.Logf("%#v\n", r)
		case <-time.After(10 * time.Second):
			t.Fatal("timeout.")
		}
	}
	t.Log("leaving the main test loop")
	service.EmitRand(t, true)
	cancel()
	drain(t, ch, 10*time.Second)
}

func drain(t *testing.T, ch <-chan client.Result, timeout time.Duration) {
	t.Log("draining ch")

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
	sch := scheme.GetSchemeFromEnv()

	addr, chainInfo, stop, emit := httpmock.NewMockHTTPPublicServer(t, false, sch)
	defer stop()

	dataDir := t.TempDir()
	identityDir := t.TempDir()

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

	c, err := newTestClient(t, g.Multiaddrs(), chainInfo)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	emit(t, false)
	ch := c.Watch(ctx)
	time.Sleep(5 * time.Millisecond)
	for i := 0; i < 3; i++ {
		emit(t, false)
		select {
		case r, ok := <-ch:
			if !ok {
				t.Fatal("expected randomness")
			} else {
				t.Logf("%#v\n", r)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("timeout.")
		}
	}
	emit(t, true)
	cancel()
	drain(t, ch, 10*time.Second)
}

func newTestClient(t *testing.T, relayMultiaddr []ma.Multiaddr, info *chain.Info) (*Client, error) {
	dataDir := t.TempDir()
	identityDir := t.TempDir()
	ds, err := bds.NewDatastore(dataDir, nil)
	if err != nil {
		return nil, err
	}

	logLevel := log.LogInfo
	debugEnv, isDebug := os.LookupEnv("DRAND_TEST_LOGS")
	if isDebug && debugEnv == "DEBUG" {
		t.Log("Enabling LogDebug logs")
		logLevel = log.LogDebug
	}
	logger := log.NewJSONLogger(zapcore.AddSync(os.Stdout), logLevel)

	priv, err := lp2p.LoadOrCreatePrivKey(path.Join(identityDir, "identity.key"), logger)
	if err != nil {
		return nil, err
	}
	h, ps, err := lp2p.ConstructHost(
		ds,
		priv,
		"/ip4/0.0.0.0/tcp/"+test.FreePort(),
		relayMultiaddr,
		logger,
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
	c, err := NewWithPubsub(ps, info, &client.NilCache{})
	if err != nil {
		return nil, err
	}
	c.SetLog(logger)
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
