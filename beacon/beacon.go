package beacon

import (
	"bytes"
	"context"
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
	Private  *key.Pair
	Share    *key.Share
	Group    *key.Group
	Scheme   sign.ThresholdScheme
	Clock    clock.Clock
	WaitTime time.Duration
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
	manager *roundManager
	// the signature of this node for the current round. acts like a cache to
	// avoid resigning it for each request.
	currentPartial *partialSig

	index int

	close   chan bool
	addr    string
	started bool
	stopped bool

	callbacks []func(*Beacon)

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

	//if conf.WaitTime == time.Duration(0) {
	//conf.WaitTime = 100 * time.Millisecond
	//}
	//conf.WaitTime = 0 * time.Millisecond

	c.SetTimeout(1 * time.Second)
	addr := conf.Private.Public.Address()
	logger := l.With("index", idx)
	handler := &Handler{
		conf:    conf,
		client:  c,
		group:   conf.Group,
		share:   conf.Share,
		pub:     conf.Share.PubPoly(),
		index:   idx,
		addr:    addr,
		store:   s,
		close:   make(chan bool),
		l:       logger,
		manager: newRoundManager(l, conf.Group.Threshold, conf.Scheme),
	}
	// genesis block at round 0, next block at round 1
	// THIS is to change when one network wants to build on top of another
	// network's chain. Note that if present it overwrites.
	b := &Beacon{
		Signature: conf.Group.GetGenesisSeed(),
		Round:     0,
	}
	s.Put(b)
	return handler, nil
}

var errOutOfRound = "out-of-round beacon request"

// ProcessBeacon receives a request for a beacon partial signature. It replies
// successfully with a valid partial signature over the given beacon packet
// information if the following is true:
// 1- the round for the request is not different than the current round by a certain threshold
// 2- the partial signature in the embedded response is valid. This proves that
// the requests comes from a qualified node from the DKG phase.
func (h *Handler) ProcessBeacon(c context.Context, p *proto.BeaconPacket) (*proto.Empty, error) {
	peer, _ := peer.FromContext(c)
	h.l.Debug("received", "request", "from", peer.Addr.String())

	nextRound, _ := NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	currentRound := nextRound - 1

	// check what we receive is for the current round
	if p.GetRound() != currentRound {
		// request is not for current round
		h.l.Error("request_round", p.GetRound(), "current_round", currentRound, "now", h.conf.Clock.Now().Unix(), "clock_pointer", fmt.Sprintf("%p", h.conf.Clock), "node_pointer", fmt.Sprintf("%p", h))
		return nil, fmt.Errorf("invalid round: %d instead of %d", p.GetRound(), nextRound-1)
	}

	// check that the previous is really a previous round
	// XXX Try to find a way to check if it's really the round we want instead
	// of relying on the cache manager
	if p.GetPreviousRound() >= currentRound {
		h.l.Error("request_round", currentRound, "previous_round", p.GetPreviousRound())
		return nil, fmt.Errorf("invalid previous round: %d > current %d", p.GetPreviousRound(), currentRound)
	}

	msg := Message(p.GetPreviousSig(), p.GetPreviousRound(), p.GetRound())
	// verify if request is valid
	if err := h.conf.Scheme.VerifyPartial(h.pub, msg, p.GetPartialSig()); err != nil {
		//shortPub := h.pub.Eval(1).V.String()[14:19]
		//fmt.Printf(" || FAIL index %d : pointer %p : shortPub: %s\n", h.index, h, shortPub)
		h.l.Error("process_request", err, "from", peer.Addr.String(), "prev_sig", shortSigStr(p.GetPreviousSig()), "prev_round", p.GetPreviousRound(), "curr_round", currentRound, "msg_sign", shortSigStr(msg))
		return nil, err
	}
	idx, _ := h.conf.Scheme.IndexOf(p.GetPartialSig())
	if idx == h.index {
		h.l.Error("process_request", "same_index", "got", idx, "our", h.index)
		return nil, errors.New("same index as this node")
	}
	h.manager.NewBeacon(p)
	return new(proto.Empty), nil
}

// Store returns the store associated with this beacon handler
func (h *Handler) Store() Store {
	return h.store
}

