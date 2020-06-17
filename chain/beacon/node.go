package beacon

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
	proto "github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/share"
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
	// Callback to use when a new beacon is created - can be nil and new
	// callbacks can be added afterwards to the beacon
	Callback func(*chain.Beacon)
}

// Handler holds the logic to initiate, and react to the TBLS protocol. Each time
// a full signature can be recosntructed, it saves it to the given Store.
type Handler struct {
	sync.Mutex
	conf *Config
	// to communicate with other drand peers
	client net.ProtocolClient
	// keeps the cryptographic info (group share etc)
	safe *cryptoSafe
	// main logic that treats incoming packet / new beacons created
	chain  *chainStore
	ticker *ticker

	close     chan bool
	addr      string
	started   bool
	stopped   bool
	l         log.Logger
	callbacks *chain.CallbackStore
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
	safe := newCryptoSafe()
	safe.SetInfo(conf.Share, node, conf.Group)
	// genesis block at round 0, next block at round 1
	// THIS is to change when one network wants to build on top of another
	// network's chain. Note that if present it overwrites.
	b := &chain.Beacon{
		Signature: conf.Group.GetGenesisSeed(),
		Round:     0,
	}
	if err := s.Put(b); err != nil {
		return nil, err
	}
	ticker := newTicker(conf.Clock, conf.Group.Period, conf.Group.GenesisTime)
	callbacks := chain.NewCallbackStore(s)
	store := newChainStore(logger, c, safe, callbacks, ticker)
	handler := &Handler{
		conf:      conf,
		client:    c,
		safe:      safe,
		chain:     store,
		ticker:    ticker,
		addr:      addr,
		close:     make(chan bool),
		l:         logger,
		callbacks: callbacks,
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
	info, err := h.safe.GetInfo(p.GetRound())
	if err != nil {
		h.l.Error("process_partial", addr, "no_info_for_round", p.GetRound())
		return nil, errors.New("no info for this round")
	}

	// XXX Remove that evaluation - find another way to show the current dist.
	// key being used
	shortPub := info.pub.Eval(1).V.String()[14:19]
	// verify if request is valid
	if err := key.Scheme.VerifyPartial(info.pub, msg, p.GetPartialSig()); err != nil {
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
	if idx == info.index {
		h.l.Error("process_partial", addr,
			"index_got", idx,
			"index_our", info.index,
			"advance_packet", p.GetRound(),
			"safe", h.safe.String(),
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
	h.chain.RunSync(context.Background())
	_, tTime := chain.NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	go h.run(tTime)
}

// Transition makes this beacon continuously sync until the time written in the
// "TransitionTime" in the handler's group file, where he will start generating
// randomness. To sync, he contact the nodes listed in the previous group file
// given.
// TODO: it should be better to use the public streaming API but since it is
// likely to change, right now we use the sync API. Later on when API is well
// defined, best to use streaming.
func (h *Handler) Transition(prevGroup *key.Group) error {
	targetTime := h.conf.Group.TransitionTime
	tRound := chain.CurrentRound(targetTime, h.conf.Group.Period, h.conf.Group.GenesisTime)
	tTime := chain.TimeOfRound(h.conf.Group.Period, h.conf.Group.GenesisTime, tRound)
	if tTime != targetTime {
		h.l.Fatal("transition_time", "invalid_offset", "expected_time", tTime, "got_time", targetTime)
		return nil
	}

	// register the previous group as well in case it needs to verify the
	// previous entries
	h.safe.SetInfo(nil, h.conf.Public, prevGroup)
	go h.run(targetTime)
	h.chain.RunSync(context.Background())
	return nil
}

// TransitionNewGroup begins transition the crypto share to a new group
func (h *Handler) TransitionNewGroup(newShare *key.Share, newGroup *key.Group) {
	targetTime := newGroup.TransitionTime
	tRound := chain.CurrentRound(targetTime, h.conf.Group.Period, h.conf.Group.GenesisTime)
	tTime := chain.TimeOfRound(h.conf.Group.Period, h.conf.Group.GenesisTime, tRound)
	if tTime != targetTime {
		h.l.Fatal("transition_time", "invalid_offset", "expected_time", tTime, "got_time", targetTime)
		return
	}
	h.l.Debug("transition", "new_group", "at_round", tRound)
	h.safe.SetInfo(newShare, h.conf.Public, newGroup)
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
				h.l.Debug("beacon_loop", "run_sync", "potential_catchup")
				// XXX Find a way to not run sync again before another has
				// finished - maybe merge with chain sync mechanism
				go h.chain.RunSync(context.Background())
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
				h.broadcastNextPartial(current, b)
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
	info, err := h.safe.GetInfo(round)
	if err != nil {
		h.l.Error("no_info", round, "BUG", h.safe.String())
		return
	}
	if info.share == nil {
		h.l.Error("no_share", round, "BUG", h.safe.String())
		return
	}
	msg := chain.Message(round, previousSig)
	currSig, err := key.Scheme.Sign(info.share.PrivateShare(), msg)
	if err != nil {
		h.l.Fatal("beacon_round", "err creating signature", "err", err, "round", round)
		return
	}
	shortPub := info.pub.Eval(1).V.String()[14:19]
	h.l.Debug("broadcast_partial", round, "from_prev_sig", shortSigStr(previousSig), "msg_sign", shortSigStr(msg), "short_pub", shortPub)
	packet := &proto.PartialBeaconPacket{
		Round:       round,
		PreviousSig: previousSig,
		PartialSig:  currSig,
	}
	h.chain.NewValidPartial(h.addr, packet)
	for _, id := range info.group.Nodes {
		if info.id.Address() == id.Address() {
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

// AddCallback registers a handler to be notified with new beacons
func (h *Handler) AddCallback(fn func(*chain.Beacon)) {
	h.callbacks.AddCallback(fn)
}

func shortSigStr(sig []byte) string {
	max := 3
	if len(sig) < max {
		max = len(sig)
	}
	return hex.EncodeToString(sig[0:max])
}

func shuffleNodes(nodes []*key.Node) []*key.Node {
	ids := make([]*key.Node, 0, len(nodes))
	ids = append(ids, nodes...)
	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	return ids
}

type cryptoInfo struct {
	group   *key.Group
	share   *key.Share
	pub     *share.PubPoly
	startAt uint64
	id      *key.Node
	index   int
}

type cryptoSafe struct {
	sync.Mutex
	infos []*cryptoInfo
}

func newCryptoSafe() *cryptoSafe {
	return &cryptoSafe{}
}

func (c *cryptoSafe) SetInfo(keyShare *key.Share, id *key.Node, group *key.Group) {
	c.Lock()
	defer c.Unlock()
	info := new(cryptoInfo)
	info.id = id
	info.group = group
	info.pub = group.PublicKey.PubPoly()
	if keyShare != nil {
		info.share = keyShare
		info.index = keyShare.Share.I
	} else {
		info.index = int(id.Index)
	}
	if group.TransitionTime != 0 {
		t := group.TransitionTime
		info.startAt = chain.CurrentRound(t, group.Period, group.GenesisTime)
	} else {
		// group started at genesis time
		info.startAt = 0
	}
	c.infos = append(c.infos, info)
	// we sort reverse order so highest round are first
	sort.Slice(c.infos, func(i, j int) bool { return c.infos[i].startAt > c.infos[j].startAt })
}

func (c *cryptoSafe) GetInfo(round uint64) (*cryptoInfo, error) {
	c.Lock()
	defer c.Unlock()
	for _, info := range c.infos {
		if round >= info.startAt {
			return info, nil
		}
	}
	fmt.Printf("failed infos for round %d: %+v\n", round, c.infos)
	return nil, fmt.Errorf("no group info for round %d", round)
}

func (c *cryptoSafe) String() string {
	c.Lock()
	defer c.Unlock()
	var out string
	for _, info := range c.infos {
		var index = -1
		if info.share != nil {
			index = info.share.Share.I
		}
		shortPub := info.pub.Eval(1).V.String()[14:19]
		out += fmt.Sprintf(" {startAt: %d, index: %d, pub: %s} ", info.startAt, index, shortPub)
	}
	return out
}
