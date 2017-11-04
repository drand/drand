package main

import (
	"math/rand"
	"net"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/nikkolasg/slog"
	"github.com/stretchr/testify/require"
)

func TestRouterBasic(t *testing.T) {
	n := 4
	_, routers := BatchRouters(n)
	defer CloseAllRouters(routers)

	for i, r1 := range routers {
		for _, r2 := range routers[i+1:] {
			err := r1.Send(r2.priv.Public, &DrandPacket{})
			require.NoError(t, err)
		}
	}
}

func TestRouterReconnection(T *testing.T) {
	n := 2
	privs, group := BatchIdentities(n)
	routers := make([]*Router, n)
	for i := 0; i < n-1; i++ {
		routers[i] = NewRouter(privs[i], group)
		go routers[i].Listen()
	}
	routers[n-1] = NewRouter(privs[n-1], group)
	defer CloseAllRouters(routers)
	sort.Sort(ByIndex(routers))
	// active only after a certain timeout
	oldMax := maxRetryConnect
	maxRetryConnect = 5
	defer func() { maxRetryConnect = oldMax }()
	oldTime := baseRetryTime
	baseRetryTime = 50 * time.Millisecond
	defer func() { baseRetryTime = oldTime }()
	timeout := baseRetryTime * 8 // 2^3
	listening := make(chan bool, 1)
	sent := make(chan error, 1)
	go func() {
		<-time.After(timeout)
		go routers[n-1].Listen()
		listening <- true
	}()
	go func() {
		err := routers[0].Send(privs[n-1].Public, &DrandPacket{})
		sent <- err
	}()
	maxTimeout := baseRetryTime * 32 // 2^5
	select {
	case <-listening:
		err := <-sent
		if err != nil {
			T.Fail()
		}
	case <-time.After(time.Duration(maxTimeout) * time.Millisecond):
		T.Fail()

	}
}

func TestRouterInverse(t *testing.T) {
	slog.Level = slog.LevelDebug
	n := 4
	_, routers := BatchRouters(n)
	defer CloseAllRouters(routers)

	oldValue := maxIncomingWaitTime
	maxIncomingWaitTime = 100 * time.Millisecond
	defer func() { maxIncomingWaitTime = oldValue }()

	first := routers[0]
	last := routers[n-1]
	blast := routers[n-2]

	// first is not actively sending connection
	require.Error(t, last.Send(first.priv.Public, &DrandPacket{}))
	// first connecting
	require.NoError(t, first.Send(last.priv.Public, &DrandPacket{}))
	_, _ = last.Receive()

	last.cond.L.Lock()
	_, ok := last.conns[first.priv.Public.Key.String()]
	last.cond.L.Unlock()
	require.True(t, ok)
	require.NoError(t, last.Send(first.priv.Public, &DrandPacket{}))

	// force connection
	require.Nil(t, first.SendForce(blast.priv.Public, &DrandPacket{}))
	time.Sleep(5 * time.Millisecond)
	blast.cond.L.Lock()
	_, ok = blast.conns[first.priv.Public.Key.String()]
	blast.cond.L.Unlock()
	require.True(t, ok)
}

// test sending one message to every other node from each node
func TestRouterSquare(t *testing.T) {
	old := maxIncomingWaitTime
	defer func() { maxIncomingWaitTime = old }()
	maxIncomingWaitTime = 500 * time.Millisecond
	n := 6
	_, routers := BatchRouters(n)
	message := &DrandPacket{}
	var wg sync.WaitGroup
	for i := range rand.Perm(n) {
		sender := routers[i]
		for j := range rand.Perm(n) {
			if i == j {
				continue
			}
			receiver := routers[j]
			wg.Add(1)
			//fmt.Printf("router[%d] send to router[%d]\n", sender.index, receiver.index)
			go func(s, r *Router) {
				require.Nil(t, s.Send(r.priv.Public, message))
				//fmt.Printf("router[%d] send to router[%d] SENT\n", s.index, r.index)
				_, _ = r.Receive()
				//fmt.Printf("router[%d] send to router[%d] RECEIVED\n", s.index, r.index)
				wg.Done()
			}(sender, receiver)
		}
	}
	wg.Wait()
}

func TestNetworkConn(t *testing.T) {
	addr := "127.0.0.1:6789"
	priv := NewKeyPair(addr)
	hello := &DrandPacket{Hello: priv.Public}
	g2 := pairing.G2()

	l, err := net.Listen("tcp", addr)
	require.Nil(t, err)

	conns := make(chan net.Conn)
	go func() {
		c, err := net.Dial("tcp", addr)
		require.Nil(t, err)
		conns <- c
	}()

	c1, err := l.Accept()
	require.Nil(t, err)
	c2 := <-conns

	cc1 := Conn{c1}
	cc2 := Conn{c2}

	require.Nil(t, cc1.Send(hello))
	buff, err := cc2.Receive()
	require.Nil(t, err)
	_, err = unmarshal(g2, buff)
	require.Nil(t, err)
	//require.NotNil(t, drand.Hello)
}
