package beacon

import (
	"fmt"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
)

const (
	defaultPartialChanBuffer = 10
	defaultNewBeaconBuffer   = 100
)

// chainStore implements CallbackStore, Syncer and deals with reconstructing the
// beacons, and sync when needed. This struct is the gateway logic for beacons to
// be inserted in the database and for replying to beacon requests.
type chainStore struct {
	CallbackStore
	l           log.Logger
	conf        *Config
	client      net.ProtocolClient
	syncm       *SyncManager
	verifier    *chain.Verifier
	crypto      *cryptoStore
	ticker      *ticker
	done        chan bool
	newPartials chan partialInfo
	// catchupBeacons is used to notify the Handler when a node has aggregated a
	// beacon.
	catchupBeacons chan *chain.Beacon
	// all beacons finally inserted into the store are sent over this cannel for
	// the aggregation loop to know
	beaconStoredAgg chan *chain.Beacon
}

func newChainStore(l log.Logger, cf *Config, cl net.ProtocolClient, c *cryptoStore, store chain.Store, t *ticker) *chainStore {
	// we make sure the chain is increasing monotonically
	as := newAppendStore(store)

	// we add a store to run some checks depending on scheme-related config
	ss := NewSchemeStore(as, cf.Group.Scheme)

	// we write some stats about the timing when new beacon is saved
	ds := newDiscrepancyStore(ss, l, c.GetGroup(), cf.Clock)

	// we can register callbacks on it
	cbs := NewCallbackStore(ds)

	// we give the final append store to the sync manager
	syncm := NewSyncManager(&SyncConfig{
		Log:      l,
		Store:    cbs,
		Info:     c.chain,
		Client:   cl,
		Clock:    cf.Clock,
		NodeAddr: cf.Public.Address(),
		BeaconID: cf.Group.ID,
	})
	go syncm.Run()

	verifier := chain.NewVerifier(cf.Group.Scheme)

	cs := &chainStore{
		CallbackStore:   cbs,
		l:               l,
		conf:            cf,
		client:          cl,
		syncm:           syncm,
		verifier:        verifier,
		crypto:          c,
		ticker:          t,
		done:            make(chan bool, 1),
		newPartials:     make(chan partialInfo, defaultPartialChanBuffer),
		catchupBeacons:  make(chan *chain.Beacon, 1),
		beaconStoredAgg: make(chan *chain.Beacon, defaultNewBeaconBuffer),
	}
	// we add callbacks to notify each time a final beacon is stored on the
	// database so to update the latest view
	cbs.AddCallback("chainstore", func(b *chain.Beacon) {
		cs.beaconStoredAgg <- b
	})
	// TODO maybe look if it's worth having multiple workers there
	go cs.runAggregator()
	return cs
}

func (c *chainStore) NewValidPartial(addr string, p *drand.PartialBeaconPacket) {
	c.newPartials <- partialInfo{
		addr: addr,
		p:    p,
	}
}

func (c *chainStore) Stop() {
	c.syncm.Stop()
	c.CallbackStore.Close()
	close(c.done)
}

// we store partials that are up to this amount of rounds more than the last
// beacon we have - it is useful to store partials that may come in advance,
// especially in case of a quick catchup.
var partialCacheStoreLimit = 3

