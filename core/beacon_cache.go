package core

import (
	"sync"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/log"
)

// XXX move to a LRU cache probably better but works
// for now
type beaconCache struct {
	sync.Mutex
	backend []*beacon.Beacon
	max     int
	head    int
	l       log.Logger
}

func newBeaconCache(l log.Logger) *beaconCache {
	max := DefaultBeaconCacheLength
	return &beaconCache{
		backend: make([]*beacon.Beacon, max),
		max:     max,
		l:       l,
	}
}

func (b *beaconCache) StoreTemp(r *beacon.Beacon) {
	b.Lock()
	defer b.Unlock()
	b.backend[b.head] = r
	b.head++
	if b.head == b.max {
		b.head = 0
	}
	b.l.Debug("cache_store", r.Round, "head", b.head)
}

func (b *beaconCache) GetBeacon(r uint64) (*beacon.Beacon, bool) {
	if r == 0 {
		return b.GetLast()
	}
	b.Lock()
	defer b.Unlock()
	// TODO improve by first checking from head
	for _, b := range b.backend {
		if b == nil {
			continue
		}
		if b.Round == r {
			return b, true
		}
	}
	return nil, false
}

func (b *beaconCache) GetLast() (*beacon.Beacon, bool) {
	b.Lock()
	defer b.Unlock()
	r := b.backend[b.head]
	if r == nil {
		return nil, false
	}
	return r, true
}
