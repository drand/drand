// test package offers some common functionalities that are used throughout many
// different tests in drand.
package test

import (
	"encoding/hex"
	n "net"
	"strconv"

	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	"github.com/dedis/kyber"
	"github.com/dedis/kyber/pairing/bn256"
)

type testPeer struct {
	a string
	b bool
}

func (t *testPeer) Address() string {
	return t.a
}

func (t *testPeer) IsTLS() bool {
	return t.b
}

func NewPeer(addr string) net.Peer {
	return &testPeer{a: addr, b: false}
}

func NewTLSPeer(addr string) net.Peer {
	return &testPeer{a: addr, b: true}
}

// Addresses returns a list of TCP localhost addresses starting from the given
// port= start.
func Addresses(n int) []string {
	addrs := make([]string, n, n)
	for i := 0; i < n; i++ {
		addrs[i] = "127.0.0.1:" + strconv.Itoa(FreePort())
	}
	return addrs
}

// Ports returns a list of ports starting from the given
// port= start.
func Ports(n int) []string {
	ports := make([]string, n, n)
	for i := 0; i < n; i++ {
		ports[i] = strconv.Itoa(FreePort())
	}
	return ports
}

// GetFreePort returns an free TCP port.
// Taken from https://github.com/phayes/freeport/blob/master/freeport.go
func FreePort() int {
	addr, err := n.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		panic(err)
	}

	l, err := n.ListenTCP("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer l.Close()
	return l.Addr().(*n.TCPAddr).Port
}

// GenerateIDs returns n keys with random port localhost addresses
func GenerateIDs(n int) []*key.Pair {
	keys := make([]*key.Pair, n)
	addrs := Addresses(n)
	for i := range addrs {
		priv := key.NewKeyPair(addrs[i])
		keys[i] = priv
	}
	return keys
}

func BatchIdentities(n int) ([]*key.Pair, *key.Group) {
	privs := GenerateIDs(n)
	keyStr := "012067064287f0d81a03e575109478287da0183fcd8f3eda18b85042d1c8903ec8160c56eb6d5884d8c519c30bfa3bf5181f42bcd2efdbf4ba42ab0f31d13c97e9552543be1acf9912476b7da129d7c7e427fbafe69ac5b635773f488b8f46f3fc40c673b93a08a20c0e30fd84de8a89adb6fb95eca61ef2fff66527b3be4912de"
	fakeKey, _ := StringToPoint(keyStr)
	group := key.NewGroup(ListFromPrivates(privs), key.DefaultThreshold(n), &key.DistPublic{Key: fakeKey})
	return privs, group
}

func BatchTLSIdentities(n int) ([]*key.Pair, *key.Group) {
	pairs, group := BatchIdentities(n)
	for i := 0; i < n; i++ {
		pairs[i].Public.TLS = true
	}
	return pairs, group
}

// ListFromPrivates returns a list of Identity from a list of Pair keys.
func ListFromPrivates(keys []*key.Pair) []*key.Identity {
	n := len(keys)
	list := make([]*key.Identity, n, n)
	for i := range keys {
		list[i] = keys[i].Public
	}
	return list

}

func StringToPoint(s string) (kyber.Point, error) {
	pairing := bn256.NewSuite()
	g := pairing.G2()
	buff, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	p := g.Point()
	return p, p.UnmarshalBinary(buff)
}
