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
	syncChain "github.com/drand/drand/chain/sync"
	commonutils "github.com/drand/drand/common"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/common"
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

//nolint:gocritic
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
	chain    *chainStore
	ticker   *ticker
	verifier *chain.Verifier
	beats    syncChain.Heartbeat

	close   chan bool
	addr    string
	started bool
	running bool
	serving bool
	stopped bool
	l       log.Logger
	version commonutils.Version
}

// NewHandler returns a fresh handler ready to serve and create randomness
// beacon
func NewHandler(c net.PrivateGateway, s chain.Store, conf *Config, l log.Logger, version commonutils.Version) (*Handler, error) {
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
	verifier := chain.NewVerifier(conf.Group.Scheme)
	beats := syncChain.NewHeartbeat(&syncChain.HeartbeatConfig{
		Log:    l,
		Clock:  conf.Clock,
		Client: c,
		// TODO
		Frequency: time.Duration(1 * time.Second),
	})

	handler := &Handler{
		conf:     conf,
		beats:    beats,
		client:   c,
		crypto:   crypto,
		chain:    store,
		verifier: verifier,
		ticker:   ticker,
		addr:     addr,
		close:    make(chan bool),
		l:        logger,
		version:  version,
	}
	return handler, nil
}

var errOutOfRound = "out-of-round beacon request"

