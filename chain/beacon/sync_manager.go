package beacon

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	cl "github.com/jonboulle/clockwork"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/common"
	proto "github.com/drand/drand/protobuf/drand"
)

// SyncManager manages all the sync requests to other peers. It performs a
// cancellation of sync requests if not progressing, performs rate limiting of
// sync requests.
type SyncManager struct {
	log    log.Logger
	clock  cl.Clock
	store  chain.Store
	info   *chain.Info
	client net.ProtocolClient
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
	isDone  bool
	mu      sync.Mutex
	// we need to know our current daemon address
	nodeAddr string
}

// sync manager will renew sync if nothing happens for factor*period time
var syncExpiryFactor = 2

// how many sync requests do we allow buffering
var syncQueueRequest = 3

type SyncConfig struct {
	Log      log.Logger
	Client   net.ProtocolClient
	Clock    cl.Clock
	Store    chain.Store
	Info     *chain.Info
	NodeAddr string
	BeaconID string
}

// NewSyncManager returns a sync manager that will use the given store to store
// newly synced beacon.
func NewSyncManager(c *SyncConfig) *SyncManager {
	return &SyncManager{
		log:      c.Log.Named("SyncManager"),
		clock:    c.Clock,
		store:    c.Store,
		info:     c.Info,
		client:   c.Client,
		period:   c.Info.Period,
		verifier: c.Info.Verifier(),
		nodeAddr: c.NodeAddr,
		factor:   syncExpiryFactor,
		newReq:   make(chan requestInfo, syncQueueRequest),
		newSync:  make(chan *chain.Beacon, 1),
		isDone:   false,
		done:     make(chan bool, 1),
	}
}

func (s *SyncManager) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isDone {
		return
	}
	s.isDone = true
	close(s.done)
}

type requestInfo struct {
	nodes []net.Peer
	upTo  uint64
}

// RequestSync asks the sync manager to sync up with those peers up to the given
// round. Depending on the current state of the syncing process, there might not
// be a new process starting (for example if we already have the round
// requested). upTo == 0 means the syncing process goes on forever.
func (s *SyncManager) RequestSync(nodes []net.Peer, upTo uint64) {
	s.newReq <- requestInfo{
		nodes: nodes,
		upTo:  upTo,
	}
}

func (s *SyncManager) Run() {
	// tracks the time of the last round we successfully synced
	lastRoundTime := 0
	// the context being used by the current sync process
	lastCtx, cancel := context.WithCancel(context.Background()) // nolint
	for {
		select {
		case request := <-s.newReq:
			// check if the request is still valid
			last, err := s.store.Last()
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
				lastCtx, cancel = context.WithCancel(context.Background())
				go s.sync(lastCtx, request)
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

func (s *SyncManager) sync(ctx context.Context, request requestInfo) {
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
			return
		default:
			node := request.nodes[n]
			if s.tryNode(ctx, request.upTo, node) {
				// we stop as soon as we've done a successful sync with a node
				return
			}
		}
	}
	s.log.Debugw("Tried all nodes without success", "sync_manager", "failed sync")
}

// tryNode tries to sync up with the given peer up to the given round, starting
// from the last beacon in the store. It returns true if the objective was
// reached (store.Last() returns upTo) and false otherwise.
func (s *SyncManager) tryNode(global context.Context, upTo uint64, peer net.Peer) bool {
	// we put a cancel to still keep the global context open but stop with this
	// peer if things go sideway
	cnode, cancel := context.WithCancel(global)
	defer cancel()
	last, err := s.store.Last()
	if err != nil {
		return false
	}

	logger := s.log.Named("tryNode")
	req := &proto.SyncRequest{
		FromRound: last.Round + 1,
		Metadata:  &common.Metadata{BeaconID: s.info.ID},
	}
	beaconCh, err := s.client.SyncChain(cnode, peer, req)
	if err != nil {
		logger.Debugw("unable_to_sync", "with_peer", peer.Address(), "err", err)
		return false
	}

	logger.Debugw("start_sync", "with_peer", peer.Address(), "from_round", last.Round+1)

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
				logger.Debugw("wrong beaconID", "expected", s.info.ID, "got", metadata.BeaconID)
				return false
			}

			logger.Debugw("new_beacon_fetched",
				"with_peer", peer.Address(),
				"from_round", last.Round+1,
				"got_round", beaconPacket.GetRound())
			beacon := protoToBeacon(beaconPacket)

			// verify the signature validity
			if err := s.verifier.VerifyBeacon(*beacon, s.info.PublicKey); err != nil {
				logger.Debugw("invalid_beacon", "with_peer", peer.Address(), "round", beacon.Round, "err", err, "beacon", fmt.Sprintf("%+v", beacon))
				return false
			}

			if err := s.store.Put(beacon); err != nil {
				logger.Debugw("unable to save", "with_peer", peer.Address(), "err", err)
				return false
			}
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
	addr := net.RemoteAddress(stream.Context())
	id := addr + strconv.Itoa(rand.Int()) // nolint
	l.Debugw("Starting SyncChain", "syncer", "sync_request", "from", addr, "from_round", fromRound, "beaconID", req.GetMetadata().BeaconID)

	last, err := store.Last()
	if err != nil {
		return err
	}
	if last.Round < fromRound {
		return fmt.Errorf("no beacon stored above requested round %d < %d", last.Round, fromRound)
	}

	var done = make(chan error, 1)
	logger := l.Named("Send")
	send := func(b *chain.Beacon) bool {
		packet := beaconToProto(b)
		packet.Metadata = &common.Metadata{BeaconID: req.GetMetadata().BeaconID}
		if err := stream.Send(packet); err != nil {
			logger.Debugw("", "syncer", "streaming_send", "err", err)
			done <- err
			return false
		}
		return true
	}
	if fromRound != 0 && fromRound <= last.Round {
		// first sync up from the store itself
		var shouldContinue = true
		store.Cursor(func(c chain.Cursor) {
			for bb := c.Seek(fromRound); bb != nil; bb = c.Next() {
				if !send(bb) {
					shouldContinue = false
					return
				}
			}
		})
		if !shouldContinue {
			return <-done
		}
	}
	// then register a callback to process new incoming beacons
	store.AddCallback(id, func(b *chain.Beacon) {
		if !send(b) {
			store.RemoveCallback(id)
		}
	})
	defer store.RemoveCallback(id)
	// either wait that the request cancels out or wait there's an error sending
	// to the stream
	select {
	case <-stream.Context().Done():
		return stream.Context().Err()
	case err := <-done:
		return err
	}
}

func peersToString(peers []net.Peer) string {
	adds := make([]string, 0, len(peers))
	for _, p := range peers {
		adds = append(adds, p.Address())
	}
	return "[ " + strings.Join(adds, " - ") + " ]"
}