// SyncChain is the server side call that reply with the beacon in order to the
// client requesting the syncing.
func (h *Handler) SyncChain(req *proto.SyncRequest, p proto.Protocol_SyncChainServer) error {
	fromRound := req.GetFromRound()
	var err error
	peer, _ := peer.FromContext(p.Context())
	h.l.Debug("received", "request", "from", peer.Addr.String())

	h.store.Cursor(func(c Cursor) {
		for beacon := c.Seek(fromRound); beacon != nil; beacon = c.Next() {
			reply := &proto.SyncResponse{
				PreviousRound: beacon.PreviousRound,
				PreviousSig:   beacon.PreviousSig,
				Round:         beacon.Round,
				Signature:     beacon.Signature,
			}
			nRound, _ := NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
			l, _ := h.store.Last()
			fmt.Printf("\nnode %d - reply sync from round %d to %d - head at %d -- last beacon %s\n\n", h.index, fromRound, reply.Round, nRound-1, l)
			h.l.Debug("sync_chain_reply", peer.Addr.String(), "from", fromRound, "to", reply.Round, "head", nRound-1)
			if err = p.Send(reply); err != nil {
				fmt.Println(" ERROR SYNC CHAIN SERVER SIDE:", err)
				return
			}
			fromRound = reply.Round
		}
	})
	return err
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
		return errors.New("beacon: genesis time already passed. Call Catchup()")
	}
	genesis, err := h.store.Get(0)
	if err != nil {
		return errors.New("no genesis block found in store")
	}
	go h.run(genesis.Signature, genesis.Round, genesis.Round+1, h.conf.Group.GenesisTime)
	return nil
}

// Catchup waits the next round's time to participate. This method is called
// when a node stops its daemon (maintenance or else) and get backs in the
// already running network . If the node does not have the previous randomness,
// it sync its local chain with other nodes to be able to participate in the
// next upcoming round.
func (h *Handler) Catchup() {
	ids := shuffleNodes(h.conf.Group.Nodes)
	// we sync with the nodes of the current network
	prevBeacon, err := h.Sync(ids)
	if err != nil {
		h.l.Error("syncing", err)
	}
	nextRound, nextTime := NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	previousSig := prevBeacon.Signature
	previousRound := prevBeacon.Round
	//fmt.Printf("\nSYNCING DONE: prevRound %d prevSig %s - nextRound %d nextTime %d\n\n", previousRound, shortSigStr(previousSig), nextRound, nextTime)
	h.run(previousSig, previousRound, nextRound, nextTime)
}

// Transition makes this beacon continuously sync until the time written in the
// "TransitionTime" in the handler's group file, where he will start generating
// randomness. To sync, he contact the nodes listed in the previous group file
// given.
// TODO: it should be better to use the public streaming API but since it is
// likely to change, right now we use the sync API. Later on when API is well
// defined, best to use streaming.
func (h *Handler) Transition(prevNodes []*key.Identity) error {
	targetTime := h.conf.Group.TransitionTime
	tRound, tTime := NextRound(targetTime, h.conf.Group.Period, h.conf.Group.GenesisTime)
	// tTime is the time of the next round -
	// we want to compare the actual roudn
	// XXX simplify this by implementing a "RoundOfTime" method
	tTime = tTime - int64(h.conf.Group.Period.Seconds())
	tRound = tRound - 1
	if tTime != targetTime {
		fmt.Printf("node %d - %s : next time %d vs transition time %d\n", h.index, h.conf.Private.Public.Address(), tTime, targetTime)
		h.l.Fatal("transition_time", "invalid")
		return nil
	}
	ids := shuffleNodes(prevNodes)
	var lastBeacon *Beacon
	var err error
	nErr := 0
	maxErr := 10
	for nErr < maxErr {
		// we keep the same list of ids - so we contact the same peer for each
		// consecutive sync calls instead of using different peers each time
		lastBeacon, err = h.Sync(ids)
		if err != nil {
			h.l.Error("transition", err)
			nErr++
			continue
		}
		if lastBeacon.Round+1 == tRound {
			// next round is the round where the transition happens !
			// switch to "normal" run mode
			h.l.Debug("transition_sync", "done", "head", lastBeacon.Round)

			break
		}
		//fmt.Printf("\t TransitionSYNC: lastRound %d - target time is %d target round is %d\n", lastBeacon.Round, tTime, tRound)
		h.l.Debug("transition_sync", "wait", "head", lastBeacon.Round, "want", tRound-1)
		// we have some rounds to go before we arrive at the transition time
		// we sleep a period and then get back the next round afterwards
		// XXX TODO This assumes the same period for the previous group as for the
		// new group ! We need to change that if we want to have two independent
		// period time
		// XXX Should definitely rely on the stream public randomness here
		// otherwise since public API is likely to change, best not introuce to
		// much dependency here.
		h.conf.Clock.Sleep(h.conf.Group.Period)
	}
	if nErr == maxErr {
		h.l.Error("transition", "too-many-failures", "nerrors", nErr)
		return errors.New("can't sync to transition time")
	}
	h.run(lastBeacon.Signature, lastBeacon.Round, tRound, tTime)
	return nil
}

