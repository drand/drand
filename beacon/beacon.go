package beacon

import (
	"bytes"
	"context"
	"crypto/sha512"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/drand/drand/log"
	proto "github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/sign"
	"google.golang.org/grpc/peer"

	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
)

// What is the maximum round difference a drand node accepts to sign
var maxRoundDelta uint64 = 2

// Config holds the different cryptographc informations necessary to run the
// randomness beacon.
type Config struct {
	// XXX Think of removing uncessary access to keypair - only given for index
	Private *key.Pair
	Share   *key.Share
	Group   *key.Group
	Scheme  sign.ThresholdScheme
	Clock   clock.Clock
	// FirstRound returns the message and the current round number to sign when
	// this beacon starts.
	FirstRound func() ([]byte, int)
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

	index int

	// current round
	round uint64
	// previous signature generated at the previous round. Useful to generate
	// the next signature on the next round.
	previousSig []byte
	// stores some recent signature to avoid recreating them
	cache *signatureCache
	// signal if a beacon node is late, it waits for the next incoming request
	// to start its own timer
	catchup bool
	// signal the beacon received from incoming request to the timer
	catchupCh chan Beacon

	ticker  *clock.Ticker
	close   chan bool
	addr    string
	seed    []byte
	started bool

	l log.Logger
}

// NewHandler returns a fresh handler ready to serve and create randomness
// beacon
func NewHandler(c net.ProtocolClient, s Store, conf *Config, l log.Logger) (*Handler, error) {
	if conf.Private == nil || conf.Share == nil || conf.Group == nil || conf.FirstRound == nil {
		return nil, errors.New("beacon: invalid configuration")
	}
	idx, exists := conf.Group.Index(conf.Private.Public)
	if !exists {
		return nil, errors.New("beacon: keypair not included in the given group")
	}

	addr := conf.Group.Nodes[idx].Addr

	c.SetTimeout(conf.Group.Period) // wait on each call no more than the period
	handler := &Handler{
		conf:      conf,
		client:    c,
		group:     conf.Group,
		share:     conf.Share,
		pub:       conf.Share.PubPoly(),
		index:     idx,
		store:     s,
		close:     make(chan bool),
		cache:     newSignatureCache(),
		addr:      addr,
		catchupCh: make(chan Beacon, 1),
		l:         l.With("group_idx", idx),
	}
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
	h.Lock()
	defer h.Unlock()

	peer, _ := peer.FromContext(c)
	h.l.Debug("process_beacon", "request", "from", peer.Addr.String())
	var err error
	// 1- we check the round number: it must be consistent between the time of
	// current round and time of next round
	shouldVerify := h.started && !h.catchup
	if shouldVerify && uint64(math.Abs(float64(p.Round-h.round))) > maxRoundDelta {
		h.l.Error("process_beacon", "out-of-bounds", "current", h.round, "packet_round", p.Round)
		return nil, errors.New(errOutOfRound)
	}

	// 2- we dont catch up at least with invalid signature
	msg := Message(p.PreviousSig, p.Round)
	if err := h.conf.Scheme.VerifyPartial(h.pub, msg, p.PartialSig); err != nil {
		h.l.Error("process_beacon", err, "from", peer.Addr.String())
		return nil, err
	}

	// check if we have it in the saved signatures
	signature, err := h.signature(p.Round, msg)
	resp := &proto.BeaconResponse{
		PartialSig: signature,
	}

	// start our own internal timer
	if h.catchup {
		h.l.Info("process_beacon", "catchup")
		h.catchupCh <- Beacon{
			PreviousSig: p.GetPreviousSig(),
			Round:       p.GetRound(),
		}
		h.catchup = false
	}
	return resp, err
}

