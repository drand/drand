package lp2p

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	madns "github.com/multiformats/go-multiaddr-dns"
)

const (
	peer0       = "12D3KooW9rwsuYZdWMZfu4hsog3rcDH9okeB9ayAWGzESLwpca78"
	peer1       = "12D3KooW9uKWKaPxUSxJySsu2YB3PxaFaXYu8KSZuvLfQxYPu8jj"
	dnsaddr0    = "/dnsaddr/example0.com"
	dnsaddr1    = "/dnsaddr/example1.com"
	p2pIP4Addr0 = "/ip4/192.168.0.1/tcp/44544/p2p/" + peer0
	p2pIP6Addr0 = "/ip6/2001:db8::a3/tcp/44544/p2p/" + peer0
	p2pIP4Addr1 = "/ip4/10.10.10.10/tcp/44544/p2p/" + peer1
	notP2PAddr1 = "/ip4/10.10.10.10/tcp/80"
)

func mockResolver(txtRecords map[string][]string) *madns.Resolver {
	mock := &madns.MockBackend{
		IP:  map[string][]net.IPAddr{},
		TXT: txtRecords,
	}
	return &madns.Resolver{Backend: mock}
}

func findPeer(t *testing.T, ais []peer.AddrInfo, peerIDStr string) peer.AddrInfo {
	t.Helper()
	peerID, err := peer.Decode(peerIDStr)
	if err != nil {
		t.Fatal(err)
	}
	for _, ai := range ais {
		if ai.ID == peerID {
			return ai
		}
	}
	t.Fatal("not found", peerID)
	return peer.AddrInfo{}
}

func TestResolveDNS(t *testing.T) {
	addrs := []multiaddr.Multiaddr{
		multiaddr.StringCast(dnsaddr0),
		multiaddr.StringCast(dnsaddr1),
	}
	txtRecords := map[string][]string{
		"_dnsaddr.example0.com": {"dnsaddr=" + p2pIP4Addr0, "dnsaddr=" + p2pIP6Addr0},
		"_dnsaddr.example1.com": {"dnsaddr=" + p2pIP4Addr1, "dnsaddr=" + notP2PAddr1},
	}
	ais, err := resolveAddresses(context.Background(), addrs, mockResolver(txtRecords))
	if err != nil {
		t.Fatal(err)
	}
	if len(ais) != 2 {
		t.Fatal("expected 2 peers", len(ais))
	}
	peer0Info := findPeer(t, ais, peer0)
	if len(peer0Info.Addrs) != 2 {
		t.Fatal("expected 2 addrs for peer", peer0, peer0Info.Addrs)
	}
	peer1Info := findPeer(t, ais, peer1)
	if len(peer1Info.Addrs) != 1 {
		t.Fatal("expected 1 addr for peer", peer1, peer1Info.Addrs)
	}
}

func TestResolveDNSNoAddrs(t *testing.T) {
	addrs := []multiaddr.Multiaddr{multiaddr.StringCast(dnsaddr0)}
	txtRecords := map[string][]string{"_dnsaddr.example0.com": {}}
	_, err := resolveAddresses(context.Background(), addrs, mockResolver(txtRecords))
	if !strings.HasPrefix(err.Error(), "found no ipfs peers at") {
		t.Fatal("unexpected error", err)
	}
}

type failBackend struct{}

func (fb *failBackend) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return nil, errors.New("failBackend")
}
func (fb *failBackend) LookupTXT(context.Context, string) ([]string, error) {
	return nil, errors.New("failBackend")
}

func TestResolveDNSFailure(t *testing.T) {
	addrs := []multiaddr.Multiaddr{multiaddr.StringCast(dnsaddr0)}
	_, err := resolveAddresses(context.Background(), addrs, &madns.Resolver{Backend: &failBackend{}})
	if !strings.Contains(err.Error(), "failBackend") {
		t.Fatal("unexpected error", err)
	}
}
