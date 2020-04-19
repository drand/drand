package beacon

import (
	"bytes"
	"context"
	"fmt"

	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
)

// chainStore is a Store that deals with reconstructing the beacons, sync when
// needed and arranges the head
type chainStore struct {
	Store
	l                log.Logger
	client           net.ProtocolClient
	safe             *cryptoSafe
	ticker           *ticker
	done             chan bool
	newPartials      chan partialInfo
	newAggregated    chan *Beacon
	newBeaconNetwork chan *Beacon
	newBeaconSync    chan *Beacon
	lastInserted     chan *Beacon
}

func newChainStore(l log.Logger, client net.ProtocolClient, safe *cryptoSafe, s Store, ticker *ticker) *chainStore {
	chain := &chainStore{
		l:                l,
		client:           client,
		safe:             safe,
		Store:            s,
		done:             make(chan bool, 1),
		ticker:           ticker,
		newPartials:      make(chan partialInfo, 10),
		newAggregated:    make(chan *Beacon, 1),
		newBeaconNetwork: make(chan *Beacon, 100),
		lastInserted:     make(chan *Beacon, 1),
		newBeaconSync:    make(chan *Beacon, 100),
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
	c.newBeaconNetwork <- protoToBeacon(proto)
}

func (c *chainStore) Stop() {
	c.Store.Close()
	close(c.done)
}

// runAggregator runs a continuous loop that tries to aggregate partial
// signatures when it can
func (c *chainStore) runAggregator() {
	var caches []*roundCache
	newRound := c.ticker.Channel()
	lastBeacon, _ := c.Store.Last()
	var currRound = roundInfo{
		round: c.ticker.CurrentRound(),
	}
	for {
		select {
		case <-c.done:
			return
		case lastBeacon = <-c.lastInserted:
			break
		case currRound = <-newRound:
			// remove all caches that are previous to this round
			var filtered []*roundCache
			for _, cache := range caches {
				if cache.round < currRound.round {
					continue
				}
				filtered = append(filtered, cache)
			}
			c.l.Debug("tick_new_round", currRound.round, "filtered_cache", fmt.Sprintf("%d/%d", len(filtered), len(caches)))
			caches = filtered
		case partial := <-c.newPartials:
			var cache *roundCache
			for _, c := range caches {
				if !c.tryAppend(partial.p) {
					continue
				}
				cache = c
			}

			ginfo, err := c.safe.GetInfo(partial.p.GetRound())
			if err != nil {
				c.l.Error("no_info_for", partial.p.GetRound())
				continue
			}
			// +1 is because depending on clock skew, this node may not have
			// passed yet to the new round for which he receives a partial.
			shouldStore := partial.p.GetRound() == currRound.round || partial.p.GetRound() == currRound.round+1
			if !shouldStore {
				c.l.Error("ignoring_partial", partial.p.GetRound(), "current_round", currRound.round)
				continue
			}
			if cache == nil {
				cache = newRoundCache(partial.p.GetRound(), partial.p.GetPreviousRound(), partial.p.GetPreviousSig())
				caches = append(caches, cache)
				if !cache.tryAppend(partial.p) {
					c.l.Fatal("bug_cache_partial")
				}
			} else if cache.done {
				c.l.Debug("store_partial", "ignored", "round", cache.round, "already_reconstructed")
				break
			}
			thr := ginfo.group.Threshold
			c.l.Debug("store_partial", partial.addr, "round", cache.round, "len_partials", fmt.Sprintf("%d/%d", cache.Len(), thr), "prev_round", partial.p.GetPreviousRound())

			// check if we can reconstruct
			if cache.Len() < thr {
				// check if it doesn't correspond to what we want, we may want to
				// sync. 2 because we dont want to sync as soon as we get one,
				// it may be a random one - aritrarily chosen XXX put more
				// thoughts into that.
				if lastBeacon != nil && cache.Len() >= 2 {
					c.maybeRunSync(currRound, lastBeacon, partial.p)
				}
				break
			}
			pub := ginfo.pub
			n := ginfo.group.Len()
			msg := cache.Msg()
			finalSig, err := key.Scheme.Recover(pub, msg, cache.Partials(), thr, n)
			if err != nil {
				c.l.Debug("invalid_recovery", err, "round", partial.p.GetRound(), "got", fmt.Sprintf("%d/%d", cache.Len(), n))
				break
			}
			if err := key.Scheme.VerifyRecovered(pub.Commit(), msg, finalSig); err != nil {
				c.l.Error("invalid_sig", err, "round", partial.p.GetRound(), "prev", partial.p.GetPreviousRound())
				return
			}
			cache.done = true
			newBeacon := &Beacon{
				Round:         cache.round,
				PreviousRound: cache.previous,
				PreviousSig:   cache.previousSig,
				Signature:     finalSig,
			}
			c.l.Info("aggregated_beacon", newBeacon.Round, "previous_round", newBeacon.PreviousRound)
			c.newAggregated <- newBeacon
			break
		}
	}
}

func (c *chainStore) runChainLoop() {
	lastBeacon, err := c.Store.Last()
	if err != nil {
		c.l.Fatal("store_last_init", err)
	}
	newRound := c.ticker.Channel()
	var currRound = roundInfo{
		round: c.ticker.CurrentRound(),
	}
	insert := func(newB *Beacon) {
		if err := c.Store.Put(newB); err != nil {
			c.l.Fatal("new_beacon_storing", err)
		}
		lastBeacon = newB
		c.lastInserted <- newB
		c.l.Info("NEWBEACON_STORE", newB.String())
	}
	for {
		select {
		case newBeacon := <-c.newAggregated:
			if c.isReorg(lastBeacon, newBeacon) {
				// TODO write depending on the final specs
				break
			}
			if !c.isAppendable(lastBeacon, newBeacon) {
				c.l.Debug("new_aggregated", "not_appendable", "last", lastBeacon.String(), "new", newBeacon.String())
				c.maybeRunSync(currRound, lastBeacon, newBeacon)
				break
			}
			insert(newBeacon)
		case newBeacon := <-c.newBeaconNetwork:
			if lastBeacon.Equal(newBeacon) {
				// we dont even verify it
				break
			}
			if c.isReorg(lastBeacon, newBeacon) {
				// TODO write depending on the final specs
				break
			}
			if newBeacon.Round < lastBeacon.Round {
				break
			}
			if !c.isAppendable(lastBeacon, newBeacon) {
				// if it's not appendable directly, we may need to sync with
				// other nodes
				c.maybeRunSync(currRound, lastBeacon, newBeacon)
				break
			}
			insert(newBeacon)
		case info := <-newRound:
			currRound = info
		case <-c.done:
			return
		}
	}
}

func (c *chainStore) isReorg(last, newb *Beacon) bool {
	// TODO
	return false
}

func (c *chainStore) isAppendable(lastBeacon, newBeacon *Beacon) bool {
	if lastBeacon.Round >= newBeacon.Round {
		c.l.Debug("invalid_new_round", newBeacon.Round, "last_beacon_round", lastBeacon.Round, "new>last?", lastBeacon.Round >= newBeacon.Round)
		return false
	}

	if lastBeacon.Round != newBeacon.PreviousRound {
		c.l.Debug("invalid_previous_round", newBeacon.Round, "last_beacon_previous_round", lastBeacon.Round)
		return false
	}

	if !bytes.Equal(lastBeacon.Signature, newBeacon.PreviousSig) {
		c.l.Debug("invalid_previous_signature", shortSigStr(newBeacon.Signature), "last_beacon_signature", shortSigStr(lastBeacon.Signature))
		return false
	}
	return true
}

type likeBeacon interface {
	GetRound() uint64
	GetPreviousRound() uint64
}

// This MUST be called only if isAppendable returns false
func (c *chainStore) maybeRunSync(curr roundInfo, last *Beacon, newB likeBeacon) {
	if newB.GetPreviousRound() > last.GetRound() {
		// run sync !
		go c.RunSync(context.Background())
	}
}

// RunSync is a blocking call that tries to sync chain to the highest height
// found
func (c *chainStore) RunSync(ctx context.Context) {
	l, _ := c.Store.Last()
	currRound := c.ticker.CurrentRound()
	outCh, err := syncChain(ctx, c.l, c.safe, l, currRound, c.client)
	if err != nil {
		c.l.Error("error_sync", err)
		return
	}

	for newB := range outCh {
		c.newBeaconNetwork <- newB
	}
	return
}

type partialInfo struct {
	addr string
	p    *drand.PartialBeaconPacket
}

type beaconInfo struct {
	addr   string
	beacon *Beacon
}

type roundCache struct {
	round       uint64
	previous    uint64
	previousSig []byte
	sigs        [][]byte
	seens       map[int]bool
	done        bool
}

func newRoundCache(round, prev uint64, prevSig []byte) *roundCache {
	return &roundCache{
		round:       round,
		previous:    prev,
		previousSig: prevSig,
		seens:       make(map[int]bool),
	}
}

func (cache *roundCache) tryAppend(p *drand.PartialBeaconPacket) bool {
	round := p.GetRound()
	prevRound := p.GetPreviousRound()
	prevSig := p.GetPreviousSig()
	idx, _ := key.Scheme.IndexOf(p.GetPartialSig())
	if _, seen := cache.seens[idx]; seen {
		return false
	}

	sameRound := round == cache.round
	samePrevR := prevRound == cache.previous
	samePrevS := bytes.Equal(prevSig, cache.previousSig)
	if sameRound && samePrevR && samePrevS {
		cache.sigs = append(cache.sigs, p.GetPartialSig())
		cache.seens[idx] = true
		return true
	}
	return false
}

func (r *roundCache) Len() int {
	return len(r.sigs)
}

func (r *roundCache) Msg() []byte {
	return Message(r.previousSig, r.previous, r.round)
}

func (r *roundCache) Partials() [][]byte {
	return r.sigs
}
