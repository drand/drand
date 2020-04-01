package beacon

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	//"github.com/benbjohnson/clock"
	"github.com/drand/drand/log"
	proto "github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/sign"
	clock "github.com/jonboulle/clockwork"
	"google.golang.org/grpc/peer"

	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
)

// Config holds the different cryptographc informations necessary to run the
// randomness beacon.
type Config struct {
	// XXX Think of removing uncessary access to keypair - only given for index
	Private *key.Pair
	Share   *key.Share
	Group   *key.Group
	Scheme  sign.ThresholdScheme
	Clock   clock.Clock
}

// Handler holds the logic to initiate, and react to the TBLS protocol. Each time
// a full signature can be recosntructed, it saves it to the given Store.
type Handler struct {
	conf *Config
	// to communicate with other drand peers
	client net.ProtocolClient
	// where to store the new randomness beacon
	store Store
	// to sign beacons
	share *key.Share
	// to verify incoming beacons
	group *key.Group
	// to verify incoming beacons with tbls
	pub *share.PubPoly
	sync.Mutex

	// keeps the partial signature for the current round in check
	// It is flushed when we pass to another round
	cache *PartialCache
	// the signature of this node for the current round. acts like a cache to
	// avoid resigning it for each request.
	currentPartial *sigPair
	// last successful beacon created by the network - it's the point of
	// reference when building the next round and/or answering partial requests
	lastSuccess *Beacon

	index int

	// current round
	round uint64

	ticker  clock.Ticker
	close   chan bool
	addr    string
	started bool

	l log.Logger
}

// NewHandler returns a fresh handler ready to serve and create randomness
// beacon
func NewHandler(c net.ProtocolClient, s Store, conf *Config, l log.Logger) (*Handler, error) {
	if conf.Private == nil || conf.Share == nil || conf.Group == nil {
		return nil, errors.New("beacon: invalid configuration")
	}
	idx, exists := conf.Group.Index(conf.Private.Public)
	if !exists {
		return nil, errors.New("beacon: keypair not included in the given group")
	}

	c.SetTimeout(conf.Group.Period) // wait on each call no more than the period
	handler := &Handler{
		conf:   conf,
		client: c,
		group:  conf.Group,
		share:  conf.Share,
		pub:    conf.Share.PubPoly(),
		index:  idx,
		addr:   conf.Private.Public.Address(),
		store:  s,
		close:  make(chan bool),
		l:      l.With("index", idx),
		cache:  NewPartialCache(conf.Scheme, conf.Group.Len()),
	}
	// genesis block at round 0, next block at round 1
	// THIS is to change when one network wants to build on top of another
	// network's chain. Note that if present it overwrites.
	s.Put(&Beacon{
		Signature: conf.Group.GenesisSeed(),
		Round:     0,
	})
	return handler, nil
}

var errOutOfRound = "out-of-round beacon request"

// ProcessBeacon receives a request for a beacon partial signature. It replies
// successfully with a valid partial signature over the given beacon packet
// information if the following is true:
// 1- the round for the request is not different than the current round by a certain threshold
// 2- the partial signature in the embedded response is valid. This proves that
// the requests comes from a qualified node from the DKG phase.
func (h *Handler) ProcessBeacon(c context.Context, p *proto.BeaconRequest) (*proto.BeaconResponse, error) {
	peer, _ := peer.FromContext(c)
	h.l.Debug("received", "request", "from", peer.Addr.String())

	nextRound, _ := NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	currentRound := nextRound - 1
	if p.GetRound() != currentRound {
		// request is not for current round
		h.l.Error("request_round", p.GetRound(), "current_round", nextRound-1)
		return nil, fmt.Errorf("invalid round: %d instead of %d", p.GetRound(), nextRound-1)
	}

	var previousSig []byte
	var previousRound uint64
	if currentRound == 1 {
		// first round built over genesis
		previousSig = h.conf.Group.GenesisSeed()
		previousRound = 0
	} else {
		// load the last successful beacon and check if the request is pointing
		// to the last created beacon of this node.
		beacon := h.loadLastSucessfulRound()
		if beacon == nil {
			// we haven't even started yet
			return nil, errors.New("no beacons")
		}
		if beacon.Round != p.GetPreviousRound() {
			// this means either
			// 1. requester is lying about request - he's trying to gather
			// partial signature that builds over another round
			// 2. Or that this node is late. In such a case we must run sync
			// Since we can not know which case, we don't answer with a partial
			// request to avoid situation 1.
			// TODO: run sync
			return nil, fmt.Errorf("last round stored is not current")
		}
		// we take our own previous signature instead of trusting the requester
		previousSig = beacon.Signature
		previousRound = beacon.Round
	}

	msg := Message(previousSig, previousRound, currentRound)
	// verify if request is valid
	if err := h.conf.Scheme.VerifyPartial(h.pub, msg, p.PartialSig); err != nil {
		h.l.Error("request", err, "from", peer.Addr.String(), "prev_sig", shortSigStr(previousSig), "prev_round", previousRound, "curr_round", currentRound)
		return nil, err
	}

	// load or create signature
	partialSig, err := h.getCurrentSignature(currentRound, msg)
	if err != nil {
		return nil, errors.New("can't generate partial signature")
	}

	// XXX add signatures received in the cache
	// index is valid since signature verified before
	index, _ := h.conf.Scheme.IndexOf(p.PartialSig)

	resp := &proto.BeaconResponse{
		PartialSig: partialSig,
	}
	h.l.Debug("process_beacon", currentRound, "answered_to", index)
	return resp, err
}

