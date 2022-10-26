package beacon

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	cl "github.com/jonboulle/clockwork"

	"github.com/drand/drand/chain"
	chainerrors "github.com/drand/drand/chain/errors"
	commonutils "github.com/drand/drand/common"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/common"
	proto "github.com/drand/drand/protobuf/drand"
)

// SyncManager manages all the sync requests to other peers. It performs a
// cancellation of sync requests if not progressing, performs rate limiting of
// sync requests.
type SyncManager struct {
	log   log.Logger
	clock cl.Clock
	store chain.Store
	// insecureStore will store beacons without doing any checks
	insecureStore chain.Store
	info          *chain.Info
	client        net.ProtocolClient
	// verifies the incoming beacon according to chain scheme
	verifier *chain.Verifier
	// period of the randomness generation
	period time.Duration
	// sync manager will renew sync if nothing happens for factor*period time
	factor int
	// receives new requests of sync
	newReq chan requestInfo
	// updated with each new beacon we receive from sync
	newSync chan *chain.Beacon
	done    chan bool
	mu      sync.Mutex
	// we need to know our current daemon address
	nodeAddr string
}

// sync manager will renew sync if nothing happens for factor*period time
var syncExpiryFactor = 2

// how many sync requests do we allow buffering
var syncQueueRequest = 3

// ErrFailedAll means all nodes failed to provide the requested beacons
var ErrFailedAll = errors.New("sync failed: tried all nodes")

type SyncConfig struct {
	Log         log.Logger
	Client      net.ProtocolClient
	Clock       cl.Clock
	Store       chain.Store
	BoltdbStore chain.Store
	Info        *chain.Info
	NodeAddr    string
}

// NewSyncManager returns a sync manager that will use the given store to store
// newly synced beacon.
func NewSyncManager(c *SyncConfig) *SyncManager {
	return &SyncManager{
		log:           c.Log.Named("SyncManager"),
		clock:         c.Clock,
		store:         c.Store,
		insecureStore: c.BoltdbStore,
		info:          c.Info,
		client:        c.Client,
		period:        c.Info.Period,
		verifier:      c.Info.Verifier(),
		nodeAddr:      c.NodeAddr,
		factor:        syncExpiryFactor,
		newReq:        make(chan requestInfo, syncQueueRequest),
		newSync:       make(chan *chain.Beacon, 1),
		done:          make(chan bool, 1),
	}
}

func (s *SyncManager) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	close(s.done)
}

type requestInfo struct {
	nodes []net.Peer
	from  uint64
	upTo  uint64
}

// RequestSync asks the sync manager to sync up with those peers up to the given
// round. Depending on the current state of the syncing process, there might not
// be a new process starting (for example if we already have the round
// requested). upTo == 0 means the syncing process goes on forever.
func (s *SyncManager) RequestSync(upTo uint64, nodes []net.Peer) {
	s.newReq <- requestInfo{
		nodes: nodes,
		upTo:  upTo,
	}
}

// Run handles non-blocking sync requests coming from the regular operation of the daemon
func (s *SyncManager) Run() {
	// no need to sync until genesis time
	for s.clock.Now().Unix() < s.info.GenesisTime {
		time.Sleep(time.Second)
	}
	// tracks the time of the last round we successfully synced
	lastRoundTime := 0
	// the context being used by the current sync process
	ctx, cancel := context.WithCancel(context.Background())
	for {
		select {
		case request := <-s.newReq:
			// check if the request is still valid
			last, err := s.store.Last(ctx)
			if err != nil {
				s.log.Debugw("unable to fetch from store", "sync_manager", "store.Last", "err", err)
				continue
			}
			// do we really need a sync request ?
			if request.upTo > 0 && last.Round >= request.upTo {
				s.log.Debugw("request already filled", "sync_manager", "skipping_request", "last", last.Round, "request", request.upTo)
				continue
			}
			// check if it's been a while we haven't received a new round from
			// sync. Either there is a sync in progress but it's stuck, so we
			// quit it and start a new one, or there isn't and we start one.
			// We always give a delay of a few periods since the one next to "now"
			// might not be exactly ready yet so only after a few periods we know we
			// must have gotten some data.
			upperBound := lastRoundTime + int(s.period.Seconds())*s.factor
			if upperBound < int(s.clock.Now().Unix()) {
				// we haven't received a new block in a while
				// -> time to start a new sync
				cancel()
				ctx, cancel = context.WithCancel(context.Background())
				//nolint
				go s.Sync(ctx, request)
			}

		case <-s.newSync:
			// just received a new beacon from sync, we keep track of this time
			lastRoundTime = int(s.clock.Now().Unix())
		case <-s.done:
			s.log.Infow("", "sync_manager", "exits")
			cancel()
			return
		}
	}
}