// Sync will try to sync to the given identities
func (h *Handler) Sync(to []*key.Identity) (*Beacon, error) {
	var nextRound uint64
	var nextTime int64
	var err error
	var lastBeacon *Beacon
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
	// only reason why trying multiple times is when the syncing takes too much
	// time and then we miss the current round. But if network is down we want
	// to build on what we can have best so it starts again. Hence we try two
	// times so first time can fail (because we finished syncing just after the
	// new round time so we're still one round behind) so the second time will
	// be quick and then network as usual.
	maxTrial := 2
	for trial := 0; trial < maxTrial; trial++ {
		// there is a gap - we need to sync with other peers
		lastBeacon, err := h.syncFrom(to, lastBeacon)
		if err != nil {
			h.l.Error("sync", "failed", "from", lastBeacon.Round)
		}
		if lastBeacon != nil {
			nextRound, nextTime = NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
			if lastBeacon.Round+1 == nextRound {
				// next round will build on the one we have - no need to sync
				h.l.Debug("sync", "done", "upto", lastBeacon.Round, "next_time", nextTime)
				return lastBeacon, nil
			}
			h.l.Debug("sync", "incomplete", "want", nextRound-1, "has", lastBeacon.Round)
		} else {
			h.l.Error("after_sync", "nil_beacon")
		}
		// not to aggressive
		sleepPeriod := 2 * time.Second
		h.l.Debug("sync_incomplete", "try_again", fmt.Sprintf("%d/%d", trial, maxTrial))
		h.conf.Clock.Sleep(sleepPeriod)
	}
	h.l.Error("sync", "failed", "network_down_or_BUG")
	return lastBeacon, errors.New("impossible to sync to current round: network is down?")
}