// Start runs the beacon protocol (threshold BLS signature). The first round
// will sign the message returned by the config.FirstRound() function. If the
// genesis time specified in the group is already passed, Start returns an
// error. In that case, if the group is already running, you should call
// SyncAndRun().
func (h *Handler) Start() error {
	h.l.Info("beacon", "start")
	if time.Now().Unix() > h.conf.Group.GenesisTime {
		return errors.New("beacon: genesis time already passed. Call SyncAndRun().")
	}
	msg, nextRound := h.conf.FirstRound()
	h.run(msg, 0, nextRound, h.conf.Group.GenesisTime)
	return nil
}

// SyncAndRun waits the next round's time to participate. This method is called
// when a node stops its daemon (maintenance or else) and get backs in the
// already running network . If the node does not have the previous randomness,
// it sync its local chain with other nodes to be able to participate in the
// next upcoming round.
func (h *Handler) SyncAndRun() error {
	trials := 0
	maxTrials := 3
	var nextRound uint64
	var nextTime int64
	var previousBeacon *Beacon
	var err error
	for trials < maxTrials {
		previousBeacon, err = h.store.Last()
		if err != nil {
			return err
		}
		nextRound, nextTime = NextRound(h.conf.Clock, h.conf.Group.Period, h.conf.Group.GenesisTime)
		if previousBeacon.Round+1 == nextRound {
			// next round will build on the one we have
			break
		}
		// there is a gap - we need to sync with other peers
		if err := h.syncFrom(previousBeacon.Round + 1); err != nil {
			return err
		}
		trials++
	}
	if trials == maxTrials {
		h.l.Fatal("beacon_sync", "failed")
	}

	// next round R is signing over rand_(R-1) || R
	previousSig := previousBeacon.PreviousSig
	previousRound := previousBeacon.PreviousRound
	go h.run(previousSig, previousRound, nextRound, nextTime)
}

func (h *Handler) syncFrom(initRound uint64, initSignature []byte) error {
	currentRound := initRound
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
		ctx, cancel := context.WithCancel(context.Background())
		request := &proto.SyncRequest{
			FromRound: currentRound,
		}
		respCh, err := h.client.SyncChain(ctx, id, request)
		if err != nil {
			h.l.Error("sync_from", currentRound, "error", err, "from", id.Address())
			continue
		}

		for syncReply := range respCh {
			nextRound, _ := NextRound(h.conf.Clock, h.conf.Period, h.conf.Genesis)
			// we only sync for increasing round numbers
			// there might be gaps so we dont check for sequentiality but our
			// chain from the round we have should be valid
			if syncReply.Round <= currentRound {
				h.l.Debug("sync_round", currentRound, "from", id.Address(), "invalid-reply")
				cancel()
				break
			}
			prevSig := syncReply.GetPreviousSig()
			prevRound := syncReply.GetPreviousRound()
			if currentRound != prevRound || !bytes.Equal(prevSig, currentSig) {
				h.l.Error("sync_round", currentRound, "from", id.Address(), "invalid chain")
				cancel()
				break
			}
			msg := Message(prevSig, prevRound, syncReply.GetRound())
			if err := h.conf.Scheme.VerifyRecovered(h.pub.Commit(), msg, syncReply.GetSignature()); err != nil {
				h.l.Error("sync_round", currentRound, "invalid_sig", err, "from", id.Address())
				cancel()
				break
			}
			h.l.Debug("sync_round", currentRound, "valid_sync", id.Address())
			h.store.Put(&Beacon{
				PreviousSig:   syncReply.GetPreviousSig(),
				PreviousRound: syncReply.GetPreviousRound(),
				Round:         syncReply.GetRound(),
				Signature:     syncReply.GetSignature(),
			})
			currentRound = syncReply.GetRound()
			currentSig = syncReply.GetSignature()
			// if it gave us the round just before the next one, then return
			if currentRound+1 == nextRound {
				cancel()
				return nil
			}
		}
	}
	return fmt.Errorf("syncing from round %d with all peers failed", initRound)
}

