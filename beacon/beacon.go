package beacon

import (
	"bytes"
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/dedis/drand/protobuf/crypto"
	proto "github.com/dedis/drand/protobuf/drand"
	"go.dedis.ch/kyber/v3/share"
	"go.dedis.ch/kyber/v3/sign/bls"
	"go.dedis.ch/kyber/v3/sign/tbls"

	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	"github.com/nikkolasg/slog"
)

// What is the maximum round difference a drand node accepts to sign
var maxRoundDelta uint64 = 2

// Config holds the different cryptographc informations necessary to run the
// randomness beacon.
type Config struct {
	Private *key.Pair
	Share   *key.Share
	Group   *key.Group
	Seed    []byte
}

// Handler holds the logic to initiate, and react to the TBLS protocol. Each time
// a full signature can be recosntructed, it saves it to the given Store.
type Handler struct {
	conf *Config
	// to communicate with other drand peers
	client net.InternalClient
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
	previousRand []byte
	// stores some recent signature to avoid recreating them
	cache *signatureCache
	// signal if a beacon node is late, it waits for the next incoming request
	// to start its own timer
	catchup bool
	// signal the beacon received from incoming request to the timer
	catchupCh chan Beacon

	ticker *time.Ticker
	close  chan bool
	addr   string
	// group id to embed in all beacons
	// XXX temporary solution to change when we really want flexible groups
	id      int32
	seed    []byte
	started bool
}

// NewHandler returns a fresh handler ready to serve and create randomness
// beacon
func NewHandler(c net.InternalClient, s Store, conf *Config) (*Handler, error) {
	if conf.Private == nil || conf.Share == nil || conf.Group == nil || conf.Seed == nil {
		return nil, errors.New("beacon: invalid configuration")
	}
	idx, exists := conf.Group.Index(conf.Private.Public)
	if !exists {
		return nil, errors.New("beacon: keypair not included in the given group")
	}
	// XXX Make it parametrizable
	id, exists := crypto.GroupToID(key.G1)
	if !exists {
		return nil, errors.New("beacon: group has no registered ID")
	}

	addr := conf.Group.Nodes[idx].Addr

	c.SetTimeout(conf.Group.Period) // wait on each call no more than the period
	return &Handler{
		conf:      conf,
		client:    c,
		group:     conf.Group,
		share:     conf.Share,
		pub:       share.NewPubPoly(key.G2, key.G2.Point().Base(), conf.Share.Commits),
		index:     idx,
		store:     s,
		close:     make(chan bool),
		cache:     newSignatureCache(),
		addr:      addr,
		catchupCh: make(chan Beacon, 1),
		id:        id,
		seed:      conf.Seed,
	}, nil
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
	var err error
	// 1- we check the round number only if we started already and are not in
	// catch-up mode
	shouldVerify := h.started && !h.catchup
	if shouldVerify && uint64(math.Abs(float64(p.Round-h.round))) > maxRoundDelta {
		return nil, errors.New(errOutOfRound)
	}

	// 2- we dont catch up at least with invalid signature
	msg := Message(p.PreviousRand, p.Round)
	if err := tbls.Verify(key.Pairing, h.pub, msg, p.PartialRand); err != nil {
		slog.Debugf("beacon: received invalid signature request")
		return nil, err
	}

	// check if we have it in the saved signatures
	signature, err := h.signature(p.Round, msg)
	resp := &proto.BeaconResponse{
		PartialRand: signature,
	}

	// start our own internal timer
	if h.catchup {
		h.catchupCh <- Beacon{
			PreviousRand: p.GetPreviousRand(),
			Round:        p.GetRound(),
		}
		h.catchup = false
	}
	return resp, err
}

