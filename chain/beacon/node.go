package beacon

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
	proto "github.com/drand/drand/protobuf/drand"
	clock "github.com/jonboulle/clockwork"

	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
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

// Handler holds the logic to initiate, and react to the TBLS protocol. Each time
// a full signature can be recosntructed, it saves it to the given Store.
type Handler struct {
	sync.Mutex
	conf *Config
	// to communicate with other drand peers
	client net.ProtocolClient
	// keeps the cryptographic info (group share etc)
	crypto *cryptoStore
	// main logic that treats incoming packet / new beacons created
	chain  *chainStore
	ticker *ticker

	close   chan bool
	addr    string
	started bool
	stopped bool
	l       log.Logger
}

// NewHandler returns a fresh handler ready to serve and create randomness
// beacon
func NewHandler(c net.ProtocolClient, s chain.Store, conf *Config, l log.Logger) (*Handler, error) {
	if conf.Share == nil || conf.Group == nil {
		return nil, errors.New("beacon: invalid configuration")
	}
	// Checking we are in the group
	node := conf.Group.Find(conf.Public.Identity)
	if node == nil {
		return nil, errors.New("beacon: keypair not included in the given group")
	}
	addr := conf.Public.Address()
	logger := l
	crypto := newCryptoStore(conf.Group, conf.Share)
	// insert genesis beacon
	if err := s.Put(chain.GenesisBeacon(crypto.chain)); err != nil {
		return nil, err
	}

	ticker := newTicker(conf.Clock, conf.Group.Period, conf.Group.GenesisTime)
	store := newChainStore(logger, conf, c, crypto, s, ticker)
	handler := &Handler{
		conf:   conf,
		client: c,
		crypto: crypto,
		chain:  store,
		ticker: ticker,
		addr:   addr,
		close:  make(chan bool),
		l:      logger,
	}
	return handler, nil
}

var errOutOfRound = "out-of-round beacon request"

// ProcessPartialBeacon receives a request for a beacon partial signature. It
// forwards it to the round manager if it is a valid beacon.
func (h *Handler) ProcessPartialBeacon(c context.Context, p *proto.PartialBeaconPacket) (*proto.Empty, error) {
	addr := net.RemoteAddress(c)
	h.l.Debug("received", "request", "from", addr, "round", p.GetRound())

	nextRound, _ := chain.NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	currentRound := nextRound - 1

	// we allow one round off in the future because of small clock drifts
	// possible, if a node receives a packet very fast just before his local
	// clock passed to the next round
	if p.GetRound() > nextRound {
		h.l.Error("process_partial", addr, "invalid_future_round", p.GetRound(), "current_round", currentRound)
		return nil, fmt.Errorf("invalid round: %d instead of %d", p.GetRound(), currentRound)
	}

	msg := chain.Message(p.GetRound(), p.GetPreviousSig())
	// XXX Remove that evaluation - find another way to show the current dist.
	// key being used
	shortPub := h.crypto.GetPub().Eval(1).V.String()[14:19]
	// verify if request is valid
	if err := key.Scheme.VerifyPartial(h.crypto.GetPub(), msg, p.GetPartialSig()); err != nil {
		h.l.Error("process_partial", addr, "err", err,
			"prev_sig", shortSigStr(p.GetPreviousSig()),
			"curr_round", currentRound,
			"msg_sign", shortSigStr(msg),
			"short_pub", shortPub)
		return nil, err
	}
	h.l.Debug("process_partial", addr,
		"prev_sig", shortSigStr(p.GetPreviousSig()),
		"curr_round", currentRound, "msg_sign",
		shortSigStr(msg), "short_pub", shortPub,
		"status", "OK")
	idx, _ := key.Scheme.IndexOf(p.GetPartialSig())
	if idx == h.crypto.Index() {
		h.l.Error("process_partial", addr,
			"index_got", idx,
			"index_our", h.crypto.Index(),
			"advance_packet", p.GetRound(),
			"pub", shortPub)
		// XXX error or not ?
		return new(proto.Empty), nil
	}
	h.chain.NewValidPartial(addr, p)
	return new(proto.Empty), nil
}

// Store returns the store associated with this beacon handler
func (h *Handler) Store() chain.Store {
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
		h.l.Error("genesis_time", "past", "call", "catchup")
		return errors.New("beacon: genesis time already passed. Call Catchup()")
	}
	_, tTime := chain.NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	h.l.Info("beacon", "start")
	go h.run(tTime)
	return nil
}

// Catchup waits the next round's time to participate. This method is called
// when a node stops its daemon (maintenance or else) and get backs in the
// already running network . If the node does not have the previous randomness,
// it sync its local chain with other nodes to be able to participate in the
// next upcoming round.
func (h *Handler) Catchup() {
	nRound, tTime := chain.NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	go h.run(tTime)
	h.chain.RunSync(context.Background(), nRound, nil)
}

