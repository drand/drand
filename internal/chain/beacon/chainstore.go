package beacon

import (
	"context"
	"errors"
	"fmt"

	"github.com/drand/drand/v2/common/tracer"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/crypto/vault"
	"github.com/drand/drand/v2/internal/chain"
	"github.com/drand/drand/v2/internal/net"
	"github.com/drand/drand/v2/protobuf/drand"
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
	crypto      *vault.Vault
	ticker      *ticker
	ctx         context.Context
	ctxCancel   context.CancelFunc
	newPartials chan partialInfo
	// catchupBeacons is used to notify the Handler when a node has aggregated a
	// beacon.
	catchupBeacons chan *common.Beacon
	// all beacons finally inserted into the store are sent over this channel for
	// the aggregation loop to know
	beaconStoredAgg chan *common.Beacon
}

//nolint:lll // The names are long but clear
func newChainStore(ctx context.Context, l log.Logger, cf *Config, cl net.ProtocolClient, v *vault.Vault, store chain.Store, t *ticker) (*chainStore, error) {
	ctx, span := tracer.NewSpan(ctx, "newChainStore")
	defer span.End()

	// we write some stats about the timing when new beacon is saved
	ds := newDiscrepancyStore(store, l, v.GetGroup(), cf.Clock)

	// we add a store to run some checks depending on scheme-related config
	ss, err := NewSchemeStore(ctx, ds, cf.Group.Scheme)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// we make sure the chain is increasing monotonically
	as, err := newAppendStore(ctx, ss)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// we can register callbacks on it
	cbs := NewCallbackStore(l, as)

	// we give the final append store to the sync manager
	syncm, err := NewSyncManager(ctx, &SyncConfig{
		Log:         l,
		Store:       cbs,
		BoltdbStore: store,
		Info:        v.GetInfo(),
		Client:      cl,
		Clock:       cf.Clock,
		NodeAddr:    cf.Public.Address(),
	})
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	go syncm.Run()

	ctx, ctxCancel := context.WithCancel(context.Background())

	cs := &chainStore{
		CallbackStore:   cbs,
		l:               l,
		conf:            cf,
		client:          cl,
		syncm:           syncm,
		crypto:          v,
		ticker:          t,
		ctx:             ctx,
		ctxCancel:       ctxCancel,
		newPartials:     make(chan partialInfo, defaultPartialChanBuffer),
		catchupBeacons:  make(chan *common.Beacon, 1),
		beaconStoredAgg: make(chan *common.Beacon, defaultNewBeaconBuffer),
	}
	// we add callbacks to notify each time a final beacon is stored on the
	// database so to update the latest view
	cbs.AddCallback("chainstore", func(b *common.Beacon, closed bool) {
		if closed {
			return
		}
		cs.beaconStoredAgg <- b
	})
	// TODO maybe look if it's worth having multiple workers there
	go func() {
		cs.runAggregator()
	}()
	return cs, nil
}

func (c *chainStore) NewValidPartial(ctx context.Context, addr string, p *drand.PartialBeaconPacket) {
	spanCtx := oteltrace.SpanContextFromContext(ctx)
	c.newPartials <- partialInfo{
		spanContext: spanCtx,
		addr:        addr,
		p:           p,
	}
}

func (c *chainStore) Stop() {
	c.ctxCancel()
	c.syncm.Stop()
	c.RemoveCallback("chainstore")
	_ = c.CallbackStore.Close()
}

// we store partials that are up to this amount of rounds more than the last
// beacon we have - it is useful to store partials that may come in advance,
// especially in case of a quick catchup.
var partialCacheStoreLimit = 3