// Run starts the TBLS protocol. There are two cases:
// - catchup is false: it means it is the beginning of the the chain for the
// current group. Run() will wait until the genesis time specified
// The seed is the first
// message signed alongside with the current round number. All subsequent
// signatures are chained: s_i+1 = SIG(s_i || round)
// The catchup parameter, if true, forces the beacon generator to wait until it
// receives a RPC call from another node. At that point, the beacon generator
// knows the current round it must execute. WARNING: It is not a bullet proof
// solution, as a remote node could trick this beacon generator to start for an
// outdated or far-in-the-future round. This is a starting point.
//func (h *Handler) Loop(seed []byte, period time.Duration, catchup bool) {
func (h *Handler) run(initSig []byte, initRound, nextRound uint64, startTime int64) {
	h.l.Info("beacon_wait", startTime)
	// sleep until beginning of next round
	time.Sleep(time.Until(time.Unix(startTime)))

	// start for this round already
	var goToNextRound = true
	var currentRoundFinished bool
	var currentRound uint64 = nextRound
	var prevSig []byte = initSig
	var prevRound uint64 = initRound
	var period = h.conf.Group.Period
	winCh := make(chan roundInfo)
	closingCh := make(chan bool)

	h.Lock()
	h.ticker = h.conf.Clock.Ticker(period)
	h.started = true
	h.Unlock()
	//h.savePreviousSignature(prevSig)
	for {
		if goToNextRound {
			// we launch the next round and close the previous operations if
			// still running
			close(winCh)
			winCh = make(chan roundInfo)
			close(closingCh)
			closingCh = make(chan bool)
			go h.runRound(currentRound, prevRound, prevSig, winCh, closingCh)

			goToNextRound = false
			currentRoundFinished = false
		}

		// that way the execution starts directly, not after *one tick*
		select {
		case <-h.ticker.C:
			if !currentRoundFinished {
				// the current round has not finished while the next round is
				// starting. In this case, we increase the round number but
				// still signs on the current signature.
				currentRound++
			}
			// the ticker is king so we always start a new round at each tick
			goToNextRound = true
			continue
		case roundInfo := <-winCh:
			if roundInfo.round != currentRound {
				// an old round that finishes later than supposed to, we need to
				// make sure to not build upon it as other nodes may be already
				// ahead - an round that finishes after its time is not
				// considered in the chain
				continue
			}
			// we signal that the round is finished and move on by waiting on
			// the next tick,i.e. proper operational flow.
			currentRound++
			prevSig = roundInfo.signature
			prevRound = roundInfo.round
			currentRoundFinished = true
		case <-h.close:
			return
		}
	}
}

type roundInfo struct {
	round     uint64
	signature []byte
}

