package main

import (
	"fmt"
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
		Threshold: defaultThreshold(n),
		List:      toIndexedList(pubs),
	}
	return privs, group
}

func BatchDrands(n int) (*Group, []*Drand) {
	ids, group := BatchIdentities(n)
	drands := make([]*Drand, n)
	var err error
	for i := range ids {
		drands[i], err = NewDrand(ids[i], group)
		if err != nil {
			panic(err)
		}
		fmt.Printf("drand[%d] => %s\n", i, ids[i].Public.Address)
	}
	return group, drands
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

func CloseAllDrands(drands []*Drand) {
	for _, d := range drands {
		d.r.Stop()
	}
}

func CloseAllRouters(routers []*Router) {
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
