package beacon

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
)

const (
	defaultPartialChanBuffer = 10
	defaultRequestSyncBuffer = 10
	defaultNewBeaconBuffer   = 100
)

// chainStore is a Store that deals with reconstructing the beacons, sync when
// needed and arranges the head
type chainStore struct {
	chain.Store
	conf          *Config
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

func newChainStore(l log.Logger, conf *Config, client net.ProtocolClient, safe *cryptoSafe, s chain.Store, ticker *ticker) *chainStore {
	c := &chainStore{
		l:             l,
		conf:          conf,
		client:        client,
		safe:          safe,
		Store:         s,
		done:          make(chan bool, 1),
		ticker:        ticker,
		newPartials:   make(chan partialInfo, defaultPartialChanBuffer),
		newBeaconCh:   make(chan *chain.Beacon, defaultNewBeaconBuffer),
		requestSync:   make(chan likeBeacon, defaultRequestSyncBuffer),
		lastInserted:  make(chan *chain.Beacon, 1),
		nonSyncBeacon: make(chan *chain.Beacon, 1),
	}
	// TODO maybe look if it's worth having multiple workers there
	go c.runChainLoop()
	go c.runAggregator()
	return c
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

	var cache = newPartialCache(c.l)
	for {
		select {
		case <-c.done:
			return
		case lastBeacon = <-c.lastInserted:
			cache.FlushRounds(lastBeacon.Round)
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
			isNotInPast := pRound > lastBeacon.Round
			isNotTooFar := pRound <= lastBeacon.Round+uint64(partialCacheStoreLimit+1)
			shouldStore := isNotInPast && isNotTooFar
			// check if we can reconstruct
			if !shouldStore {
				c.l.Debug("ignoring_partial", partial.p.GetRound(), "last_beacon_stored", lastBeacon.Round)
				break
			}
			thr := ginfo.group.Threshold
			cache.Append(partial.p)
			roundCache := cache.GetRoundCache(partial.p.GetRound(), partial.p.GetPreviousSig())
			if roundCache == nil {
				c.l.Error("store_partial", partial.addr, "no_round_cache", partial.p.GetRound())
				break
			}

			c.l.Debug("store_partial", partial.addr, "round", roundCache.round, "len_partials", fmt.Sprintf("%d/%d", roundCache.Len(), thr))
			if roundCache.Len() < thr {
				break
			}

			pub := ginfo.pub
			n := ginfo.group.Len()
			msg := roundCache.Msg()
			finalSig, err := key.Scheme.Recover(pub, msg, roundCache.Partials(), thr, n)
			if err != nil {
				c.l.Debug("invalid_recovery", err, "round", pRound, "got", fmt.Sprintf("%d/%d", roundCache.Len(), n))
				break
			}
			if err := key.Scheme.VerifyRecovered(pub.Commit(), msg, finalSig); err != nil {
				c.l.Error("invalid_sig", err, "round", pRound)
				break
			}
			cache.FlushRounds(partial.p.GetRound())
			newBeacon := &chain.Beacon{
				Round:       roundCache.round,
				PreviousSig: roundCache.prev,
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
		// measure beacon creation time discrepancy in milliseconds
		actual := time.Now().UnixNano()
		expected := chain.TimeOfRound(c.conf.Group.Period, c.conf.Group.GenesisTime, newB.Round) * 1e9
		discrepancy := float64(actual-expected) / float64(time.Millisecond)
		metrics.BeaconDiscrepancyLatency.Set(float64(actual-expected) / float64(time.Millisecond))
		c.l.Info("NEW_BEACON_STORED", newB.String(), "time_discrepancy_ms", discrepancy)
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
}

func (c *chainStore) AppendedBeaconNoSync() chan *chain.Beacon {
	return c.nonSyncBeacon
}

type partialInfo struct {
	addr string
	p    *drand.PartialBeaconPacket
}
