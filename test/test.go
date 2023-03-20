// Package test offers some common functionalities that are used throughout many
// different tests in drand.
package test

import (
	"encoding/hex"
	"fmt"
	n "net"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rogpeppe/go-internal/lockedfile"

	commonutils "github.com/drand/drand/common"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	"github.com/drand/kyber"
	"github.com/drand/kyber/pairing/bn256"
	"github.com/drand/kyber/util/random"
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

// NewPeer returns a new net.Peer
func NewPeer(addr string) net.Peer {
	return &testPeer{a: addr, b: false}
}

// NewTLSPeer returns a new net.Peer with TLS enabled
func NewTLSPeer(addr string) net.Peer {
	return &testPeer{a: addr, b: true}
}

// Addresses returns a list of TCP localhost addresses starting from the given
// port= start.
func Addresses(n int) []string {
	addrs := make([]string, 0, n)
	ports := Ports(n)
	for i := 0; i < n; i++ {
		addrs = append(addrs, "127.0.0.1:"+ports[i])
	}
	return addrs
}

// LocalHost returns the default local bind address: e.g. [::] or 127.0.0.1
func LocalHost() string {
	addr, err := n.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		panic(err)
	}
	return addr.IP.String()
}

// FreeBind provides an address for binding on provided address
func FreeBind(a string) string {
	globalLock.Lock()
	defer globalLock.Unlock()

	if os.Getenv("CI") == "true" {
		var fileMtxPath = path.Join(os.TempDir(), ".drand_test_ports")
		fileMtx := lockedfile.MutexAt(fileMtxPath)
		var fileMtxUnlock func()
		for {
			fileMtxUnlock, _ = fileMtx.Lock()
			if fileMtxUnlock != nil {
				break
			}
			time.Sleep(3 * time.Millisecond)
		}
		defer fileMtxUnlock()

		// First, let's update the existing ports list
		contents, err := os.ReadFile(fileMtxPath)
		if err != nil {
			panic(err)
		}
		allPorts = strings.Fields(string(contents))

		// Finally, we update the list of ports before we return
		defer func() {
			contents := strings.Join(allPorts, "\n")
			err := os.WriteFile(fileMtxPath, []byte(contents), 0600)
			if err != nil {
				panic(err)
			}
		}()
	}

	for {
		addr, err := n.ResolveTCPAddr("tcp", a+":0")
		if err != nil {
			panic(err)
		}

		l, err := n.ListenTCP("tcp", addr)
		if err != nil {
			panic(err)
		}
		p := strconv.Itoa(l.Addr().(*n.TCPAddr).Port)
		var found bool
		for _, u := range allPorts {
			if p == u {
				found = true
				break
			}
		}
		if !found {
			allPorts = append(allPorts, p)
			_ = l.Close()
			return l.Addr().String()
		}
		_ = l.Close()
	}
}

// Ports returns a list of ports starting from the given
// port= start.
// TODO: This function is flaky.
func Ports(n int) []string {
	ports := make([]string, 0, n)
	for i := 0; i < n; i++ {
		ports = append(ports, FreePort())
	}
	return ports
}

var allPorts []string
var globalLock sync.Mutex

// FreePort returns an free TCP port.
// Taken from https://github.com/phayes/freeport/blob/master/freeport.go
func FreePort() string {
	addr := FreeBind("localhost")
	_, p, _ := n.SplitHostPort(addr)
	return p
}

// GenerateIDs returns n keys with random port localhost addresses
func GenerateIDs(n int) []*key.Pair {
	keys := make([]*key.Pair, n)
	addrs := Addresses(n)
	for i := range addrs {
		priv, _ := key.NewKeyPair(addrs[i], nil)
		keys[i] = priv
	}
	return keys
}

// BatchIdentities generates n insecure identities
func BatchIdentities(n int, sch *crypto.Scheme, beaconID string) ([]*key.Pair, *key.Group) {
	beaconID = commonutils.GetCanonicalBeaconID(beaconID)
	privs := GenerateIDs(n)

	thr := key.MinimumT(n)
	var dpub []kyber.Point
	for i := 0; i < thr; i++ {
		dpub = append(dpub, sch.KeyGroup.Point().Pick(random.New()))
	}

	dp := &key.DistPublic{Coefficients: dpub}
	group := key.LoadGroup(ListFromPrivates(privs), 1, dp, 30*time.Second, 0, sch, beaconID)
	group.Threshold = thr
	return privs, group
}

// BatchTLSIdentities generates n secure (TLS) identities
func BatchTLSIdentities(n int, sch *crypto.Scheme, beaconID string) ([]*key.Pair, *key.Group) {
	pairs, group := BatchIdentities(n, sch, beaconID)
	for i := 0; i < n; i++ {
		pairs[i].Public.TLS = true
	}
	return pairs, group
}

// ListFromPrivates returns a list of Identity from a list of Pair keys.
func ListFromPrivates(keys []*key.Pair) []*key.Node {
	list := make([]*key.Node, len(keys))
	for i := range keys {
		list[i] = &key.Node{
			Index:    uint32(i),
			Identity: keys[i].Public,
		}
	}
	return list

}

// StringToPoint ...
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

// GetBeaconIDFromEnv read beacon id from an environment variable.
func GetBeaconIDFromEnv() string {
	beaconID := os.Getenv("BEACON_ID")
	return commonutils.GetCanonicalBeaconID(beaconID)
}

func SleepDuration() time.Duration {
	if os.Getenv("CI") != "" {
		fmt.Println("--- Sleeping on CI")
		return time.Duration(800) * time.Millisecond
	}
	return time.Duration(200) * time.Millisecond
}
