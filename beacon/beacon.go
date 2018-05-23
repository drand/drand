package beacon

import (
	"bytes"
	"context"
	"errors"
	"math"
	"sync"
	"time"

	proto "github.com/dedis/drand/protobuf/drand"
	"github.com/dedis/kyber/share"
	"github.com/dedis/kyber/sign/bls"
	"github.com/dedis/kyber/sign/tbls"

	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	"github.com/nikkolasg/slog"
)

// What is the maximum round difference a drand node accepts to sign
var maxRoundDelta uint64 = 2

// Handler holds the logic to initiate, and react to the TBLS protocol. Each time
// a full signature can be recosntructed, it saves it to the given Store.
type Handler struct {
	// to communicate with other drand peers
	client net.Client
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
}

// NewHandler returns a fresh handler ready to serve and create randomness
// beacon
func NewHandler(c net.Client, priv *key.Pair, sh *key.Share, group *key.Group, s Store) *Handler {
	idx, exists := group.Index(priv.Public)
	if !exists {
		// XXX
		panic("that's just plain wrong and I should be an error")
	}
	addr := group.Nodes[idx].Addr
	return &Handler{
		client:    c,
		group:     group,
		share:     sh,
		pub:       share.NewPubPoly(key.G2, key.G2.Point().Base(), sh.Commits),
		index:     idx,
		store:     s,
		close:     make(chan bool),
		cache:     newSignatureCache(),
		addr:      addr,
		catchupCh: make(chan Beacon, 1),
	}
}

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
	// 1 and only test if we are running, not if we just started and are trying
	// to catch up
	if !h.catchup && uint64(math.Abs(float64(p.Round-h.round))) > maxRoundDelta {
		return nil, errors.New("beacon won't sign out-of-round beacon request")
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

// RandomBeacon starts periodically the TBLS protocol. The seed is the first
// message signed alongside with the current round number. All subsequent
// signatures are chained: s_i+1 = SIG(s_i || round)
// The catchup parameter, if true, forces the beacon generator to wait until it
// receives a RPC call from another node. At that point, the beacon generator
// knows the current round it must execute. WARNING: It is not a bullet proof
// solution, as a remote node could trick this beacon generator to start for an
// outdated or far-in-the-future round. This is a starting point.
func (h *Handler) Loop(seed []byte, period time.Duration, catchup bool) {
	if catchup {
		// signal that we are waiting on the next call
		h.setCatchup(true)
		b := <-h.catchupCh
		seed = b.PreviousRand
		// set the current round -1 so the next iteration starts correctly
		h.setRound(b.Round - 1)
		slog.Infof("beacon: catch up on request for round %d (previous %x)", b.Round, b.PreviousRand)
		// catchup is set to false by the request directly

	}
	h.Lock()
	h.ticker = time.NewTicker(period)
	h.Unlock()
	var failed uint64
	h.savePreviousSignature(seed)
	//for wt := range b.ticker.C {
	closingCh := make(chan bool)
	fn := func(closeCh chan bool) {
		round := h.nextRound()
		prevRand := h.getPreviousSignature()
		slog.Debugf("beacon %s: next tick for round %d", h.addr, round)
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
			if h.index == id.Index {
				continue
			}
			// this go routine sends the packet to one node. It will always
			// return assuming there's a timeout on the connection
			go func(i *key.Identity) {
				//slog.Debugf("beacon: %s round %d: request new beacon to %s", h.addr, round, i.Address())
				resp, err := h.client.NewBeacon(i, request)
				if err != nil {
					slog.Debugf("beacon: %s round %d err receiving response from %s: %s", h.addr, round, i.Address(), err)
					return
				}
				if err := tbls.Verify(key.Pairing, h.pub, msg, resp.PartialRand); err != nil {
					slog.Debugf("beacon: invalid beacon response: %s", err)
					return
				}
				slog.Debugf("beacon: %s round %s valid response from %s", h.addr, i.Address())
				respCh <- resp
			}(id.Identity)
		}
		// wait for a threshold of replies or if the timeout occured
		for len(sigs) < h.group.Threshold {
			select {
			case resp := <-respCh:
				sigs = append(sigs, resp.PartialRand)
				slog.Debugf("beacon: %s round %d received %d/%d response", h.addr, round, len(sigs), h.group.Threshold)
			case <-closeCh:
				// it's already time to go to the next, there has been not
				// enough time or nodes are too slow. In any case it's a
				// problem.
				// XXX should be accessed in thread safe manner but highly
				// unlikely that the rounds are that short in practice...
				failed++
				slog.Infof("beacon: quitting prematurely round %d (%d failed).", round, failed)
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
			slog.Print("beacon: invalid reconstructed beacon signature ? That's BAD")
			return
		}

		beacon := &Beacon{
			Round:        round,
			PreviousRand: prevRand,
			Randomness:   finalSig,
		}
		//slog.Debugf("beacon: %s round %d -> SAVING beacon in store ", h.addr, round)
		if err := h.store.Put(beacon); err != nil {
			slog.Infof("beacon: error storing beacon randomness: %s", err)
			return
		}
		//slog.Debugf("beacon: %s round %d -> saved beacon in store sucessfully", h.addr, round)
		h.savePreviousSignature(finalSig)
		slog.Infof("beacon: round %d finished: %x", round, finalSig)
		slog.Debugf("beacon: %s round %d finished: \n\tfinal: %x\n\tprev: %x\n", h.addr, round, finalSig, prevRand)
	}

	// run the loop !
	for {
		// close the previous operations if still running
		close(closingCh)
		closingCh = make(chan bool)
		// start the new one
		go fn(closingCh)
		select {
		// that way the execution starts directly, not after *one tick*
		case <-h.ticker.C:
			continue
		case <-h.close:
			return
		}
	}
	slog.Info("beacon: stopped loop")
}

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
	return
	s.Lock()
	defer s.Unlock()
	s.cache[round] = &partialRand{message: msg, partialRand: rand}
}

// Get returns the partial signature associated with the given round. It
// verifies if the message is consistent (it should not be).It returns false if
// the signature is not present or the message is not consistent.
func (s *signatureCache) Get(round uint64, msg []byte) ([]byte, bool) {
	return nil, false
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
