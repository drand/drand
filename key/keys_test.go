package main

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeysGroupPoint(t *testing.T) {
	n := 5
	_, group := BatchIdentities(n)
	points := group.Points()
	for i, p := range points {
		k := group.Public(i).Key
		require.Equal(t, p.String(), k.String())
	}
}

func BatchIdentities(n int) ([]*Private, *Group) {
	startPort := 8000
	startAddr := "127.0.0.1:"
	privs := make([]*Private, n)
	pubs := make([]*Public, n)
	for i := 0; i < n; i++ {
		port := strconv.Itoa(startPort + i)
		addr := startAddr + port
		privs[i] = NewKeyPair(addr)
		pubs[i] = privs[i].Public
	}
	group := &Group{
		Threshold: defaultThreshold(n),
		Nodes:     toIndexedList(pubs),
	}
	return privs, group
}

// default threshold for the distributed key generation protocol & TBLS.
func defaultThreshold(n int) int {
	return n * 2 / 3
}
