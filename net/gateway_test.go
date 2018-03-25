package net

import (
	"crypto/rand"
	"strconv"
	"testing"
	"time"

	"github.com/nikkolasg/dsign/key"
	"github.com/nikkolasg/dsign/net/transport/noise"
	"github.com/stretchr/testify/require"
)

func TestGateway(t *testing.T) {
	priv1, pub1 := FakeID("127.0.0.1:8000")
	priv2, pub2 := FakeID("127.0.0.1:8001")
	list := []*key.Identity{pub1, pub2}

	tr1 := noise.NewTCPNoiseTransport(priv1, list)
	g1 := NewGateway(priv1.Public, tr1)
	tr2 := noise.NewTCPNoiseTransport(priv2, list)
	g2 := NewGateway(priv2.Public, tr2)

	listenDone := make(chan bool)
	rcvDone := make(chan bool)
	sentDone := make(chan bool, 1)
	handler2 := func(from *key.Identity, msg []byte) {
		<-sentDone
		require.Nil(t, g2.Send(pub1, msg))
		listenDone <- true
	}
	handler1 := func(from *key.Identity, msg []byte) {
		require.Equal(t, []byte{0x2a}, msg)
		rcvDone <- true
	}

	require.Nil(t, g2.Start(handler2))
	require.Nil(t, g1.Start(handler1))

	time.Sleep(10 * time.Millisecond)
	msg := []byte{0x2a}
	err := g1.Send(pub2, msg)
	require.NoError(t, err)
	sentDone <- true

	select {
	case <-listenDone:
		break
	case <-time.After(20 * time.Millisecond):
		t.Fatal("g2 not closing listening...")
	}
	select {
	case <-rcvDone:
		break
	case <-time.After(20 * time.Millisecond):
		t.Fatal("g1 not receiving anything")
	}
	require.Nil(t, g1.Stop())
	require.Nil(t, g2.Stop())
}

func TestGatewayBroadcast(t *testing.T) {
	n := 4
	privs, gws := Gateways(n)
	list := ListFromPrivates(privs)
	root := privs[0].Public
	expected := n * (n - 1)
	//fmt.Println("expected => ", expected)
	rcvChan := make(chan bool)
	handlers := make([]Processor, n, n)

	// close all after
	defer func() {
		for i := range gws {
			require.NoError(t, gws[i].Stop())
		}
	}()

	for i := range handlers {
		handlers[i] = func(gw Gateway) Processor {
			return func(from *key.Identity, msg []byte) {
				rcvChan <- true
				if from.Equals(root) {
					require.NoError(t, gw.Broadcast(list, msg))
				}
			}
		}(gws[i])
		gws[i].Start(handlers[i])
		time.Sleep(5 * time.Millisecond)
	}

	//fmt.Println("broadcast start")
	require.NoError(t, gws[0].Broadcast(list, []byte("Hello World")))
	//fmt.Println("broadcast done")
	var received int
	for {
		select {
		case <-rcvChan:
			received++
			//fmt.Println("received => ", received)
			if received == expected {
				return
			}
		}
	}
}

// Gateways returns n test Gateway using encrypted noise communication
func Gateways(n int) ([]*key.Private, []Gateway) {
	keys := GenerateIDs(8000, n)
	gws := make([]Gateway, n, n)
	list := ListFromPrivates(keys)
	for i := range keys {
		noiseTr := noise.NewTCPNoiseTransport(keys[i], list)
		gws[i] = NewGateway(keys[i].Public, noiseTr)
	}
	return keys, gws
}

// FakeID returns a random ID with the given address.
func FakeID(addr string) (*key.Private, *key.Identity) {
	priv, id, err := key.NewPrivateIdentityWithAddr(addr, rand.Reader)
	if err != nil {
		panic(err)
	}
	return priv, id
}

// Addresses returns a list of TCP localhost addresses starting from the given
// port= start.
func Addresses(start, n int) []string {
	addrs := make([]string, n, n)
	for i := 0; i < n; i++ {
		addrs[i] = "127.0.0.1:" + strconv.Itoa(start+i)
	}
	return addrs
}

// GenerateIDs returns n private keys with the start address given to Addresses
func GenerateIDs(start, n int) []*key.Private {
	keys := make([]*key.Private, n)
	addrs := Addresses(start, n)
	for i := range addrs {
		priv, _ := FakeID(addrs[i])
		keys[i] = priv
	}
	return keys
}

// ListFromPrivates returns a list of Identity from a list of Private keys.
func ListFromPrivates(keys []*key.Private) []*key.Identity {
	n := len(keys)
	list := make([]*key.Identity, n, n)
	for i := range keys {
		list[i] = keys[i].Public
	}
	return list

}
