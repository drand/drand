package lp2p

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/multiformats/go-multiaddr"
)

func isConnected(h0, h1 host.Host) bool {
	return len(h0.Network().ConnsToPeer(h1.ID())) > 0
}

func waitConnect(ctx context.Context, h0, h1 host.Host, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if isConnected(h0, h1) {
			return nil
		}
		time.Sleep(time.Millisecond * 100)
	}
}

func TestSwarmBind(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h0, err := libp2p.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer h0.Close()

	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	h1, err := libp2p.New(ctx, libp2p.Identity(priv))
	if err != nil {
		t.Fatal(err)
	}

	if isConnected(h0, h1) {
		t.Fatal("peers should not be connected yet")
	}

	h1Addrs := h1.Addrs()
	h1P2PAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("%v/p2p/%v", h1Addrs[0], h1.ID()))

	SwarmBind(ctx, h0, []multiaddr.Multiaddr{h1P2PAddr}, time.Second)

	err = waitConnect(ctx, h0, h1, time.Second*10)
	if err != nil {
		t.Fatal(err)
	}

	h1.Close() // get disconnected
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second * 2)

	// start up again
	h1, err = libp2p.New(ctx, libp2p.Identity(priv), libp2p.ListenAddrs(h1Addrs...))
	if err != nil {
		t.Fatal(err)
	}

	err = waitConnect(ctx, h0, h1, time.Second*10)
	if err != nil {
		t.Fatal(err)
	}
}
