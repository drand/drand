package beacon

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
)

// chainStore is a Store that deals with reconstructing the beacons, sync when
// needed and arranges the head
type chainStore struct {
	chain.Store
	l             log.Logger
	client        net.ProtocolClient
	safe          *cryptoSafe
	ticker        *ticker
	done          chan bool
	newPartials   chan partialInfo
	newBeaconCh   chan *chain.Beacon
	lastInserted  chan *chain.Beacon
	requestSync   chan likeBeacon
	nonSyncBeacon chan *chain.Beacon
}

func newChainStore(l log.Logger, client net.ProtocolClient, safe *cryptoSafe, s chain.Store, ticker *ticker) *chainStore {
	chain := &chainStore{
		l:             l,
		client:        client,
		safe:          safe,
		Store:         s,
		done:          make(chan bool, 1),
		ticker:        ticker,
		newPartials:   make(chan partialInfo, 10),
		newBeaconCh:   make(chan *chain.Beacon, 100),
		requestSync:   make(chan likeBeacon, 10),
		lastInserted:  make(chan *chain.Beacon, 1),
		nonSyncBeacon: make(chan *chain.Beacon, 1),
	}
	// TODO maybe look if it's worth having multiple workers there
	go chain.runChainLoop()
	go chain.runAggregator()
	return chain
}

func (c *chainStore) NewValidPartial(addr string, p *drand.PartialBeaconPacket) {
	c.newPartials <- partialInfo{
		addr: addr,
		p:    p,
	}
}

func (c *chainStore) NewBeacon(addr string, proto *drand.BeaconPacket) {
	c.newBeaconCh <- protoToBeacon(proto)
}

func (c *chainStore) Stop() {
	c.Store.Close()
	close(c.done)
}

// we store partials that are up to this amount of rounds more than the last
// beacon we have - it is useful to store partials that may come in advance,
// especially in case of a quick catchup.
var partialCacheStoreLimit = 3

// runAggregator runs a continuous loop that tries to aggregate partial
// signatures when it can
func (c *chainStore) runAggregator() {
	lastBeacon, err := c.Store.Last()
	if err != nil {
		c.l.Fatal("chain_aggregator", "loading", "last_beacon", err)
	}
	var caches = []*roundCache{
		newRoundCache(lastBeacon.Round+1, lastBeacon.Signature),
	}
	for {
		select {
		case <-c.done:
			return
		case lastBeacon = <-c.lastInserted:
			// filter all caches inferior to this beacon
			var newCaches []*roundCache
			for _, cache := range caches {
				if cache.round <= lastBeacon.Round {
					continue
				}
				newCaches = append(newCaches, cache)
			}
			caches = newCaches
			break
		case partial := <-c.newPartials:
			// look if we have info for this round first
			pRound := partial.p.GetRound()
			ginfo, err := c.safe.GetInfo(pRound)
			if err != nil {
				c.l.Error("chain_aggregator", "partial", "no_info_for", partial.p.GetRound())
				break
			}
			// look if we want to store ths partial anyway
			shouldStore := pRound >= lastBeacon.Round+1 && pRound <= lastBeacon.Round+uint64(partialCacheStoreLimit+1)
			// check if we can reconstruct
			if !shouldStore {
				c.l.Debug("ignoring_partial", partial.p.GetRound(), "last_beacon_stored", lastBeacon.Round)
				break
			}
			thr := ginfo.group.Threshold
			c.l.Debug("store_partial", partial.addr, "round", cache.round, "len_partials", fmt.Sprintf("%d/%d", cache.Len(), thr))
			if cache.Len() < thr {
				break
			}

			pub := ginfo.pub
			n := ginfo.group.Len()
			msg := cache.Msg()
			finalSig, err := key.Scheme.Recover(pub, msg, cache.Partials(), thr, n)
			if err != nil {
				c.l.Debug("invalid_recovery", err, "round", pRound, "got", fmt.Sprintf("%d/%d", cache.Len(), n))
				break
			}
			if err := key.Scheme.VerifyRecovered(pub.Commit(), msg, finalSig); err != nil {
				c.l.Error("invalid_sig", err, "round", pRound)
				break
			}
			cache.done = true
			newBeacon := &chain.Beacon{
				Round:       cache.round,
				PreviousSig: cache.previousSig,
				Signature:   finalSig,
			}
			c.l.Info("aggregated_beacon", newBeacon.Round)
			c.newBeaconCh <- newBeacon
			break
		}
	}
}

func (c *chainStore) runChainLoop() {
	var syncing bool
	var syncingDone = make(chan bool, 1)
	lastBeacon, err := c.Store.Last()
	if err != nil {
		c.l.Fatal("store_last_init", err)
	}
	insert := func(newB *chain.Beacon) {
		if err := c.Store.Put(newB); err != nil {
			c.l.Fatal("new_beacon_storing", err)
		}
		lastBeacon = newB
		c.l.Info("NEW_BEACON_STORED", newB.String())
		c.lastInserted <- newB
		if !syncing {
			// during syncing we don't do a fast sync
			select {
			// only send if it's not full already
			case c.nonSyncBeacon <- newB:
			default:
			}
		}
	}
	for {
		select {
		case newBeacon := <-c.newBeaconCh:
			if isAppendable(lastBeacon, newBeacon) {
				insert(newBeacon)
				break
			}
			// XXX store them for lfutur usage if it's a later round than what
			// we have
			c.l.Debug("new_aggregated", "not_appendable", "last", lastBeacon.String(), "new", newBeacon.String())
			if c.shouldSync(lastBeacon, newBeacon) {
				c.requestSync <- newBeacon
			}
		case seen := <-c.requestSync:
			if !c.shouldSync(lastBeacon, seen) || syncing {
				continue
			}
			syncing = true
			go func() {
				// XXX Could do something smarter with context and cancellation
				// if we got to the right round
				c.RunSync(context.Background())
				syncingDone <- true
			}()
		case <-syncingDone:
			syncing = false
		case <-c.done:
			return
		}
	}
}