// 1 week with 30s round time
const MaxSyncLenght = 20160

func (h *Handler) SyncChain(req *proto.SyncRequest, p proto.Protocol_SyncChainServer) error {
	if h.lastSuccess == nil {
		return errors.New("no beacon created yet")
	}
	to := req.GetFromRound()
	var allBeacons = []*Beacon{h.lastSuccess}
	from := allBeacons[0].Round
	if from-to > MaxSyncLenght {
		return errors.New("too long sync request")
	}
	// XXX Change store to include pointer to round that builds upon it
	// so we don't have to fetch in reverse order (or blindy if we want to go in
	// normal order)
	for from >= to {
		beacon, err := h.store.Get(from)
		if err == ErrNoBeaconSaved {
			from--
		}
		if err != nil {
			h.l.Error("loading_beacon", from)
			// XXX Should probably fatal here; it's not good
			return errors.New("error loading beacons")
		}
		from = beacon.PreviousRound
		allBeacons = append(allBeacons, beacon)
	}
	for i := len(allBeacons) - 1; i >= 0; i-- {
		beacon := allBeacons[i]
		reply := &proto.SyncResponse{
			PreviousRound: beacon.PreviousRound,
			PreviousSig:   beacon.PreviousSig,
			Round:         beacon.Round,
			Signature:     beacon.Signature,
		}
		fmt.Printf("\nnode %d - reply sync from round %d to %d\n\n", h.index, to, reply.Round)
		if err := p.Send(reply); err != nil {
			return err
		}
		to = beacon.Round
	}
	return nil
}

// Start runs the beacon protocol (threshold BLS signature). The first round
// will sign the message returned by the config.FirstRound() function. If the
// genesis time specified in the group is already passed, Start returns an
// error. In that case, if the group is already running, you should call
// SyncAndRun().
// Round 0 = genesis seed - fixed
// Round 1 starts at genesis time, and is signing over the genesis seed
func (h *Handler) Start() error {
	h.l.Info("beacon", "start")
	if h.conf.Clock.Now().Unix() > h.conf.Group.GenesisTime {
		return errors.New("beacon: genesis time already passed. Call SyncAndRun().")
	}
	genesis, err := h.store.Get(0)
	if err != nil {
		return errors.New("no genesis block found in store")
	}
	go h.run(genesis.Signature, genesis.Round, genesis.Round+1, h.conf.Group.GenesisTime)
	return nil
}

// SyncAndRun waits the next round's time to participate. This method is called
// when a node stops its daemon (maintenance or else) and get backs in the
// already running network . If the node does not have the previous randomness,
// it sync its local chain with other nodes to be able to participate in the
// next upcoming round.
func (h *Handler) SyncAndRun() error {
	prevBeacon, err := h.Sync()
	if err != nil {

	}
	nextRound, nextTime := NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	previousSig := prevBeacon.Signature
	previousRound := prevBeacon.Round
	fmt.Printf("\n SYNCING DONE: prevRound %d prevSig %s - nextRound %d nextTime %d\n\n", previousRound, shortSigStr(previousSig), nextRound, nextTime)
	go h.run(previousSig, previousRound, nextRound, nextTime)
	return nil
}

