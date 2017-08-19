package main

import (
	"sort"
	"strconv"
	"time"
)

func BatchIdentities(n int) ([]*Private, IndexedList) {
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
	return privs, Sort(pubs)
}

func BatchRouters(n int) ([]*Private, []*Router) {
	privs, list := BatchIdentities(n)
	g := pairing.G2()
	routers := make([]*Router, n)
	for i := 0; i < n; i++ {
		routers[i] = NewRouter(privs[i], list, g)
		go routers[i].Listen()
	}
	sort.Sort(ByIndex(routers))
	time.Sleep(10 * time.Millisecond)
	return privs, routers
}

func CloseAll(routers []*Router) {
	for _, r := range routers {
		r.Stop()
	}
}

type ByIndex []*Router

func (b ByIndex) Len() int {
	return len(b)
}

func (b ByIndex) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b ByIndex) Less(i, j int) bool {
	return b[i].index < b[j].index
}