// runAggregator runs a continuous loop that tries to aggregate partial
// signatures when it can.
//
//nolint:gocyclo,funlen // This function should be simplified, if possible.
func (c *chainStore) runAggregator() {
	select {
	case <-c.ctx.Done():
		return
	default:
		c.l.Debugw("starting chain_aggregator")
	}
	var lastBeacon *common.Beacon

	var cache = newPartialCache(c.l, c.crypto.Scheme)
	for {
		select {
		case <-c.ctx.Done():
			return
		case lastBeacon = <-c.beaconStoredAgg:
			cache.FlushRounds(lastBeacon.Round)
		case partial := <-c.newPartials:
			ctx, span := tracer.NewSpanFromSpanContext(c.ctx, partial.spanContext, "c.runAggregator")

			span.SetAttributes(
				attribute.Int64("round", int64(partial.p.Round)),
				attribute.String("addr", partial.addr),
			)

			var err error
			if lastBeacon == nil {
				lastBeacon, err = c.Last(ctx)
				if err != nil {
					span.RecordError(err)
					if errors.Is(err, context.Canceled) {
						c.l.Errorw("stopping chain_aggregator", "loading", "last_beacon", "err", err)
						span.End()
						return
					}
					if err.Error() == "sql: database is closed" {
						c.l.Errorw("stopping chain_aggregator", "loading", "last_beacon", "err", err)
						span.End()
						return
					}
					span.End()
					c.l.Fatalw("stopping chain_aggregator", "loading", "last_beacon", "err", err)
				}
			}

			// look if we have info for this round first
			pRound := partial.p.GetRound()
			// look if we want to store ths partial anyway
			isNotInPast := pRound > lastBeacon.Round
			isNotTooFar := pRound <= lastBeacon.Round+uint64(partialCacheStoreLimit+1)
			shouldStore := isNotInPast && isNotTooFar
			// check if we can reconstruct
			if !shouldStore {
				span.AddEvent("ignoring_partial")
				c.l.Debugw("", "ignoring_partial", partial.p.GetRound(), "last_beacon_stored", lastBeacon.Round)
				span.End()
				break
			}
			// NOTE: This line means we can only verify partial signatures of
			// the current group we are in as only current members should
			// participate in the randomness generation. Previous beacons can be
			// verified using the single distributed public key point from the
			// crypto store.
			thr := c.crypto.GetGroup().Threshold
			n := c.crypto.GetGroup().Len()

			select {
			case <-ctx.Done():
				span.AddEvent("ctx.Done")
				span.End()
				return
			default:
			}

			cache.Append(partial.p)
			roundCache := cache.GetRoundCache(partial.p.GetRound(), partial.p.GetPreviousSignature())
			if roundCache == nil {
				c.l.Errorw("", "store_partial", partial.addr, "no_round_cache", partial.p.GetRound())
				span.RecordError(errors.New("no round cache"))
				span.End()
				break
			}

			c.l.Debugw("", "store_partial", partial.addr,
				"round", roundCache.round, "len_partials", fmt.Sprintf("%d/%d", roundCache.Len(), thr))
			if roundCache.Len() < thr {
				span.AddEvent("roundCache.Len < thr")
				span.End()
				break
			}

			msg := c.crypto.DigestBeacon(roundCache)

			finalSig, err := c.crypto.Scheme.ThresholdScheme.Recover(c.crypto.GetPub(), msg, roundCache.Partials(), thr, n)
			if err != nil {
				c.l.Errorw("invalid_recovery", "error", err, "round", pRound, "got", fmt.Sprintf("%d/%d", roundCache.Len(), n))
				span.RecordError(errors.New("invalid recovery"))
				break
			}
			if err := c.crypto.Scheme.ThresholdScheme.VerifyRecovered(c.crypto.GetPub().Commit(), msg, finalSig); err != nil {
				c.l.Errorw("invalid_sig", "error", err, "round", pRound)
				span.RecordError(errors.New("invalid signature"))
				span.End()
				break
			}

			span.AddEvent("cache.FlushRounds")
			cache.FlushRounds(partial.p.GetRound())
			span.AddEvent("cache.FlushRounds - done")

			newBeacon := &common.Beacon{
				Round:       roundCache.round,
				PreviousSig: roundCache.prev,
				Signature:   finalSig,
			}

			c.l.Infow("", "aggregated_beacon", newBeacon.Round)
			span.AddEvent("calling tryAppend")
			if c.tryAppend(ctx, lastBeacon, newBeacon) {
				lastBeacon = newBeacon
				span.End()
				break
			}

			select {
			case <-c.ctx.Done():
				span.AddEvent("ctx.Done")
				span.End()
				return
			default:
			}

			// TODO store them for future usage if it's a later round than what we have
			c.l.Debugw("", "new_aggregated", "not_appendable", "last", lastBeacon.String(), "new", newBeacon.String())
			if c.shouldSync(lastBeacon, newBeacon) {
				peers := toPeers(c.crypto.GetGroup().Nodes)
				c.syncm.SendSyncRequest(ctx, newBeacon.Round, peers)
			}
			span.End()
		}
	}
}