func (h *Handler) Sync() (*Beacon, error) {
	var nextRound uint64
	var nextTime int64
	var err error
	var lastBeacon *Beacon
	// only reason why trying multiple times is when the syncing takes too much
	// time and then we miss the current round, hence 2 times should be fine.
	for trial := 0; trial < 2; trial++ {
		lastBeacon, err = h.store.Last()
		if err == ErrNoBeaconSaved {
			return nil, errors.New("no genesis block stored. BUG")
		}
		if err != nil {
			return nil, err
		}
		nextRound, nextTime = NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
		if lastBeacon.Round+1 == nextRound {
			// next round will build on the one we have - no need to sync
			return lastBeacon, nil
		}
		// there is a gap - we need to sync with other peers
		currRound := lastBeacon.Round
		currSig := lastBeacon.Signature
		//fmt.Printf("\n node %d LAUNCHING SYNC from round %d -- previousBeacon.Round %d\n\n", h.index, currRound, previousBeacon.Round)
		if err := h.syncFrom(currRound, currSig); err != nil {
			h.l.Error("sync", "failed", "from", currRound)
		}
		lastBeacon = h.loadLastSucessfulRound()
		if lastBeacon == nil {
			h.l.Fatal("after_sync", "nil_beacon")
		}
	}
	h.l.Debug("sync", "done", "upto", lastBeacon.Round, "next_time", nextTime)
	return lastBeacon, nil
}

// Run starts the TBLS protocol: it will start the round "nextRound" that is
// building over the given initSig & the initRound. It sleeps until the starting
// time specified has kicked in.
func (h *Handler) run(initSig []byte, initRound, nextRound uint64, startTime int64) {
	// sleep until beginning of next round
	now := h.conf.Clock.Now().Unix()
	sleepTime := startTime - now
	h.l.Info("run_round", nextRound, "waiting_for", sleepTime)
	fmt.Printf("node %d (genesis %d) - current time %d / now %d -> startTime %d - sleeping for %d ... (clock %p)\n", h.index, h.conf.Group.GenesisTime, h.conf.Clock.Now().Unix(), now, startTime, sleepTime, h.conf.Clock)
	h.conf.Clock.Sleep(time.Duration(sleepTime) * time.Second)
	fmt.Printf("\n%d: node %d finished sleeping - time %d - starttime should be %d\n", time.Now().Unix(), h.index, h.conf.Clock.Now().Unix(), startTime)
	// start for this round already
	var goToNextRound = true
	var currentRoundFinished bool
	var currentRound uint64 = nextRound
	var prevSig []byte = initSig
	var prevRound uint64 = initRound
	var period = h.conf.Group.Period
	winCh := make(chan *Beacon)
	closingCh := make(chan bool)

	h.Lock()
	h.ticker = h.conf.Clock.NewTicker(period)
	h.started = true
	h.Unlock()
	for {
		if goToNextRound {
			fmt.Printf("\nnode %d - goToNextRound %d!\n\n", h.index, currentRound)
			// we launch the next round and close the previous operations if
			// still running
			close(winCh)
			winCh = make(chan *Beacon)
			close(closingCh)
			closingCh = make(chan bool)

			go h.runRound(currentRound, prevRound, prevSig, winCh, closingCh)

			goToNextRound = false
			currentRoundFinished = false
		}
		// that way the execution starts directly, not after *one tick*
		select {
		case <-h.ticker.Chan():
			if !currentRoundFinished {
				// the current round has not finished while the next round is
				// starting. In this case, we increase the round number but
				// still signs on the current signature.
				currentRound++
			}
			h.cache.Flush()
			h.flushCurrentSig()
			// the ticker is king so we always start a new round at each tick
			goToNextRound = true
			fmt.Printf("\n <<- node %d : NEW TICK round %d -  %d \n\n", h.index, currentRound, h.conf.Clock.Now().Unix())
			continue
		case beacon := <-winCh:
			if beacon.Round != currentRound {
				// an old round that finishes later than supposed to, we need to
				// make sure to not build upon it as other nodes may be already
				// ahead - an round that finishes after its time is not
				// considered in the chain
				continue
			}
			// we signal that the round is finished and move on by waiting on
			// the next tick,i.e. proper operational flow.
			currentRound++
			h.saveLastSuccessfulRound(beacon)
			prevSig = beacon.Signature
			prevRound = beacon.Round
			currentRoundFinished = true
			fmt.Printf("\n FINISHED node %d - round %d\n\n", h.index, prevRound)
		case <-h.close:
			return
		}
	}
}

func (h *Handler) saveLastSuccessfulRound(r *Beacon) {
	h.Lock()
	defer h.Unlock()
	h.lastSuccess = r
}

func (h *Handler) loadLastSucessfulRound() *Beacon {
	h.Lock()
	defer h.Unlock()
	return h.lastSuccess
}

