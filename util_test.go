package main

import (
	"sort"
	"strconv"
	"time"
)

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
		T:    defaultThreshold(n),
		List: toIndexedList(pubs),
	}
	return privs, group
}

func BatchRouters(n int) ([]*Private, []*Router) {
	privs, group := BatchIdentities(n)
	routers := make([]*Router, n)
	for i := 0; i < n; i++ {
		routers[i] = NewRouter(privs[i], group)
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
