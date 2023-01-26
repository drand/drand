package beacon

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/drand/drand/crypto/vault"

	clock "github.com/jonboulle/clockwork"

	"github.com/drand/drand/chain"
	commonutils "github.com/drand/drand/common"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/common"
	proto "github.com/drand/drand/protobuf/drand"
)

// Config holds the different cryptographc informations necessary to run the
// randomness beacon.
type Config struct {
	// Public key of this node
	Public *key.Node
	// Share of this node in the network
	Share *key.Share
	// Group listing all nodes and public key of the network
	Group *key.Group
	// Clock to use - useful to testing
	Clock clock.Clock
}

// Handler holds the logic to initiate, and react to the tBLS protocol. Each time
// a full signature can be reconstructed, it saves it to the given Store.
//
//nolint:gocritic
type Handler struct {
	sync.Mutex
	conf *Config
	// to communicate with other drand peers
	client net.ProtocolClient
	// keeps the cryptographic info (group share etc)
	crypto *vault.Vault
	// main logic that treats incoming packet / new beacons created
	chain  *chainStore
	ticker *ticker

	close   chan bool
	addr    string
	started bool
	running bool
	serving bool
	stopped bool
	version commonutils.Version
	l       log.Logger
}

// NewHandler returns a fresh handler ready to serve and create randomness
// beacon
func NewHandler(c net.ProtocolClient, s chain.Store, conf *Config, l log.Logger, version commonutils.Version) (*Handler, error) {
	if conf.Share == nil || conf.Group == nil {
		return nil, errors.New("beacon: invalid configuration")
	}
	// Checking we are in the group
	node := conf.Group.Find(conf.Public.Identity)
	if node == nil {
		return nil, errors.New("beacon: keypair not included in the given group")
	}
	addr := conf.Public.Address()

	v := vault.NewVault(conf.Group, conf.Share, conf.Group.Scheme)
	// insert genesis beacon
	if err := s.Put(context.Background(), chain.GenesisBeacon(conf.Group.GenesisSeed)); err != nil {
		return nil, err
	}

	ticker := newTicker(conf.Clock, conf.Group.Period, conf.Group.GenesisTime)
	store, err := newChainStore(l, conf, c, v, s, ticker)
	if err != nil {
		return nil, err
	}

	handler := &Handler{
		conf:    conf,
		client:  c,
		crypto:  v,
		chain:   store,
		ticker:  ticker,
		addr:    addr,
		close:   make(chan bool),
		l:       l,
		version: version,
	}
	return handler, nil
}

var errOutOfRound = "out-of-round beacon request"

// ProcessPartialBeacon receives a request for a beacon partial signature. It
// forwards it to the round manager if it is a valid beacon.
func (h *Handler) ProcessPartialBeacon(c context.Context, p *proto.PartialBeaconPacket) (*proto.Empty, error) {
	addr := net.RemoteAddress(c)
	h.l.Debugw("", "received", "request", "from", addr, "round", p.GetRound())

	nextRound, _ := chain.NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	currentRound := nextRound - 1

	// we allow one round off in the future because of small clock drifts
	// possible, if a node receives a packet very fast just before his local
	// clock passed to the next round
	if p.GetRound() > nextRound {
		h.l.Errorw("", "process_partial", addr, "invalid_future_round", p.GetRound(), "current_round", currentRound)
		return nil, fmt.Errorf("invalid round: %d instead of %d", p.GetRound(), currentRound)
	}

	msg := h.crypto.DigestBeacon(&chain.Beacon{Round: p.GetRound(), PreviousSig: p.GetPreviousSignature()})

	idx, _ := h.crypto.ThresholdScheme.IndexOf(p.GetPartialSig())
	if idx < 0 {
		return nil, fmt.Errorf("invalid index %d in partial with msg %v", idx, msg)
	}

	node := h.crypto.GetGroup().Node(uint32(idx))

	if node == nil {
		return nil, fmt.Errorf("attempted to process beacon from node of index %d, but it was not in the group file", uint32(idx))
	}

	nodeName := node.Address()
	// verify if request is valid
	if err := h.crypto.ThresholdScheme.VerifyPartial(h.crypto.GetPub(), msg, p.GetPartialSig()); err != nil {
		h.l.Errorw("",
			"process_partial", addr, "err", err,
			"prev_sig", shortSigStr(p.GetPreviousSignature()),
			"curr_round", currentRound,
			"msg_sign", shortSigStr(msg),
			"from_idx", idx,
			"from_node", nodeName)
		return nil, err
	}
	h.l.Debugw("",
		"process_partial", addr,
		"prev_sig", shortSigStr(p.GetPreviousSignature()),
		"curr_round", currentRound,
		"msg_sign", shortSigStr(msg),
		"from_node", nodeName,
		"status", "OK")
	if idx == h.crypto.Index() {
		h.l.Errorw("",
			"process_partial", addr,
			"index_got", idx,
			"index_our", h.crypto.Index(),
			"advance_packet", p.GetRound(),
			"from_node", nodeName)
		// XXX error or not ?
		return new(proto.Empty), nil
	}
	h.chain.NewValidPartial(addr, p)
	return new(proto.Empty), nil
}