func (h *Handler) runRound(currentRound, prevRound uint64, prevSig []byte, winCh chan *Beacon, closeCh chan bool) {
	// we sign for the new current round
	msg := Message(prevSig, prevRound, currentRound)
	currSig, err := h.getCurrentSignature(currentRound, msg)
	if err != nil {
		h.l.Fatal("beacon_round", fmt.Sprintf("creating signature: %s", err), "round", currentRound)
		return
	}
	h.l.Debug("start_round", currentRound, "time", h.conf.Clock.Now(), "from_sig", shortSigStr(prevSig), "from_round", prevRound, "msg_sign", shortSigStr(msg))
	request := &proto.BeaconRequest{
		Round:         currentRound,
		PreviousRound: prevRound,
		PartialSig:    currSig,
	}
	respCh := make(chan *proto.BeaconResponse, h.group.Len())
	// send all requests in parallel
	// XXX Use the cache for a smarter fetching strategy
	for _, id := range h.group.Nodes {
		if h.addr == id.Addr {
			continue
		}
		// this go routine sends the packet to one node. It will always
		// return assuming there's a timeout on the connection
		go func(i *key.Identity) {
			resp, err := h.client.NewBeacon(i, request)
			if err != nil {
				h.l.Error("beacon_round", currentRound, "err_request", err, "from", i.Address())
				if strings.Contains(err.Error(), errOutOfRound) {
					h.l.Error("beacon_round", currentRound, "node", i.Addr, "reply", "out-of-round")
				}
				return
			}
			if err := h.conf.Scheme.VerifyPartial(h.pub, msg, resp.PartialSig); err != nil {
				h.l.Error("beacon_round", currentRound, "invalid beacon resp", err)
				return
			}
			h.l.Debug("beacon_round", currentRound, "valid_resp_from", i.Address())
			respCh <- resp
		}(id)
	}
	// wait for a threshold of replies or if the timeout occured
	for h.cache.Len() < h.group.Threshold {
		select {
		case resp := <-respCh:
			h.cache.Add(resp.PartialSig)
			h.l.Debug("beacon_round", currentRound, "partial_signature", h.cache.Len(), "required", h.group.Threshold)
		case <-closeCh:
			// it's already time to go to the next, there has been not
			// enough time or nodes are too slow. In any case it's a
			// problem.
			h.l.Error("beacon_round", currentRound, "quitting prematurely", "problem with short period or beacon nodes")
			return
		}
	}
	fmt.Printf("\n%d got ALL signatures\n\n", h.index)
	finalSig, err := h.conf.Scheme.Recover(h.pub, msg, h.cache.GetAll(), h.group.Threshold, h.group.Len())
	if err != nil {
		fmt.Printf("\n%d got ALL signatures #2 - %v - msg %s\n\n", h.index, err, shortSigStr(msg))
		h.l.Error("beacon_round", currentRound, "no final beacon", err)
		return
	}

	if err := h.conf.Scheme.VerifyRecovered(h.pub.Commit(), msg, finalSig); err != nil {
		h.l.Error("beacon_round", currentRound, "invalid beacon signature", err)
		return
	}

	hash := sha512.New()
	hash.Write(finalSig)
	randomness := hash.Sum(nil)

	beacon := &Beacon{
		Round:         currentRound,
		PreviousRound: prevRound,
		PreviousSig:   prevSig,
		Signature:     finalSig,
	}
	//slog.Debugf("beacon: %s round %d -> SAVING beacon in store ", h.addr, round)
	// we can always store it even if it is too late, since it is valid anyway
	if err := h.store.Put(beacon); err != nil {
		h.l.Error("beacon_round", currentRound, "storing beacon", err)
		return
	}
	//slog.Debugf("beacon: %s round %d -> saved beacon in store sucessfully", h.addr, round)
	//slog.Infof("beacon: %s round %d finished: %x", h.addr, round, finalSig)
	shortSig := shortSigStr(finalSig)
	shortPrevSig := shortSigStr(prevSig)
	shortRand := shortSigStr(randomness)
	h.l.Info("done_round", currentRound, "signature", shortSig, "randomness", shortRand, "previous_sig", shortPrevSig)
	winCh <- beacon
}