func (h *Handler) runRound(currentRound, prevRound uint64, prevSig []byte, winCh chan roundInfo, closeCh chan bool) {
	h.l.Debug("beacon_round", currentRound, "time", h.conf.Clock.Now())
	msg := Message(prevSig, prevRound, currentRound)
	signature, err := h.signature(currentRound, msg)
	if err != nil {
		h.l.Error("beacon_round", fmt.Sprintf("creating signature: %s", err), "round", currentRound)
		return
	}

	var sigs [][]byte
	sigs = append(sigs, signature)
	request := &proto.BeaconRequest{
		Round:         currentRound,
		PreviousRound: prevRound,
		PreviousSig:   prevSig,
		PartialSig:    signature,
	}
	respCh := make(chan *proto.BeaconResponse, h.group.Len())
	// send all requests in parallel
	for _, id := range h.group.Nodes {
		if h.addr == id.Addr {
			continue
		}
		// this go routine sends the packet to one node. It will always
		// return assuming there's a timeout on the connection
		go func(i *key.Identity) {
			resp, err := h.client.NewBeacon(i, request)
			if err != nil {
				h.l.Error("beacon_round", currentRound, "error requesting beacon", err, "from", i.Address())
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
	for len(sigs) < h.group.Threshold {
		select {
		case resp := <-respCh:
			sigs = append(sigs, resp.PartialSig)
			h.l.Debug("beacon_round", currentRound, "partial_signature", len(sigs), "required", h.group.Threshold)
		case <-closeCh:
			// it's already time to go to the next, there has been not
			// enough time or nodes are too slow. In any case it's a
			// problem.
			h.l.Error("beacon_round", currentRound, "quitting prematurely", "problem with short period or beacon nodes")
			return
		}
	}
	finalSig, err := h.conf.Scheme.Recover(h.pub, msg, sigs, h.group.Threshold, h.group.Len())
	if err != nil {
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
	h.l.Info("beacon_round", currentRound, "signature", fmt.Sprintf("%x", finalSig), "previous_sig", fmt.Sprintf("%x", prevSig), "randomness", fmt.Sprintf("%x", randomness))
	winCh <- roundInfo{round: currentRound, signature: finalSig}
}

// Stop the beacon loop from aggregating  further randomness, but it
// finishes the one it is aggregating currently.
func (h *Handler) Stop() {
	h.Lock()
	defer h.Unlock()
	if h.ticker != nil {
		h.ticker.Stop()
	}
	close(h.close)
	h.store.Close()
	h.l.Info("beacon", "stop")
}

// nextRound increase the round counter and evicts the cache from old entries.
func (h *Handler) nextRound() uint64 {
	h.Lock()
	defer h.Unlock()
	h.round++
	h.cache.Evict(h.round)
	return h.round
}

func (h *Handler) setRound(r uint64) {
	h.Lock()
	defer h.Unlock()
	h.round = r
}

func (h *Handler) savePreviousSignature(sig []byte) {
	h.Lock()
	defer h.Unlock()
	h.previousSig = sig
}

func (h *Handler) getPreviousSignature() []byte {
	h.Lock()
	defer h.Unlock()
	return h.previousSig
}

func (h *Handler) signature(round uint64, msg []byte) ([]byte, error) {
	var err error
	signature, ok := h.cache.Get(round, msg)
	if !ok {
		signature, err = h.conf.Scheme.Sign(h.share.PrivateShare(), msg)
		if err != nil {
			return nil, err
		}
		h.cache.Put(round, msg, signature)
	}
	return signature, nil
}

func (h *Handler) setCatchup(catchup bool) {
	h.Lock()
	defer h.Unlock()
	h.catchup = catchup
}

type signatureCache struct {
	sync.Mutex
	cache map[uint64]*PartialSig
}

func newSignatureCache() *signatureCache {
	return &signatureCache{
		cache: make(map[uint64]*PartialSig),
	}
}

// Put saves the partial signature associated with the given round and
// message for futur usage.
func (s *signatureCache) Put(round uint64, msg, sig []byte) {
	// XXX signature cache is disabled for the moment
	if false {
		s.Lock()
		defer s.Unlock()
		s.cache[round] = &PartialSig{message: msg, PartialSig: sig}

	}
}

// Get returns the partial signature associated with the given round. It
// verifies if the message is consistent (it should not be).It returns false if
// the signature is not present or the message is not consistent.
func (s *signatureCache) Get(round uint64, msg []byte) ([]byte, bool) {
	if false {
		s.Lock()
		defer s.Unlock()
		sig, ok := s.cache[round]
		if !ok {
			return nil, false
		}
		if !bytes.Equal(msg, sig.message) {
			//slog.Infof("beacon: inconsistency for round %d: msg stored %x vs msg received %x", round, msg, rand.message)
			return nil, false
		}
		return sig.PartialSig, true
	}
	return nil, false
}

// evictCache evicts some old entries that should not be required anymore.
func (s *signatureCache) Evict(currRound uint64) {
	for round := range s.cache {
		if round < (currRound - maxRoundDelta) {
			delete(s.cache, round)
		}
	}
}

//PartialSig holds partial signature
type PartialSig struct {
	message    []byte
	PartialSig []byte
}