// Store returns the store associated with this beacon handler
func (h *Handler) Store() CallbackStore {
	return h.chain
}

// Start runs the beacon protocol (threshold BLS signature). The first round
// will sign the message returned by the config.FirstRound() function. If the
// genesis time specified in the group is already passed, Start returns an
// error. In that case, if the group is already running, you should call
// SyncAndRun().
// Round 0 = genesis seed - fixed
// Round 1 starts at genesis time, and is signing over the genesis seed
func (h *Handler) Start() error {
	if h.conf.Clock.Now().Unix() > h.conf.Group.GenesisTime {
		h.l.Errorw("", "genesis_time", "past", "call", "catchup")
		return errors.New("beacon: genesis time already passed. Call Catchup()")
	}

	h.Lock()
	// XXX: do we really need both started and running?
	h.started = true
	h.Unlock()

	_, tTime := chain.NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	h.l.Infow("", "beacon", "start", "scheme", h.crypto.Name)
	go h.run(tTime)

	return nil
}

// Catchup waits the next round's time to participate. This method is called
// when a node stops its daemon (maintenance or else) and get backs in the
// already running network . If the node does not have the previous randomness,
// it sync its local chain with other nodes to be able to participate in the
// next upcoming round.
func (h *Handler) Catchup() {
	h.Lock()
	h.started = true
	h.Unlock()

	nRound, tTime := chain.NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	go h.run(tTime)
	h.chain.RunSync(nRound, nil)
}

// Transition makes this beacon continuously sync until the time written in the
// "TransitionTime" in the handler's group file, where he will start generating
// randomness. To sync, he contacts the nodes listed in the previous group file
// given.
func (h *Handler) Transition(prevGroup *key.Group) error {
	targetTime := h.conf.Group.TransitionTime
	tRound := chain.CurrentRound(targetTime, h.conf.Group.Period, h.conf.Group.GenesisTime)
	tTime := chain.TimeOfRound(h.conf.Group.Period, h.conf.Group.GenesisTime, tRound)
	if tTime != targetTime {
		h.l.Fatalw("", "transition_time", "invalid_offset", "expected_time", tTime, "got_time", targetTime)
		return nil
	}

	h.Lock()
	h.started = true
	h.Unlock()

	go h.run(targetTime)

	// we run the sync up until (inclusive) one round before the transition
	h.l.Debugw("", "new_node", "following chain", "to_round", tRound-1)
	h.chain.RunSync(tRound-1, toPeers(prevGroup.Nodes))

	return nil
}

// TransitionNewGroup prepares the node to transition to the new group
func (h *Handler) TransitionNewGroup(newShare *key.Share, newGroup *key.Group) {
	if h == nil {
		return
	}
	targetTime := newGroup.TransitionTime
	tRound := chain.CurrentRound(targetTime, h.conf.Group.Period, h.conf.Group.GenesisTime)
	tTime := chain.TimeOfRound(h.conf.Group.Period, h.conf.Group.GenesisTime, tRound)
	if tTime != targetTime {
		h.l.Fatalw("", "transition_time", "invalid_offset", "expected_time", tTime, "got_time", targetTime)
		return
	}
	h.l.Infow("", "transition", "new_group", "at_round", tRound)
	// register a callback such that when the round happening just before the
	// transition is stored, then it switches the current share to the new one
	targetRound := tRound - 1
	h.chain.AddCallback("transition", func(b *chain.Beacon) {
		if b.Round < targetRound {
			return
		}
		h.crypto.SetInfo(newGroup, newShare)
		h.chain.RemoveCallback("transition")
	})
}