func isAppendable(lastBeacon, newBeacon *chain.Beacon) bool {
	return newBeacon.Round == lastBeacon.Round+1 &&
		bytes.Equal(lastBeacon.Signature, newBeacon.PreviousSig)
}

type likeBeacon interface {
	GetRound() uint64
}

func (c *chainStore) shouldSync(last *chain.Beacon, newB likeBeacon) bool {
	// we should sync if we are two blocks late
	return newB.GetRound() > last.GetRound()+1
}

// RunSync is a blocking call that tries to sync chain to the highest height
// found
func (c *chainStore) RunSync(ctx context.Context) {
	l, err := c.Store.Last()
	if err != nil {
		c.l.Error("run_sync", "load", "last_beacon", err)
		return
	}
	currRound := c.ticker.CurrentRound()
	outCh, err := syncChain(ctx, c.l, c.safe, l, currRound, c.client)
	if err != nil {
		c.l.Error("error_sync", err)
		return
	}
	for newB := range outCh {
		c.newBeaconCh <- newB
	}
	return
}

func (c *chainStore) AppendedBeaconNoSync() chan *chain.Beacon {
	return c.nonSyncBeacon
}

type partialInfo struct {
	addr string
	p    *drand.PartialBeaconPacket
}

type beaconInfo struct {
	addr   string
	beacon *chain.Beacon
}

// partialCache is a cache that stores (or not) all the partials the node
// receives.
// The partialCache contains some logic to prevent a DDOS attack on the partial
// signatures cache. Namely, it makes sure that there is a limited number of
// partial signatures from the same index stored at any given time.
type partialCache struct {
	threshold int
	rounds    map[string]*roundCache
	rcvd      map[int][]string
	l         log.Logger
}

func newPartialCache(l log.Logger, threshold int) *partialCache {
	return &partialCache{
		threshold: threshold,
		rounds:    make(map[string]*roundCache),
		rcvd:      make(map[int][]string),
		l:         l,
	}
}

func idFromPartial(p *drand.PartialBeaconPacket) string {
	var buff bytes.Buffer
	binary.Write(&buff, binary.BigEndian, p.GetRound())
	buff.Write(p.GetPartialSig())
	return buff.String()
}

func (c *partialCache) append(p *drand.PartialBeaconPacket) {
	id = idFromPartial(p)
	cache, ok := c.rounds[id]
	if !ok {
		cache = newRoundCache(partial.p.GetRound(), partial.p.GetPreviousSig())
		caches = append(caches, cache)
		if !cache.tryAppend(partial.p) {
			c.l.Fatal("chain-aggregator", "bug_cache_partial")
		}
	} else if cache.done {
		c.l.Debug("store_partial", "ignored", "round", cache.round, "already_reconstructed")
		return
	}
}

// newRoundCache creates a new round cache given p. If the signer of the partial
// already has more than `
func (c *partialCache) newRoundCache(id string, p *drand.PartialBeaconPacket) {
	idx, _ := key.Scheme.IndexOf(p.GetPartialSig())
	if len(c.rcvd[idx]) > MaxPartialsPerNode {
		// this node has submitted too many partials - we take the last one off
		toEvict := c.rcvd[idx][0]
		rounds, ok := c.rounds[toEvict]
		if !ok {
			c.l.Error("cache", "miss", "node", idx, "not_present_for", p.GetRound())
			// something's off
		}
		c.rcvd[idx] = append(c.rdvc[1:], id)
	}
}

type roundCache struct {
	round       uint64
	previous    uint64
	previousSig []byte
	sigs        map[int][]byte
	indexes     []int
	done        bool
}

func newRoundCache(round uint64, prevSig []byte) *cacheEntry {
	return &cacheEntry{
		round:       round,
		previousSig: prevSig,
		sigs:        make(map[int][]byte),
	}
}

func (cache *cacheEntry) tryAppend(p *drand.PartialBeaconPacket) bool {
	round := p.GetRound()
	prevSig := p.GetPreviousSig()
	idx, _ := key.Scheme.IndexOf(p.GetPartialSig())
	if _, seen := cache.sigs[idx]; seen {
		return true
	}

	sameRound := round == cache.round
	samePrevS := bytes.Equal(prevSig, cache.previousSig)
	if sameRound && samePrevS {
		cache.sigs[idx] = p.GetPartialSig()
		indexes = append(indexes, idx)
		return true
	}
	return false
}

// Len shows how many items are in the cache
func (cache *cacheEntry) Len() int {
	return len(cache.sigs)
}

// Msg provides the chain for the current round
func (cache *cacheEntry) Msg() []byte {
	return chain.Message(cache.round, cache.previousSig)
}

// Partials provides all cached partial signatures
func (cache *cacheEntry) Partials() [][]byte {
	partials := make([][]byte, 0, len(cache.sigs))
	for _, sig := range cache.sigs {
		partials = append(partials, sig)
	}
	return partials
}