// ProcessPartialBeacon receives a request for a beacon partial signature. It
// forwards it to the round manager if it is a valid beacon.
func (h *Handler) ProcessPartialBeacon(c context.Context, p *proto.PartialBeaconPacket) (*proto.Empty, error) {
	beaconID := h.conf.Group.ID
	addr := net.RemoteAddress(c)
	h.l.Debugw("", "beacon_id", beaconID, "received", "request", "from", addr, "round", p.GetRound())

	nextRound, _ := chain.NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	currentRound := nextRound - 1

	// we allow one round off in the future because of small clock drifts
	// possible, if a node receives a packet very fast just before his local
	// clock passed to the next round
	if p.GetRound() > nextRound {
		h.l.Errorw("", "beacon_id", beaconID, "process_partial", addr, "invalid_future_round", p.GetRound(), "current_round", currentRound)
		return nil, fmt.Errorf("invalid round: %d instead of %d", p.GetRound(), currentRound)
	}

	msg := h.verifier.DigestMessage(p.GetRound(), p.GetPreviousSig())

	// XXX Remove that evaluation - find another way to show the current dist.
	// key being used
	shortPub := h.crypto.GetPub().Eval(1).V.String()[14:19]
	// verify if request is valid
	if err := key.Scheme.VerifyPartial(h.crypto.GetPub(), msg, p.GetPartialSig()); err != nil {
		h.l.Errorw("", "beacon_id", beaconID,
			"process_partial", addr, "err", err,
			"prev_sig", shortSigStr(p.GetPreviousSig()),
			"curr_round", currentRound,
			"msg_sign", shortSigStr(msg),
			"short_pub", shortPub)
		return nil, err
	}
	h.l.Debugw("", "beacon_id", beaconID,
		"process_partial", addr,
		"prev_sig", shortSigStr(p.GetPreviousSig()),
		"curr_round", currentRound, "msg_sign",
		shortSigStr(msg), "short_pub", shortPub,
		"status", "OK")
	idx, _ := key.Scheme.IndexOf(p.GetPartialSig())
	if idx == h.crypto.Index() {
		h.l.Errorw("", "beacon_id", beaconID,
			"process_partial", addr,
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
	beaconID := h.conf.Group.ID
	if h.conf.Clock.Now().Unix() > h.conf.Group.GenesisTime {
		h.l.Errorw("", "beacon_id", beaconID, "genesis_time", "past", "call", "catchup")
		return errors.New("beacon: genesis time already passed. Call Catchup()")
	}

	h.Lock()
	h.started = true
	h.Unlock()

	_, tTime := chain.NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	h.l.Infow("", "beacon_id", beaconID, "beacon", "start")
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
	h.chain.RunSync(context.Background(), nRound, nil)
}

// Transition makes this beacon continuously sync until the time written in the
// "TransitionTime" in the handler's group file, where he will start generating
// randomness. To sync, he contact the nodes listed in the previous group file
// given.
func (h *Handler) Transition(prevGroup *key.Group) error {
	beaconID := h.conf.Group.ID
	targetTime := h.conf.Group.TransitionTime
	tRound := chain.CurrentRound(targetTime, h.conf.Group.Period, h.conf.Group.GenesisTime)
	tTime := chain.TimeOfRound(h.conf.Group.Period, h.conf.Group.GenesisTime, tRound)
	if tTime != targetTime {
		h.l.Fatalw("", "beacon_id", beaconID, "transition_time", "invalid_offset", "expected_time", tTime, "got_time", targetTime)
		return nil
	}

	h.Lock()
	h.started = true
	h.Unlock()

	go h.run(targetTime)

	// we run the sync up until (inclusive) one round before the transition
	h.l.Debugw("", "beacon_id", beaconID, "new_node", "following chain", "to_round", tRound-1)
	h.chain.RunSync(context.Background(), tRound-1, toPeers(prevGroup.Nodes))

	return nil
}

// TransitionNewGroup prepares the node to transition to the new group
func (h *Handler) TransitionNewGroup(newShare *key.Share, newGroup *key.Group) {
	beaconID := h.conf.Group.ID
	targetTime := newGroup.TransitionTime
	tRound := chain.CurrentRound(targetTime, h.conf.Group.Period, h.conf.Group.GenesisTime)
	tTime := chain.TimeOfRound(h.conf.Group.Period, h.conf.Group.GenesisTime, tRound)
	if tTime != targetTime {
		h.l.Fatalw("", "beacon_id", beaconID, "transition_time", "invalid_offset", "expected_time", tTime, "got_time", targetTime)
		return
	}
	h.l.Debugw("", "beacon_id", beaconID, "transition", "new_group", "at_round", tRound)
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
	beaconID := h.conf.Group.ID
	chanTick := h.ticker.ChannelAt(startTime)
	h.l.Debugw("", "beacon_id", beaconID, "run_round", "wait", "until", startTime)

	var current roundInfo
	setRunning := sync.Once{}

	h.Lock()
	h.running = true
	h.Unlock()
	var highestSeenRound uint64
	var catchupMode bool
	for {
		select {
		case beat := <-h.beats.Beats():
			if beat.LastRound > highestSeenRound {
				highestSeenRound = beat.LastRound
			}
		case current = <-chanTick:
			setRunning.Do(func() {
				h.Lock()
				h.serving = true
				h.Unlock()
			})

			lastBeacon, err := h.chain.Last()
			if err != nil {
				h.l.Errorw("", "beacon_id", beaconID, "beacon_loop", "loading_last", "err", err)
				break
			}
			h.l.Debugw("", "beacon_id", beaconID, "beacon_loop", "new_round", "round", current.round, "lastbeacon", lastBeacon.Round)
			h.broadcastNextPartial(current, lastBeacon)
			if lastBeacon.Round+1 < current.round {
				// now we look if WE are late or the NETWORK is late
				if highestSeenRound > lastBeacon.Round {
					catchupMode = false
					// WE are latethen run sync. Once we are caught up with
					// others, then we'll switch to catchup mode eventually.
					go h.chain.RunSync(context.Background(), current.round, nil)
				} else {
					// the network is late, then we run catchup mode
					// we already broadcasted for this round so we can wait the
					// next time the catchup period beat wakes up
					catchupMode = true
				}
			} else {
				// always set it to false
				catchupMode = false
			}
		case <-h.conf.Clock.After(h.conf.Group.CatchupPeriod):
			if catchupMode {
				lastBeacon, err := h.chain.Last()
				if err != nil {
					h.l.Errorw("", "beacon_id", beaconID, "beacon_loop", "loading_last", "err", err)
					break
				}
				// we can pass 0 here since we're late in schedule anyway so we wont
				// enter the edge case of already having the current round
				h.broadcastNextPartial(roundInfo{0, 0}, lastBeacon)
			}
		case <-h.close:
			h.l.Debugw("", "beacon_id", beaconID, "beacon_loop", "finished")
			return
		}
	}
}

func (h *Handler) broadcastNextPartial(current roundInfo, upon *chain.Beacon) {
	ctx := context.Background()
	previousSig := upon.Signature
	round := upon.Round + 1
	beaconID := h.conf.Group.ID
	if current.round == upon.Round {
		// we already have the beacon of the current round for some reasons - on
		// CI it happens due to time shifts -
		// the spec says we should broadcast the current round at the correct
		// tick so we still broadcast a partial signature over it - even though
		// drand guarantees a threshold of nodes already have it
		previousSig = upon.PreviousSig
		round = current.round
	}

	msg := h.verifier.DigestMessage(round, previousSig)

	currSig, err := h.crypto.SignPartial(msg)
	if err != nil {
		h.l.Fatal("beacon_id", beaconID, "beacon_round", "err creating signature", "err", err, "round", round)
		return
	}
	h.l.Debugw("", "beacon_id", beaconID, "broadcast_partial", round, "from_prev_sig", shortSigStr(previousSig), "msg_sign", shortSigStr(msg))
	metadata := common.NewMetadata(h.version.ToProto())
	metadata.BeaconID = beaconID

	packet := &proto.PartialBeaconPacket{
		Round:       round,
		PreviousSig: previousSig,
		PartialSig:  currSig,
		Metadata:    metadata,
	}

	h.chain.NewValidPartial(h.addr, packet)
	for _, id := range h.crypto.GetGroup().Nodes {
		if h.addr == id.Address() {
			continue
		}
		go func(i *key.Identity) {
			h.l.Debugw("", "beacon_id", beaconID, "beacon_round", round, "send_to", i.Address())
			err := h.client.PartialBeacon(ctx, i, packet)
			if err != nil {
				h.l.Errorw("", "beacon_id", beaconID, "beacon_round", round, "err_request", err, "from", i.Address())
				if strings.Contains(err.Error(), errOutOfRound) {
					h.l.Errorw("", "beacon_id", beaconID, "beacon_round", round, "node", i.Addr, "reply", "out-of-round")
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

	beaconID := h.conf.Group.ID
	h.stopped = true
	h.running = false
	h.l.Infow("", "beacon_id", beaconID, "beacon", "stop")
}

// StopAt will stop the handler at the given time. It is useful when
// transitionining for a resharing.
func (h *Handler) StopAt(stopTime int64) error {
	now := h.conf.Clock.Now().Unix()
	beaconID := h.conf.Group.ID

	if stopTime <= now {
		// actually we can stop in the present but with "Stop"
		return errors.New("can't stop in the past or present")
	}
	duration := time.Duration(stopTime-now) * time.Second

	h.l.Debugw("", "beacon_id", beaconID, "stop_at", stopTime, "sleep_for", duration.Seconds())
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