func (h *Handler) IsStarted() bool {
	h.Lock()
	defer h.Unlock()

	return h.started
}

func (h *Handler) IsServing() bool {
	h.Lock()
	defer h.Unlock()

	return h.serving
}

func (h *Handler) IsRunning() bool {
	h.Lock()
	defer h.Unlock()

	return h.running
}

func (h *Handler) IsStopped() bool {
	h.Lock()
	defer h.Unlock()

	return h.stopped
}

func (h *Handler) Reset() {
	h.Lock()
	defer h.Unlock()

	h.stopped = false
	h.started = false
	h.running = false
	h.serving = false
}

// run will wait until it is supposed to start
func (h *Handler) run(startTime int64) {
	chanTick := h.ticker.ChannelAt(startTime)
	h.l.Debugw("", "run_round", "wait", "until", startTime)

	var current roundInfo
	setRunning := sync.Once{}

	h.Lock()
	// XXX: do we really need both started and running?
	h.running = true
	h.Unlock()

	for {
		select {
		case current = <-chanTick:

			setRunning.Do(func() {
				h.Lock()
				h.serving = true
				h.Unlock()
			})

			lastBeacon, err := h.chain.Last(context.Background())
			if err != nil {
				h.l.Errorw("", "beacon_loop", "loading_last", "err", err)
				break
			}
			h.l.Debugw("", "beacon_loop", "new_round", "round", current.round, "lastbeacon", lastBeacon.Round)
			h.broadcastNextPartial(current, lastBeacon)
			// if the next round of the last beacon we generated is not the round we
			// are now, that means there is a gap between the two rounds. In other
			// words, the chain has halted for that amount of rounds or our
			// network is not functioning properly.
			if lastBeacon.Round+1 < current.round {
				// We also launch a sync with the other nodes. If there is one node
				// that has a higher beacon, we'll build on it next epoch. If
				// nobody has a higher beacon, then this one will be next if the
				// network conditions allow for it.
				// XXX find a way to start the catchup as soon as the runsync is
				// done. Not critical but leads to faster network recovery.
				h.l.Debugw("", "beacon_loop", "run_sync_catchup", "last_is", lastBeacon, "should_be", current.round)
				h.chain.RunSync(current.round, nil)
			}
		case b := <-h.chain.AppendedBeaconNoSync():
			h.l.Debugw("", "beacon_loop", "catchupmode", "last_is", b.Round, "current", current.round, "catchup_launch", b.Round < current.round)
			if b.Round < current.round {
				// When network is down, all alive nodes will broadcast their
				// signatures periodically with the same period. As soon as one
				// new beacon is created,i.e. network is up again, this channel
				// will be triggered and we enter fast mode here.
				// Since that last node is late, nodes must now hurry up to do
				// the next beacons in time -> we run the next beacon now
				// already. If that next beacon is created soon after, this
				// channel will trigger again etc until we arrive at the correct
				// round.
				go func(c roundInfo, latest *chain.Beacon) {
					h.l.Debugw("sleeping now", "beacon_loop", "catchupmode",
						"last_is", latest.Round,
						"sleep_for", h.conf.Group.CatchupPeriod)
					h.conf.Clock.Sleep(h.conf.Group.CatchupPeriod)
					h.l.Debugw("broadcast next partial", "beacon_loop", "catchupmode",
						"last_is", latest.Round)
					h.broadcastNextPartial(c, latest)
				}(current, b)
			}
		case <-h.close:
			h.l.Debugw("", "beacon_loop", "finished")
			return
		}
	}
}