func (c *chainStore) tryAppend(ctx context.Context, last, newB *common.Beacon) bool {
	ctx, span := tracer.NewSpan(ctx, "chainStore.tryAppend")
	defer span.End()

	select {
	case <-ctx.Done():
		return false
	default:
	}

	if last.Round+1 != newB.Round {
		// quick check before trying to compare bytes
		return false
	}

	if err := c.CallbackStore.Put(ctx, newB); err != nil {
		span.RecordError(err)
		// if round is ok but bytes are different, error will be raised
		if errors.Is(err, ErrBeaconAlreadyStored) {
			c.l.Debugw("Put: race with SyncManager", "err", err)
			select {
			// only send if it's not full already
			case c.catchupBeacons <- newB:
			default:
				c.l.Debugw("", "chain_store", "catchup", "channel", "full")
			}

			return true
		}

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

func (c *chainStore) shouldSync(last *common.Beacon, newB likeBeacon) bool {
	// we should sync if we are two blocks late
	return newB.GetRound() > last.GetRound()+1
}

// RunSync will sync up with other nodes and fill the store.
// It will start from the latest stored beacon. If upTo is equal to 0, then it
// will follow the chain indefinitely. If peers is nil, it uses the peers of
// the current group.
func (c *chainStore) RunSync(ctx context.Context, upTo uint64, peers []net.Peer) {
	ctx, span := tracer.NewSpan(ctx, "c.RunSync")
	defer span.End()

	if len(peers) == 0 {
		peers = toPeers(c.crypto.GetGroup().Nodes)
	}

	c.syncm.SendSyncRequest(ctx, upTo, peers)
}

// RunReSync will sync up with other nodes to repair the invalid beacons in the store.
func (c *chainStore) RunReSync(ctx context.Context, faultyBeacons []uint64, peers []net.Peer, cb func(r, u uint64)) error {
	ctx, span := tracer.NewSpan(ctx, "c.RunReSync")
	defer span.End()

	// we do this check here because the SyncManager doesn't have the notion of group
	if len(peers) == 0 {
		peers = toPeers(c.crypto.GetGroup().Nodes)
	}

	return c.syncm.CorrectPastBeacons(ctx, faultyBeacons, peers, cb)
}

// ValidateChain asks the sync manager to check the chain store up to the given beacon, in order to find invalid beacons
// and it returns the list of round numbers for which the beacons were corrupted / invalid / not found in the store.
// Note: it does not attempt to correct or fetch these faulty beacons.
func (c *chainStore) ValidateChain(ctx context.Context, upTo uint64, cb func(r, u uint64)) ([]uint64, error) {
	ctx, span := tracer.NewSpan(ctx, "c.ValidateChain")
	defer span.End()

	return c.syncm.CheckPastBeacons(ctx, upTo, cb)
}

func (c *chainStore) AppendedBeaconNoSync() chan *common.Beacon {
	return c.catchupBeacons
}

type partialInfo struct {
	spanContext oteltrace.SpanContext

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