// runAggregator runs a continuous loop that tries to aggregate partial
// signatures when it can
func (c *chainStore) runAggregator() {
	beaconID := c.conf.Group.ID
	lastBeacon, err := c.Last()
	if err != nil {
		c.l.Fatalw("", "chain_aggregator", "loading", "last_beacon", err)
	}

	var cache = newPartialCache(c.l, beaconID)
	for {
		select {
		case <-c.done:
			return
		case lastBeacon = <-c.beaconStoredAgg:
			cache.FlushRounds(lastBeacon.Round)
			break
		case partial := <-c.newPartials:
			// look if we have info for this round first
			pRound := partial.p.GetRound()
			// look if we want to store ths partial anyway
			isNotInPast := pRound > lastBeacon.Round
			isNotTooFar := pRound <= lastBeacon.Round+uint64(partialCacheStoreLimit+1)
			shouldStore := isNotInPast && isNotTooFar
			// check if we can reconstruct
			if !shouldStore {
				c.l.Debugw("", "ignoring_partial", partial.p.GetRound(), "last_beacon_stored", lastBeacon.Round)
				break
			}
			// NOTE: This line means we can only verify partial signatures of
			// the current group we are in as only current members should
			// participate in the randomness generation. Previous beacons can be
			// verified using the single distributed public key point from the
			// crypto store.
			thr := c.crypto.GetGroup().Threshold
			n := c.crypto.GetGroup().Len()
			cache.Append(partial.p)
			roundCache := cache.GetRoundCache(partial.p.GetRound(), partial.p.GetPreviousSig())
			if roundCache == nil {
				c.l.Errorw("", "store_partial", partial.addr, "no_round_cache", partial.p.GetRound())
				break
			}

			c.l.Debugw("", "store_partial", partial.addr,
				"round", roundCache.round, "len_partials", fmt.Sprintf("%d/%d", roundCache.Len(), thr))
			if roundCache.Len() < thr {
				break
			}

			msg := c.verifier.DigestMessage(roundCache.round, roundCache.prev)

			finalSig, err := key.Scheme.Recover(c.crypto.GetPub(), msg, roundCache.Partials(), thr, n)
			if err != nil {
				c.l.Debugw("", "invalid_recovery", err, "round", pRound, "got", fmt.Sprintf("%d/%d", roundCache.Len(), n))
				break
			}
			if err := key.Scheme.VerifyRecovered(c.crypto.GetPub().Commit(), msg, finalSig); err != nil {
				c.l.Errorw("", "invalid_sig", err, "round", pRound)
				break
			}
			cache.FlushRounds(partial.p.GetRound())

			newBeacon := &chain.Beacon{
				Round:       roundCache.round,
				PreviousSig: roundCache.prev,
				Signature:   finalSig,
			}

			c.l.Infow("", "aggregated_beacon", newBeacon.Round)
			if c.tryAppend(lastBeacon, newBeacon) {
				lastBeacon = newBeacon
				break
			}
			// XXX store them for future usage if it's a later round than what we have
			c.l.Debugw("", "new_aggregated", "not_appendable", "last", lastBeacon.String(), "new", newBeacon.String())
			if c.shouldSync(lastBeacon, newBeacon) {
				peers := toPeers(c.crypto.GetGroup().Nodes)
				c.syncm.RequestSync(peers, newBeacon.Round)
			}
			break
		}
	}
}

func (c *chainStore) tryAppend(last, newB *chain.Beacon) bool {
	if last.Round+1 != newB.Round {
		// quick check before trying to compare bytes
		return false
	}

	if err := c.CallbackStore.Put(newB); err != nil {
		// if round is ok but bytes are different, error will be raised
		c.l.Errorw("", "chain_store", "error storing beacon", "err", err)
		return false
	}
	select {
	// only send if it's not full already
	case c.catchupBeacons <- newB:
	default:
		c.l.Debugw("", "chain_store", "catchup", "channel", "full")
	}
	return true
}

type likeBeacon interface {
	GetRound() uint64
}

func (c *chainStore) shouldSync(last *chain.Beacon, newB likeBeacon) bool {
	// we should sync if we are two blocks late
	return newB.GetRound() > last.GetRound()+1
}

// RunSync will sync up with other nodes and fill the store. If upTo is equal to
// 0, then it will follow the chain indefinitely. If peers is nil, it uses the
// peers of the current group.
func (c *chainStore) RunSync(upTo uint64, peers []net.Peer) {
	if peers == nil {
		peers = toPeers(c.crypto.GetGroup().Nodes)
	}

	c.syncm.RequestSync(peers, upTo)
}

func (c *chainStore) AppendedBeaconNoSync() chan *chain.Beacon {
	return c.catchupBeacons
}

type partialInfo struct {
	addr string
	p    *drand.PartialBeaconPacket
}

func toPeers(nodes []*key.Node) []net.Peer {
	peers := make([]net.Peer, len(nodes))
	for i := 0; i < len(nodes); i++ {
		peers[i] = nodes[i].Identity
	}
	return peers
}