// initRound & initSignature are the round & signature this node has
func (h *Handler) syncFrom(initRound uint64, initSignature []byte) error {
	currentRound := initRound
	fmt.Printf("\n node %d runs SYNCFROM --- currentRound %d\n\n", h.index, currentRound)
	currentSig := initSignature
	ids := make([]*key.Identity, 0, len(h.group.Nodes))
	for _, id := range h.group.Nodes {
		ids = append(ids, id)
	}
	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	for _, id := range h.group.Nodes {
		if h.addr == id.Addr {
			continue
		}
		h.l.Debug("request", "sync", "to", id.Addr, "from", currentRound+1)
		ctx, cancel := context.WithCancel(context.Background())
		request := &proto.SyncRequest{
			// we ask rounds from at least one round more than what we already
			// have
			FromRound: currentRound + 1,
		}
		respCh, err := h.client.SyncChain(ctx, id, request)
		if err != nil {
			h.l.Error("sync_from", currentRound, "error", err, "from", id.Address())
			continue
		}

		for syncReply := range respCh {
			// we only sync for increasing round numbers
			// there might be gaps so we dont check for sequentiality but our
			// chain from the round we have should be valid
			if syncReply.Round <= currentRound {
				h.l.Debug("sync_round", currentRound, "from", id.Address(), "invalid-reply")
				cancel()
				break
			}
			// we want answers consistent from our round that we have
			prevSig := syncReply.GetPreviousSig()
			prevRound := syncReply.GetPreviousRound()
			if currentRound != prevRound || !bytes.Equal(prevSig, currentSig) {
				h.l.Error("sync_round", currentRound, "from", id.Address(), "want_round", currentRound, "got_round", prevRound, "want_sig", shortSigStr(currentSig), "got_sig", shortSigStr(prevSig), "sig", shortSigStr(syncReply.GetSignature()), "round", syncReply.GetRound())
				cancel()
				break
			}
			msg := Message(prevSig, prevRound, syncReply.GetRound())
			if err := h.conf.Scheme.VerifyRecovered(h.pub.Commit(), msg, syncReply.GetSignature()); err != nil {
				h.l.Error("sync_round", currentRound, "invalid_sig", err, "from", id.Address())
				cancel()
				break
			}
			h.l.Debug("sync_round", syncReply.GetRound(), "valid_sync", id.Address())
			beacon := &Beacon{
				PreviousSig:   syncReply.GetPreviousSig(),
				PreviousRound: syncReply.GetPreviousRound(),
				Round:         syncReply.GetRound(),
				Signature:     syncReply.GetSignature(),
			}
			h.store.Put(beacon)
			h.saveLastSuccessfulRound(beacon)
			currentRound = syncReply.GetRound()
			currentSig = syncReply.GetSignature()
			// we check each time that we haven't advanced a round in the
			// syncing process
			nextRound, _ := NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
			// if it gave us the round just before the next one, then we are
			// synced!
			if currentRound+1 == nextRound {
				cancel()
				return nil
			}
		}
	}

	nextRound, _ := NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	return fmt.Errorf("syncing went from %d to %d whereas current round is %d: network is down", initRound, currentRound, nextRound-1)
}

// Stop the beacon loop from aggregating  further randomness, but it
// finishes the one it is aggregating currently.
func (h *Handler) Stop() {
	h.Lock()
	defer h.Unlock()
	if !h.started {
		return
	}
	if h.ticker != nil {
		h.ticker.Stop()
	}
	close(h.close)
	h.store.Close()
	h.started = false
	h.l.Info("beacon", "stop")
}

func (h *Handler) StopAt(stopTime int64) error {
	now := h.conf.Clock.Now().Unix()
	if stopTime <= now {
		// actually we can stop in the present but with "Stop"
		return errors.New("can't stop in the past or present")
	}
	duration := time.Duration(stopTime - now)
	go func() {
		h.conf.Clock.Sleep(duration)
		h.Stop()
	}()
	return nil
}

func (h *Handler) flushCurrentSig() {
	h.Lock()
	defer h.Unlock()
	h.currentPartial = nil
}

func (h *Handler) getCurrentSignature(round uint64, msg []byte) ([]byte, error) {
	h.Lock()
	defer h.Unlock()
	if h.currentPartial == nil || h.currentPartial.round != round {
		signature, err := h.conf.Scheme.Sign(h.share.PrivateShare(), msg)
		if err != nil {
			return nil, err
		}
		h.cache.Add(signature)
		h.currentPartial = &sigPair{
			round: round,
			sig:   signature,
		}
	}
	return h.currentPartial.sig, nil
}

func shortSigStr(sig []byte) string {
	return hex.EncodeToString(sig[0:3])
}

type sigPair struct {
	round uint64
	sig   []byte
}