func (s *SyncManager) CheckPastBeacons(ctx context.Context, upTo uint64, cb func(r, u uint64)) ([]uint64, error) {
	logger := s.log.Named("pastBeaconCheck")
	logger.Debugw("Starting to check past beacons", "upTo", upTo)

	last, err := s.store.Last(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch and check last beacon in store: %w", err)
	}

	if last.Round < upTo {
		logger.Errorw("No beacon stored above", "last round", last.Round, "requested round", upTo)
		logger.Infow("Checking beacons only up to the last stored", "round", last.Round)
		upTo = last.Round
	}

	var faultyBeacons []uint64
	// notice that we do not validate the genesis round 0
	storeLen, err := s.store.Len(ctx)
	if err != nil {
		return nil, fmt.Errorf("error while retrieving store size: %w", err)
	}
	for i := uint64(1); i < uint64(storeLen); i++ {
		select {
		case <-ctx.Done():
			logger.Debugw("Context done, returning")
			return nil, ctx.Err()
		default:
		}

		// we call our callback with the round to send the progress, N.B. we need to do it before returning.
		// Batching/rate-limiting is handled on the callback side
		if cb != nil {
			cb(i, upTo)
		}

		b, err := s.store.Get(ctx, i)
		if err != nil {
			logger.Errorw("unable to fetch beacon in store", "round", i, "err", err)
			faultyBeacons = append(faultyBeacons, i)
			if i >= upTo {
				break
			}
			continue
		}
		// verify the signature validity
		if err = s.verifier.VerifyBeacon(*b, s.info.PublicKey); err != nil {
			logger.Errorw("invalid_beacon", "round", b.Round, "err", err)
			faultyBeacons = append(faultyBeacons, b.Round)
		} else if i%commonutils.LogsToSkip == 0 { // we do some rate limiting on the logging
			logger.Debugw("valid_beacon", "round", b.Round)
		}

		if i >= upTo {
			break
		}
	}

	logger.Debugw("Finished checking past beacons", "faulty_beacons", len(faultyBeacons))

	if len(faultyBeacons) > 0 {
		logger.Warnw("Found invalid beacons in store", "amount", len(faultyBeacons))
		return faultyBeacons, nil
	}

	return nil, nil
}

func (s *SyncManager) CorrectPastBeacons(ctx context.Context, faultyBeacons []uint64, peers []net.Peer, cb func(r, u uint64)) error {
	target := uint64(len(faultyBeacons))
	if target == 0 {
		return nil
	}
	if cb == nil {
		return fmt.Errorf("undefined callback for CorrectPastBeacons")
	}

	var errAcc []error
	for i, b := range faultyBeacons {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		cb(uint64(i+1), target)
		s.log.Debugw("Fetching from peers incorrect beacon", "round", b)

		err := s.ReSync(ctx, b, b, peers)
		if err != nil {
			errAcc = append(errAcc, err)
		}
	}

	if len(errAcc) > 0 {
		s.log.Errorw("One or more errors occurred while correcting the chain", "errors", errAcc)
		return fmt.Errorf("error while correcting past beacons. First error: %w; All errors: %+v", errAcc[0], errAcc)
	}

	return nil
}

// ReSync handles resyncs that where necessarily launched by a CLI.
func (s *SyncManager) ReSync(ctx context.Context, from, to uint64, nodes []net.Peer) error {
	s.log.Debugw("Launching re-sync request", "from", from, "upTo", to)

	if from == 0 {
		return fmt.Errorf("invalid re-sync: from %d to %d", from, to)
	}

	// we always do it and we block while doing it if it's a resync. Notice that the regular sync will
	// keep running in the background in their own go routine.
	err := s.Sync(ctx, requestInfo{
		nodes: nodes,
		from:  from,
		upTo:  to,
	})

	if errors.Is(err, ErrFailedAll) {
		s.log.Warnw("All node have failed resync once, retrying one time")
		err = s.Sync(ctx, requestInfo{
			nodes: nodes,
			from:  from,
			upTo:  to,
		})
	}

	return err
}