// Run starts the TBLS protocol: it will start the round "nextRound" that is
// building over the given initSig & the initRound. It sleeps until the starting
// time specified has kicked in.
func (h *Handler) run(initSig []byte, initRound, nextRound uint64, startTime int64) {
	// sleep until beginning of next round
	now := h.conf.Clock.Now().Unix()
	sleepTime := startTime - now
	h.l.Info("run_round", nextRound, "waiting_for", sleepTime, "period", h.conf.Group.Period.String())
	//fmt.Printf("node %d - %s | pointer: %p (genesis %d) - current time %d / now %d -> startTime %d - sleeping for %d ... (clock %p) - initRound: %d, nextRound %d\n", h.index, h.conf.Private.Public.Address(), h, h.conf.Group.GenesisTime, h.conf.Clock.Now().Unix(), now, startTime, sleepTime, h.conf.Clock, initRound, nextRound)
	h.conf.Clock.Sleep(time.Duration(sleepTime) * time.Second)
	//fmt.Printf("\n%d: node %d finished sleeping - time %d - starttime should be %d - clock pointer %p\n", time.Now().Unix(), h.index, h.conf.Clock.Now().Unix(), startTime, h.conf.Clock)
	// start for this round already
	var goToNextRound = true
	var currentRoundFinished bool
	var currentRound uint64 = nextRound
	var prevSig []byte = initSig
	var prevRound uint64 = initRound
	var period = h.conf.Group.Period
	winCh := make(chan *Beacon)
	closingCh := make(chan bool)
	ticker := h.conf.Clock.NewTicker(period)
	defer ticker.Stop()
	for {
		if goToNextRound {
			//fmt.Printf("\nnode %d - %p - goToNextRound %d!\n\n", h.index, h, currentRound)
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
		/*now := h.conf.Clock.Now().Unix()*/
		//_, targetTime := NextRound(now, period, h.conf.Group.GenesisTime)
		//toSleep := time.Duration(targetTime-now) * time.Second
		/*sleepCh := h.conf.Clock.After(toSleep)*/
		//for goToNextRound == false {
		select {
		case <-ticker.Chan():
			if !currentRoundFinished {
				// the current round has not finished while the next round is
				// starting. In this case, we increase the round number but
				// still signs on the current signature.
				currentRound++
			}
			// the ticker is king so we always start a new round at each
			// tick
			goToNextRound = true
			//fmt.Printf("\n <<- node %d : NEW TICK round %d -  %d \n\n", h.index, currentRound, h.conf.Clock.Now().Unix())
			break
		case beacon := <-winCh:
			if beacon.Round != currentRound {
				// an old round that finishes later than supposed to, we need to
				// make sure to not build upon it as other nodes may be already
				// ahead - an round that finishes after its time is not
				// considered in the chain
				break
			}
			// we signal that the round is finished and move on by waiting on
			// the next tick,i.e. proper operational flow.
			currentRound++
			prevSig = beacon.Signature
			prevRound = beacon.Round
			currentRoundFinished = true
			h.applyCallbacks(beacon)
			//fmt.Printf("\n FINISHED node %d - round %d\n\n", h.index, prevRound)
			break
		case <-h.manager.ProbablyNeedSync():
			// in this case we need to quit this main loop and start as in the catchup node
			h.l.Info("need_sync", "leaving_main_loop")
			go h.Catchup()
			return
		case <-h.close:
			//fmt.Printf("\n\t --- Beacon LOOP OUT - node pointer %p\n", h)
			h.l.Debug("beacon_loop", "finished")
			return
		}
		//}
	}
}

func (h *Handler) runRound(currentRound, prevRound uint64, prevSig []byte, winCh chan *Beacon, closeCh chan bool) {
	incomings := h.manager.NewRound(prevRound, currentRound)
	// we sign for the new current round
	msg := Message(prevSig, prevRound, currentRound)
	currSig, err := h.conf.Scheme.Sign(h.share.PrivateShare(), msg)
	if err != nil {
		h.l.Fatal("beacon_round", fmt.Sprintf("creating signature: %s", err), "round", currentRound)
		return
	}
	shortPub := h.pub.Eval(1).V.String()[14:19]
	h.l.Debug("start_round", currentRound, "time", h.conf.Clock.Now(), "from_sig", shortSigStr(prevSig), "from_round", prevRound, "msg_sign", shortSigStr(msg), "short_pub", shortPub, "handler", fmt.Sprintf("%p", h), "addr", h.conf.Private.Public.Address())
	packet := &proto.BeaconPacket{
		Round:         currentRound,
		PreviousRound: prevRound,
		PreviousSig:   prevSig,
		PartialSig:    currSig,
	}
	h.manager.NewBeacon(packet)

	// NOTE: sleep a while to not ask nodes too fast - they may have a slight bias
	// in time
	time.Sleep(h.conf.WaitTime)
	// send all requests in parallel
	h.client.SetTimeout(1 * time.Second)
	for _, id := range h.conf.Group.Nodes {
		if h.addr == id.Address() {
			continue
		}
		// this go routine sends the packet to one node. It will always
		// return assuming there's a timeout on the connection
		go func(i *key.Identity) {
			h.l.Debug("beacon_round", currentRound, "send_to", i.Address())
			_, err := h.client.NewBeacon(i, packet)
			if err != nil {
				h.l.Error("beacon_round", currentRound, "err_request", err, "from", i.Address())
				if strings.Contains(err.Error(), errOutOfRound) {
					h.l.Error("beacon_round", currentRound, "node", i.Addr, "reply", "out-of-round")
				}
				return
			}
		}(id)
	}
	// wait for a threshold of replies or if the timeout occured
	var partials [][]byte
	var finalSig []byte
	for {
		select {
		case partial := <-incomings:
			partials = append(partials, partial)
			h.l.Debug("beacon_round", currentRound, "partial_get", len(partials), "partial_want", h.group.Threshold)
		case <-closeCh:
			// it's already time to go to the next, there has been not
			// enough time or nodes are too slow. In any case it's a
			// problem.
			h.l.Error("beacon_round", currentRound, "quitting prematurely", "problem with short period or beacon nodes")
			return
		}
		if len(partials) < h.conf.Group.Threshold {
			continue
		}
		h.l.Debug("beacon_round", currentRound, "got_all_sig", fmt.Sprintf("%d/%d", len(partials), h.conf.Group.Threshold))
		//fmt.Printf("\n%d - %s got ALL signatures #1\n\n", h.index, h.conf.Private.Public.Address())
		finalSig, err = h.conf.Scheme.Recover(h.pub, msg, partials, h.group.Threshold, h.group.Len())
		if err != nil {
			h.l.Error("beacon_round", currentRound, "final-beacon-err", err)
			return
		}

		if err := h.conf.Scheme.VerifyRecovered(h.pub.Commit(), msg, finalSig); err != nil {
			h.l.Error("beacon_round", currentRound, "invalid beacon signature", err)
			return
		}
		// all is good
		break
	}

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
	shortRand := shortSigStr(beacon.Randomness())
	h.l.Info("done_round", currentRound, "signature", shortSig, "randomness", shortRand, "previous_sig", shortPrevSig)
	select {
	case <-closeCh:
		// round is already time'd out
		// XXX what do we do with the beacon just saved ? he is a valid one but
		// is a "fork"
		return
	default:
		winCh <- beacon
	}
}

// initRound & initSignature are the round & signature this node has
func (h *Handler) syncFrom(to []*key.Identity, lastBeacon *Beacon) (*Beacon, error) {
	initRound := lastBeacon.Round
	currentRound := lastBeacon.Round
	//fmt.Printf("\n node %d runs SYNCFROM --- currentRound %d\n\n", h.index, currentRound)
	currentSig := lastBeacon.Signature
	var currentBeacon = lastBeacon
	for _, id := range to {
		if h.addr == id.Addr {
			continue
		}
		//fmt.Println(" TRYING TO SYNC TO ", id.Address())
		// if node doesn't answer quickly, we move on
		//h.client.SetTimeout(1 * time.Second)
		h.l.Debug("sync_from", "try_sync", "to", id.Addr, "from_round", currentRound+1)
		//ctx, cancel := context.WithCancel(context.Background())
		ctx, cancel := context.Background(), func() {}
		request := &proto.SyncRequest{
			// we ask rounds from at least one round more than what we already
			// have
			FromRound: currentRound + 1,
		}
		respCh, err := h.client.SyncChain(ctx, id, request)
		if err != nil {
			h.l.Error("sync_from", currentRound, "error", err, "from", id.Address())
			//fmt.Println(" CAN NOT SYNC TO ", id.Address())
			continue
		}

		//fmt.Println(" LISTENING TO SYNC CHANNEL FROM ", id.Address())
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
				h.l.Error("sync_round", currentRound, "from", id.Address(), "want_prevRound", currentRound, "got_prevRound", prevRound, "want_prevSig", shortSigStr(currentSig), "got_prevSig", shortSigStr(prevSig), "got_sig", shortSigStr(syncReply.GetSignature()), "round", syncReply.GetRound())
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

			currentBeacon = beacon
			currentRound = beacon.Round
			currentSig = beacon.Signature
			// we check each time that we haven't advanced a round in the
			// syncing process
			nextRound, _ := NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
			// if it gave us the round just before the next one, then we are
			// synced!
			if currentRound+1 == nextRound {
				h.l.Debug("sync", "finished", "round", currentRound, "sig", shortSigStr(currentSig))
				cancel()
				return currentBeacon, nil
			}
		}
	}

	nextRound, _ := NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	return currentBeacon, fmt.Errorf("syncing went from %d to %d whereas current round is %d: network is down", initRound, currentRound, nextRound-1)
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
	h.manager.Stop()
	h.store.Close()
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
	//fmt.Printf(" || STOP now is %d, stopTime is %d -> will sleep %d - beacon address %p - %s\n", now, stopTime, int64(duration.Seconds()), h, h.conf.Private.Public.Address())
	h.conf.Clock.Sleep(duration)
	h.Stop()
	//fmt.Printf(" || STOP beacon address %p\n", h)
	return nil
}

var errOutdatedRound = errors.New("current partial signature not for this round")

// AddCallback adds function  that is called each time a new beacon is created
func (h *Handler) AddCallback(fn func(*Beacon)) {
	h.Lock()
	defer h.Unlock()
	h.callbacks = append(h.callbacks, fn)
}

func (h *Handler) applyCallbacks(b *Beacon) {
	h.Lock()
	defer h.Unlock()
	for _, fn := range h.callbacks {
		go fn(b)
	}
}

func shortSigStr(sig []byte) string {
	max := 3
	if len(sig) < max {
		max = len(sig)
	}
	return hex.EncodeToString(sig[0:max])
}

type partialSig struct {
	round   uint64
	partial []byte
}

type roundInfo struct {
	round uint64
	sig   []byte
}

func shuffleNodes(nodes []*key.Identity) []*key.Identity {
	ids := make([]*key.Identity, 0, len(nodes))
	for _, id := range nodes {
		ids = append(ids, id)
	}
	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	return ids
}