func (h *Handler) broadcastNextPartial(current roundInfo, upon *chain.Beacon) {
	ctx := context.Background()
	previousSig := upon.Signature
	round := upon.Round + 1
	beaconID := commonutils.GetCanonicalBeaconID(h.conf.Group.ID)
	if current.round == upon.Round {
		h.l.Debugw("broadcastNextPartial re-broadcasting already stored beacon", "round", current.round)
		// we already have the beacon of the current round for some reasons - on
		// CI it happens due to time shifts -
		// the spec says we should broadcast the current round at the correct
		// tick so we still broadcast a partial signature over it - even though
		// drand guarantees a threshold of nodes already have it
		previousSig = upon.PreviousSig
		round = current.round
	}

	msg := h.crypto.DigestBeacon(&chain.Beacon{
		Round:       round,
		PreviousSig: previousSig,
	})

	currSig, err := h.crypto.SignPartial(msg)
	if err != nil {
		h.l.Fatal("beacon_round", "err creating signature", "err", err, "round", round)
		return
	}
	h.l.Debugw("", "broadcast_partial", round, "from_prev_sig", shortSigStr(previousSig), "msg_sign", shortSigStr(msg))
	metadata := common.NewMetadata(h.version.ToProto())
	metadata.BeaconID = beaconID

	packet := &proto.PartialBeaconPacket{
		Round:             round,
		PreviousSignature: previousSig,
		PartialSig:        currSig,
		Metadata:          metadata,
	}

	h.chain.NewValidPartial(h.addr, packet)
	for _, id := range h.crypto.GetGroup().Nodes {
		idt := id.Identity
		if h.addr == id.Address() {
			continue
		}
		go func(i *key.Identity) {
			h.l.Debugw("", "beacon_round", round, "send_to", i.Address())
			err := h.client.PartialBeacon(ctx, i, packet)
			if err != nil {
				h.l.Errorw("", "beacon_round", round, "err_request", err, "from", i.Address())
				if strings.Contains(err.Error(), errOutOfRound) {
					h.l.Errorw("", "beacon_round", round, "node", i.Addr, "reply", "out-of-round")
				}
				return
			}
		}(idt)
	}
}

// Stop the beacon loop from aggregating  further randomness, but it
// finishes the one it is aggregating currently.
func (h *Handler) Stop() {
	h.Lock()
	defer h.Unlock()
	if h.stopped {
		return
	}
	close(h.close)

	h.ticker.Stop()
	h.chain.Stop()

	h.stopped = true
	h.running = false
	h.l.Infow("beacon handler stopped", "time", h.conf.Clock.Now())
}

// StopAt will stop the handler at the given time. It is useful when
// transitioning for a resharing.
func (h *Handler) StopAt(stopTime int64) error {
	now := h.conf.Clock.Now().Unix()

	if stopTime <= now {
		// actually we can stop in the present but with "Stop"
		return errors.New("can't stop in the past or present")
	}
	duration := time.Duration(stopTime-now) * time.Second

	h.l.Debugw("", "stop_at", stopTime, "sleep_for", duration.Seconds())
	h.conf.Clock.Sleep(duration)
	h.Stop()
	return nil
}

// AddCallback is a proxy method to register a callback on the backend store
func (h *Handler) AddCallback(id string, fn func(*chain.Beacon)) {
	h.chain.AddCallback(id, fn)
}

// RemoveCallback is a proxy method to remove a callback on the backend store
func (h *Handler) RemoveCallback(id string) {
	h.chain.RemoveCallback(id)
}

// GetConfg returns the conf used by the handler
func (h *Handler) GetConfg() *Config {
	return h.conf
}

// ValidateChain asks the chain store to ask the sync manager to check the chain store up to the given beacon,
// in order to find invalid beacons and it returns the list of round numbers for which the beacons
// were corrupted / invalid / not found in the store.
// Note: it does not attempt to correct or fetch these faulty beacons.
func (h *Handler) ValidateChain(ctx context.Context, upTo uint64, cb func(r, u uint64)) ([]uint64, error) {
	return h.chain.ValidateChain(ctx, upTo, cb)
}

// CorrectChain tells the sync manager to fetch the invalid beacon from its peers.
func (h *Handler) CorrectChain(ctx context.Context, faultyBeacons []uint64, peers []net.Peer, cb func(r, u uint64)) error {
	return h.chain.RunReSync(ctx, faultyBeacons, peers, cb)
}

func shortSigStr(sig []byte) string {
	max := 3
	if len(sig) < max {
		max = len(sig)
	}
	return hex.EncodeToString(sig[0:max])
}