// Sync will launch the requested sync with the requested peers and returns once done, even if it failed
func (s *SyncManager) Sync(ctx context.Context, request requestInfo) error {
	s.log.Debugw("starting new sync", "sync_manager", "start sync", "up_to", request.upTo, "nodes", peersToString(request.nodes))
	// shuffle through the nodes
	for _, n := range rand.Perm(len(request.nodes)) {
		if request.nodes[n].Address() == s.nodeAddr {
			// we ignore our own node
			s.log.Debugw("skipping sync with our own node", "sync_manager", "sync")
			continue
		}
		select {
		// let us cancel early in case the context is canceled
		case <-ctx.Done():
			s.log.Debugw("sync canceled early", "source", "ctx", "err?", ctx.Err())
			return fmt.Errorf("ctx done: sync canceled")
		default:
			node := request.nodes[n]
			if s.tryNode(ctx, request.from, request.upTo, node) {
				// we stop as soon as we've done a successful sync with a node
				return nil
			}
		}
	}
	s.log.Debugw("Tried all nodes without success", "sync_manager", "failed sync")
	return ErrFailedAll
}

// tryNode tries to sync up with the given peer up to the given round, starting
// from the last beacon in the store. It returns true if the objective was
// reached (store.Last() returns upTo) and false otherwise.
//
//nolint:gocyclo,funlen
func (s *SyncManager) tryNode(global context.Context, from, upTo uint64, peer net.Peer) bool {
	logger := s.log.Named("tryNode")

	// we put a cancel to still keep the global context open but stop with this
	// peer if things go sideway
	cnode, cancel := context.WithCancel(global)
	defer cancel()

	// if from > 0 then we're doing a ReSync, not a plain Sync.
	isResync := from > 0

	last, err := s.store.Last(cnode)
	if err != nil {
		logger.Errorw("unable to fetch from store", "sync_manager", "store.Last", "err", err)
		return false
	}

	if from == 0 {
		from = last.Round + 1
	} else if from > upTo {
		logger.Errorw("Invalid request: from > upTo", "from", from, "upTo", upTo)
		return false
	}

	req := &proto.SyncRequest{
		FromRound: from,
		Metadata:  &common.Metadata{BeaconID: s.info.ID},
	}

	beaconCh, err := s.client.SyncChain(cnode, peer, req)
	if err != nil {
		logger.Errorw("unable_to_sync", "with_peer", peer.Address(), "err", err)
		return false
	}

	// for effective rate limiting but not when we are caught up and following a chain live
	target := chain.CurrentRound(s.clock.Now().Unix(), s.info.Period, s.info.GenesisTime)
	if upTo > 0 {
		target = upTo
	}

	logger.Debugw("start_sync", "with_peer", peer.Address(), "from_round", from, "up_to", upTo)
	s.log.Debugw("sync log rate limiting", "skipping logs", commonutils.LogsToSkip)

	for {
		select {
		case beaconPacket, ok := <-beaconCh:
			if !ok {
				logger.Debugw("SyncChain channel closed", "with_peer", peer.Address())
				return false
			}

			// Check if we got the right packet
			metadata := beaconPacket.GetMetadata()
			if metadata != nil && metadata.BeaconID != s.info.ID {
				logger.Errorw("wrong beaconID", "expected", s.info.ID, "got", metadata.BeaconID)
				return false
			}

			// We rate limit our logging, but when we are "close enough", we display all logs in case we want to follow
			// for a long time.
			if idx := beaconPacket.GetRound(); target < idx || target-idx < commonutils.LogsToSkip || idx%commonutils.LogsToSkip == 0 {
				logger.Debugw("new_beacon_fetched",
					"with_peer", peer.Address(),
					"from_round", from,
					"got_round", idx)
			}

			beacon := protoToBeacon(beaconPacket)

			// verify the signature validity
			if err := s.verifier.VerifyBeacon(*beacon, s.info.PublicKey); err != nil {
				logger.Debugw("Invalid_beacon", "from_peer", peer.Address(), "round", beacon.Round, "err", err, "beacon", fmt.Sprintf("%+v", beacon))
				return false
			}

			if isResync {
				logger.Debugw("Resync Put: trying to save beacon", "beacon", beacon.Round)
				if err := s.insecureStore.Put(cnode, beacon); err != nil {
					logger.Errorw("Resync Put: unable to save", "with_peer", peer.Address(), "err", err)
					return false
				}
			} else {
				if err := s.store.Put(cnode, beacon); err != nil {
					logger.Errorw("Put: unable to save", "with_peer", peer.Address(), "err", err)
					return false
				}
			}

			// TODO: fix the fact that we currently never send beacons on newSync and always restart the sync
			// 		 when receiving new sync requests. See #1020.
			// we let know the sync manager that we received a beacon
			// s.newSync <- beacon

			last = beacon
			if last.Round == upTo {
				logger.Debugw("sync_manager finished syncing up to", "round", upTo)
				return true
			}
			// else, we keep waiting for the next beacons
		case <-cnode.Done():
			// it can be the remote note that stopped the syncing or a network error with it
			logger.Debugw("sync canceled", "source", "remote", "err?", cnode.Err())
			// we still go on with the other peers
			return false
		case <-global.Done():
			// or a cancellation of the syncing process itself, maybe because it's stuck
			logger.Debugw("sync canceled", "source", "global", "err?", global.Err())
			// we stop
			return false
		}
	}
}