// Run starts periodically the TBLS protocol. The seed is the first
// message signed alongside with the current round number. All subsequent
// signatures are chained: s_i+1 = SIG(s_i || round)
// The catchup parameter, if true, forces the beacon generator to wait until it
// receives a RPC call from another node. At that point, the beacon generator
// knows the current round it must execute. WARNING: It is not a bullet proof
// solution, as a remote node could trick this beacon generator to start for an
// outdated or far-in-the-future round. This is a starting point.
//func (h *Handler) Loop(seed []byte, period time.Duration, catchup bool) {
func (h *Handler) Run(period time.Duration, catchup bool) {
	var goToNextRound = true // need to start one round anyway
	var currentRoundFinished bool

	var round uint64
	var prevRand []byte
	winCh := make(chan roundInfo)
	closingCh := make(chan bool)

	h.Lock()
	if !catchup {
		// let's determine the previous signature we should build upon. It can
		// be the seed or a guenuine one.
		b, err := h.store.Last()
		if err == ErrNoBeaconSaved {
			prevRand = h.seed
		} else if err == nil {
			prevRand = b.Randomness
			h.round = b.Round
		} else {
			slog.Infof("beacon: can't load from the database: %v", err)
		}
	}

	h.ticker = time.NewTicker(period)
	h.started = true
	h.Unlock()
	h.savePreviousSignature(prevRand)

	for {
		if goToNextRound {
			// we launch the next round and close the previous operations if
			// still running
			close(closingCh)
			closingCh = make(chan bool)
			if catchup {
				// signal that we are waiting on the next call
				h.setCatchup(true)
				// it's OK here to potentially wait indefinitely since we anyway
				// need to be up to date to continue so if we receive nothing we
				// can't do anything else anyway.
				b := <-h.catchupCh
				slog.Infof("beacon: catched up on round %d (previous round %d)", b.Round, round)
				// nextRound() automatically increases
				h.setRound(b.Round - 1)
				h.savePreviousSignature(b.PreviousRand)
				catchup = false
			}

			// take the next round and prev signature
			round = h.nextRound()
			prevRand = h.getPreviousSignature()

			go h.run(round, prevRand, winCh, closingCh)

			goToNextRound = false
			currentRoundFinished = false
		}
		// that way the execution starts directly, not after *one tick*
		select {
		case <-h.ticker.C:
			if !currentRoundFinished {
				// the current round has not finished yet, so we must catchup
				// first to get up-to-date info
				catchup = true
			}
			// the ticker is king so we always start a new round at each tick
			goToNextRound = true
			continue
		case roundInfo := <-winCh:
			if roundInfo.round != round {
				// an old round that finishes later than supposed to, we need to
				// make sure to not build upon it as other nodes may be already
				// ahead by a few rounds
				continue
			}
			// since it is the expected round number, we can set that signature
			// as the basis for the next round
			h.savePreviousSignature(roundInfo.signature)
			// we signal that the round is finished and move on by waiting on
			// the next tick,i.e. proper operational flow.
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

func (h *Handler) run(round uint64, prevRand []byte, winCh chan roundInfo, closeCh chan bool) {
	slog.Debugf("beacon %s: next tick for round %d - time %s", h.addr, round, time.Now())
	msg := Message(prevRand, round)
	signature, err := h.signature(round, msg)
	if err != nil {
		slog.Debugf("beacon: round %d err creating/caching signature %s", round, err)
		return
	}

	var sigs [][]byte
	sigs = append(sigs, signature)
	request := &proto.BeaconRequest{
		Round:        round,
		PreviousRand: prevRand,
		PartialRand:  signature,
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
			//slog.Debugf("beacon: %s round %d: request new beacon to %s", h.addr, round, i.Address())
			resp, err := h.client.NewBeacon(i, request)
			if err != nil {
				slog.Debugf("beacon: %s round %d err from %s: %s", h.addr, round, i.Address(), err)
				if strings.Contains(err.Error(), errOutOfRound) {

				}
				return
			}
			if err := tbls.Verify(key.Pairing, h.pub, msg, resp.PartialRand); err != nil {
				slog.Debugf("beacon: invalid beacon response: %s", err)
				return
			}
			slog.Debugf("beacon: %s round %d valid response from %s", h.addr, round, i.Address())
			respCh <- resp
		}(id)
	}
	// wait for a threshold of replies or if the timeout occured
	for len(sigs) < h.group.Threshold {
		select {
		case resp := <-respCh:
			sigs = append(sigs, resp.PartialRand)
			slog.Debugf("beacon: %s round %d received partial randomness %d/%d", h.addr, round, len(sigs), h.group.Threshold)
		case <-closeCh:
			// it's already time to go to the next, there has been not
			// enough time or nodes are too slow. In any case it's a
			// problem.
			slog.Infof("beacon: %s quitting prematurely round %d.", h.addr, round)
			slog.Infof("beacon: might be a problem with the nodes or the beacon period is too short")
			return
		}
	}
	//slog.Debugf("beacon: %s round %d -> out of the waiting loop (%d sigs)", h.addr, round, len(sigs))
	finalSig, err := tbls.Recover(key.Pairing, h.pub, msg, sigs, h.group.Threshold, h.group.Len())
	if err != nil {
		slog.Infof("beacon: could not reconstruct final beacon: %s", err)
		return
	}
	if err := bls.Verify(key.Pairing, h.pub.Commit(), msg, finalSig); err != nil {
		slog.Print(sigs)
		slog.Print("beacon: invalid reconstructed beacon signature ? That's BAD, threshold ", h.group.Threshold)
		return
	}

	beacon := &Beacon{
		Round:        round,
		PreviousRand: prevRand,
		Randomness:   finalSig,
		Gid:          h.id,
	}
	//slog.Debugf("beacon: %s round %d -> SAVING beacon in store ", h.addr, round)
	// we can always store it even if it is too late, since it is valid anyway
	if err := h.store.Put(beacon); err != nil {
		slog.Infof("beacon: %s error storing beacon randomness: %s", h.addr, err)
		return
	}
	//slog.Debugf("beacon: %s round %d -> saved beacon in store sucessfully", h.addr, round)
	//slog.Infof("beacon: %s round %d finished: %x", h.addr, round, finalSig)
	slog.Debugf("beacon: %s round %d finished: \n\tfinal: (id=%d) %x\n\tprev: %x\n", h.addr, round, beacon.Gid, finalSig, prevRand)
	winCh <- roundInfo{round: round, signature: finalSig}
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
	h.previousRand = sig
}

func (h *Handler) getPreviousSignature() []byte {
	h.Lock()
	defer h.Unlock()
	return h.previousRand
}

func (h *Handler) signature(round uint64, msg []byte) ([]byte, error) {
	var err error
	signature, ok := h.cache.Get(round, msg)
	if !ok {
		signature, err = tbls.Sign(key.Pairing, h.share.Share, msg)
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
	cache map[uint64]*partialRand
}

func newSignatureCache() *signatureCache {
	return &signatureCache{
		cache: make(map[uint64]*partialRand),
	}
}

// Put saves the partial signature associated with the given round and
// message for futur usage.
func (s *signatureCache) Put(round uint64, msg, rand []byte) {
	// XXX signature cache is disabled for the moment
	if false {
		s.Lock()
		defer s.Unlock()
		s.cache[round] = &partialRand{message: msg, partialRand: rand}

	}
}

// Get returns the partial signature associated with the given round. It
// verifies if the message is consistent (it should not be).It returns false if
// the signature is not present or the message is not consistent.
func (s *signatureCache) Get(round uint64, msg []byte) ([]byte, bool) {
	if false {
		s.Lock()
		defer s.Unlock()
		rand, ok := s.cache[round]
		if !ok {
			return nil, false
		}
		if !bytes.Equal(msg, rand.message) {
			slog.Infof("beacon: inconsistency for round %d: msg stored %x vs msg received %x", round, msg, rand.message)
			return nil, false
		}
		return rand.partialRand, true
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

type partialRand struct {
	message     []byte
	partialRand []byte
}