// Transition makes this beacon continuously sync until the time written in the
// "TransitionTime" in the handler's group file, where he will start generating
// randomness. To sync, he contact the nodes listed in the previous group file
// given.
func (h *Handler) Transition(prevGroup *key.Group) error {
	targetTime := h.conf.Group.TransitionTime
	tRound := chain.CurrentRound(targetTime, h.conf.Group.Period, h.conf.Group.GenesisTime)
	tTime := chain.TimeOfRound(h.conf.Group.Period, h.conf.Group.GenesisTime, tRound)
	if tTime != targetTime {
		h.l.Fatal("transition_time", "invalid_offset", "expected_time", tTime, "got_time", targetTime)
		return nil
	}
	go h.run(targetTime)
	// we run the sync up until (inclusive) one round before the transition
	h.l.Debug("new_node", "following chain", "to_round", tRound-1)
	h.chain.RunSync(context.Background(), tRound-1, toPeers(prevGroup.Nodes))
	return nil
}

// TransitionNewGroup prepares the node to transition to the new group
func (h *Handler) TransitionNewGroup(newShare *key.Share, newGroup *key.Group) {
	targetTime := newGroup.TransitionTime
	tRound := chain.CurrentRound(targetTime, h.conf.Group.Period, h.conf.Group.GenesisTime)
	tTime := chain.TimeOfRound(h.conf.Group.Period, h.conf.Group.GenesisTime, tRound)
	if tTime != targetTime {
		h.l.Fatal("transition_time", "invalid_offset", "expected_time", tTime, "got_time", targetTime)
		return
	}
	h.l.Debug("transition", "new_group", "at_round", tRound)
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

// run will wait until it is supposed to start
func (h *Handler) run(startTime int64) {
	chanTick := h.ticker.ChannelAt(startTime)
	h.l.Debug("run_round", "wait", "until", startTime)
	var current roundInfo
	for {
		select {
		case current = <-chanTick:
			lastBeacon, err := h.chain.Last()
			if err != nil {
				h.l.Error("beacon_loop", "loading_last", "err", err)
				break
			}
			h.l.Debug("beacon_loop", "new_round", "round", current.round, "lastbeacon", lastBeacon.Round)
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
				h.l.Debug("beacon_loop", "run_sync_catchup", "last_is", lastBeacon, "should_be", current.round)
				go h.chain.RunSync(context.Background(), current.round, nil)
			}
		case b := <-h.chain.AppendedBeaconNoSync():
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
					h.conf.Clock.Sleep(h.conf.Group.CatchupPeriod)
					h.broadcastNextPartial(c, latest)
				}(current, b)
			}
		case <-h.close:
			h.l.Debug("beacon_loop", "finished")
			return
		}
	}
}

func (h *Handler) broadcastNextPartial(current roundInfo, upon *chain.Beacon) {
	ctx := context.Background()
	previousSig := upon.Signature
	round := upon.Round + 1
	if current.round == upon.Round {
		// we already have the beacon of the current round for some reasons - on
		// CI it happens due to time shifts -
		// the spec says we should broadcast the current round at the correct
		// tick so we still broadcast a partial signature over it - even though
		// drand guarantees a threshold of nodes already have it
		previousSig = upon.PreviousSig
		round = current.round
	}
	msg := chain.Message(round, previousSig)
	currSig, err := h.crypto.SignPartial(msg)
	if err != nil {
		h.l.Fatal("beacon_round", "err creating signature", "err", err, "round", round)
		return
	}
	h.l.Debug("broadcast_partial", round, "from_prev_sig", shortSigStr(previousSig), "msg_sign", shortSigStr(msg))
	packet := &proto.PartialBeaconPacket{
		Round:       round,
		PreviousSig: previousSig,
		PartialSig:  currSig,
	}
	h.chain.NewValidPartial(h.addr, packet)
	for _, id := range h.crypto.GetGroup().Nodes {
		if h.addr == id.Address() {
			continue
		}
		go func(i *key.Identity) {
			h.l.Debug("beacon_round", round, "send_to", i.Address())
			err := h.client.PartialBeacon(ctx, i, packet)
			if err != nil {
				h.l.Error("beacon_round", round, "err_request", err, "from", i.Address())
				if strings.Contains(err.Error(), errOutOfRound) {
					h.l.Error("beacon_round", round, "node", i.Addr, "reply", "out-of-round")
				}
				return
			}
		}(id.Identity)
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
	h.chain.Stop()
	h.ticker.Stop()
	h.stopped = true
	h.l.Info("beacon", "stop")
}

// StopAt will stop the handler at the given time. It is useful when
// transitionining for a resharing.
func (h *Handler) StopAt(stopTime int64) error {
	now := h.conf.Clock.Now().Unix()
	if stopTime <= now {
		// actually we can stop in the present but with "Stop"
		return errors.New("can't stop in the past or present")
	}
	duration := time.Duration(stopTime-now) * time.Second
	h.l.Debug("stop_at", stopTime, "sleep_for", duration.Seconds())
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

// SyncChain is a proxy method to sync a chain
func (h *Handler) SyncChain(req *proto.SyncRequest, stream proto.Protocol_SyncChainServer) error {
	return h.chain.sync.SyncChain(req, stream)
}

func shortSigStr(sig []byte) string {
	max := 3
	if len(sig) < max {
		max = len(sig)
	}
	return hex.EncodeToString(sig[0:max])
}
