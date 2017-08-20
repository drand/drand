package main

import (
	"net"
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