// SyncRequest is an interface representing any kind of request to sync.
// Those exist in both the protocol API and the public API.
type SyncRequest interface {
	GetFromRound() uint64
	GetMetadata() *common.Metadata
}

// SyncStream is an interface representing any kind of stream to send beacons to.
// Those exist in both the protocol API and the public API.
type SyncStream interface {
	Context() context.Context
	Send(*proto.BeaconPacket) error
}

// SyncChain holds the receiver logic to reply to a sync request
func SyncChain(l log.Logger, store CallbackStore, req SyncRequest, stream SyncStream) error {
	fromRound := req.GetFromRound()
	ctx := stream.Context()
	addr := net.RemoteAddress(ctx)
	id := addr + strconv.Itoa(rand.Int()) //nolint

	logger := l.Named("SyncChain")

	beaconID := beaconIDToSync(l, req, addr)

	last, err := store.Last(ctx)
	if err != nil {
		return fmt.Errorf("unable to get last beacon: %w", err)
	}

	if last.Round < fromRound {
		return fmt.Errorf("%w %d < %d", chainerrors.ErrNoBeaconStored, last.Round, fromRound)
	}

	send := func(b *chain.Beacon) error {
		packet := beaconToProto(b)
		packet.Metadata = &common.Metadata{BeaconID: beaconID}
		err := stream.Send(packet)
		if err != nil {
			logger.Debugw("", "syncer", "streaming_send", "err", err)
		}
		return err
	}

	// we know that last.Round >= fromRound from the above if
	if fromRound != 0 {
		// TODO (dlsniper): During the loop below, we can receive new data
		//  which may not be observed as the callback is added after the loop ends.
		//  Investigate if how the storage view updates while the cursor runs.

		// first sync up from the store itself
		err = store.Cursor(ctx, func(ctx context.Context, c chain.Cursor) error {
			bb, err := c.Seek(ctx, fromRound)
			for ; bb != nil; bb, err = c.Next(ctx) {
				// This is needed since send will use a pointer and could result in pointer reassignment
				bb := bb
				if err != nil {
					return err
				}
				// Force send the correct
				if err := send(bb); err != nil {
					logger.Debugw("Error while sending beacon", "syncer", "cursor_seek")
					return err
				}
			}

			return err
		})
		if err != nil {
			// We always have ErrNoBeaconStored returned as last value
			// so let's ignore it and not send it back to the client
			if !errors.Is(err, chainerrors.ErrNoBeaconStored) {
				return err
			}
		}
	}

	// Register a callback to process all new incoming beacons until an error happens.
	// The callback happens in a separate goroutine.
	errChan := make(chan error)
	store.AddCallback(id, func(b *chain.Beacon) {
		if err := send(b); err != nil {
			logger.Debugw("Error while sending beacon", "syncer", "callback")
			store.RemoveCallback(id)
			errChan <- err
		}
	})

	defer store.RemoveCallback(id)

	// Wait until the request cancels or until an error happens in the callback.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errChan:
		return err
	}
}

// Versions prior to 1.4 did not support multibeacon and thus did not have attached metadata.
// This function resolves the `beaconId` given a `SyncRequest`
func beaconIDToSync(logger log.Logger, req SyncRequest, addr string) string {
	// this should only happen if the requester is on a version < 1.4
	if req.GetMetadata() == nil {
		logger.Errorw("Received a sync request without metadata - probably an old version", "from_addr", addr)
		return commonutils.DefaultBeaconID
	}
	return req.GetMetadata().GetBeaconID()
}

func peersToString(peers []net.Peer) string {
	adds := make([]string, 0, len(peers))
	for _, p := range peers {
		adds = append(adds, p.Address())
	}
	return "[ " + strings.Join(adds, " - ") + " ]"
}
