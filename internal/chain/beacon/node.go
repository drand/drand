package beacon

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	clock "github.com/jonboulle/clockwork"
	"go.opentelemetry.io/otel/attribute"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/common/tracer"
	"github.com/drand/drand/v2/crypto/vault"
	"github.com/drand/drand/v2/internal/chain"
	"github.com/drand/drand/v2/internal/metrics"
	"github.com/drand/drand/v2/internal/net"
	proto "github.com/drand/drand/v2/protobuf/drand"
)

// Config holds the different cryptographic information necessary to run the
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
	// keeps the cryptographic info (group share etc.)
	crypto *vault.Vault
	// main logic that treats incoming packet / new beacons created
	chain            *chainStore
	ticker           *ticker
	thresholdMonitor *metrics.ThresholdMonitor

	ctx       context.Context
	ctxCancel context.CancelFunc
	addr      string
	// a handler is running when its main run method is launched
	running bool
	// a handler becomes serving once its ticker starts ticking, but not necessarily if catching up
	serving bool
	// a handler is really stopped only once
	stopped bool
	version common.Version
	l       log.Logger
}

// NewHandler returns a fresh handler ready to serve and create randomness
// beacon.
//
//nolint:lll // This is a complex function
func NewHandler(ctx context.Context, c net.ProtocolClient, s chain.Store, conf *Config, l log.Logger, version common.Version) (*Handler, error) {
	ctx, span := tracer.NewSpan(ctx, "NewHandler")
	defer span.End()

	if conf.Share == nil || conf.Group == nil {
		err := errors.New("beacon: invalid configuration")
		span.RecordError(err)
		return nil, err
	}
	// Checking we are in the group
	node := conf.Group.Find(conf.Public.Identity)
	if node == nil {
		err := errors.New("beacon: keypair not included in the given group")
		span.RecordError(err)
		return nil, err
	}
	addr := conf.Public.Address()

	v := vault.NewVault(l, conf.Group, conf.Share, conf.Group.Scheme)
	// insert genesis beacon
	if err := s.Put(ctx, chain.GenesisBeacon(conf.Group.GenesisSeed)); err != nil {
		span.RecordError(err)
		return nil, err
	}

	ticker := newTicker(conf.Clock, conf.Group.Period, conf.Group.GenesisTime)
	store, err := newChainStore(ctx, l, conf, c, v, s, ticker)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	handler := &Handler{
		conf:             conf,
		client:           c,
		crypto:           v,
		chain:            store,
		ticker:           ticker,
		addr:             addr,
		ctx:              ctx,
		ctxCancel:        ctxCancel,
		l:                l,
		version:          version,
		thresholdMonitor: metrics.NewThresholdMonitor(conf.Group.ID, l, conf.Group.Len(), conf.Group.Threshold),
	}
	return handler, nil
}

// ProcessPartialBeacon receives a request for a beacon partial signature. It
// forwards it to the round manager if it is a valid beacon.
func (h *Handler) ProcessPartialBeacon(ctx context.Context, p *proto.PartialBeaconPacket) (*proto.Empty, error) {
	ctx, span := tracer.NewSpan(ctx, "h.ProcessPartialBeacon")
	defer span.End()

	addr := net.RemoteAddress(ctx)
	pRound := p.GetRound()
	h.l.Debugw("Processing PartialBeacon", "from", addr, "round", pRound)

	nextRound, _ := common.NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	currentRound := nextRound - 1

	// we allow one round off in the future because of small clock drifts
	// possible, if a node receives a packet very fast just before his local
	// clock passed to the next round
	if pRound > nextRound {
		h.l.Errorw("ignoring future partial", "from", addr, "round", pRound, "current_round", currentRound)
		return nil, fmt.Errorf("invalid round: %d instead of %d", pRound, currentRound)
	}

	// we don't want to process partials for beacons that we've already stored.
	if latest, err := h.chain.Last(ctx); err == nil && pRound <= latest.GetRound() {
		h.l.Debugw("ignoring past partial", "from", addr, "round", pRound, "current_round", currentRound, "latestStored", latest.GetRound())
		span.RecordError(fmt.Errorf("invalid past partial"))
		return new(proto.Empty), nil
	}

	span.AddEvent("h.crypto.DigestBeacon")
	msg := h.crypto.DigestBeacon(&common.Beacon{Round: pRound, PreviousSig: p.GetPreviousSignature()})
	span.AddEvent("h.crypto.DigestBeacon - done")

	idx, _ := h.crypto.ThresholdScheme.IndexOf(p.GetPartialSig())
	if idx < 0 {
		err := fmt.Errorf("invalid index %d in partial with msg %v partial_round %v", idx, msg, pRound)
		span.RecordError(err)
		h.l.Errorw("error", "err", err)
		return nil, err
	}

	node := h.crypto.GetGroup().Node(uint32(idx))
	if node == nil {
		err := fmt.Errorf("attempted to process beacon from node of index %d, but it was not in the group file", uint32(idx))
		span.RecordError(err)
		h.l.Errorw("error", "err", err)
		return nil, err
	}

	nodeName := node.Address()
	if nodeName == h.addr {
		h.l.Warnw("received a partial with our own index", "partial", pRound, "from", addr)
		return nil, fmt.Errorf("invalid self index %d in partial with msg %v partial_round %v", idx, msg, pRound)
	}

	// verify if request is valid
	span.AddEvent("h.crypto.ThresholdScheme.VerifyPartial")
	err := h.crypto.ThresholdScheme.VerifyPartial(h.crypto.GetPub(), msg, p.GetPartialSig())
	span.AddEvent("h.crypto.ThresholdScheme.VerifyPartial - done")

	if err != nil {
		h.l.Errorw("",
			"process_partial", addr, "err", err,
			"prev_sig", shortSigStr(p.GetPreviousSignature()),
			"curr_round", currentRound,
			"partial_round", pRound,
			"msg_sign", shortSigStr(msg),
			"from_idx", idx,
			"from_node", nodeName)
		span.RecordError(err)
		return nil, err
	}

	h.l.Debugw("",
		"process_partial", addr,
		"prev_sig", shortSigStr(p.GetPreviousSignature()),
		"curr_round", currentRound,
		"partial_round", pRound,
		"msg_sign", shortSigStr(msg),
		"from_node", nodeName,
		"status", "OK")

	if idx == h.crypto.Index() {
		h.l.Errorw("",
			"process_partial", addr,
			"index_got", idx,
			"index_our", h.crypto.Index(),
			"advance_packet", pRound,
			"from_node", nodeName)
		// TODO error or not ?
		return new(proto.Empty), nil
	}

	h.chain.NewValidPartial(ctx, addr, p)
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
func (h *Handler) Start(ctx context.Context) error {
	_, span := tracer.NewSpan(ctx, "h.Handler")
	defer span.End()
	if h.IsStopped() {
		return fmt.Errorf("a stopped handler cannot be re-started")
	}

	if h.conf.Clock.Now().Unix() > h.conf.Group.GenesisTime {
		h.l.Errorw("", "genesis_time", "past", "call", "catchup")
		return errors.New("beacon: genesis time already passed. Call Catchup()")
	}

	h.thresholdMonitor.Start()
	_, tTime := common.NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	h.l.Infow("", "beacon", "start", "scheme", h.crypto.Name)
	go h.run(tTime)

	return nil
}

// Catchup waits the next round's time to participate. This method is called
// when a node stops its daemon (maintenance or else) and get backs in the
// already running network . If the node does not have the previous randomness,
// it syncs its local chain with other nodes to be able to participate in the
// next upcoming round.
func (h *Handler) Catchup(ctx context.Context) {
	ctx, span := tracer.NewSpan(ctx, "h.Catchup")
	defer span.End()

	nRound, tTime := common.NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	h.thresholdMonitor.Start()
	go h.run(tTime)
	h.l.Infow("Launching Catchup", "upto", nRound)
	h.chain.RunSync(ctx, nRound, nil)
}

// Transition makes this beacon continuously sync until the time written in the
// "TransitionTime" in the handler's group file, where he will start generating
// randomness. To sync, he contacts the nodes listed in the previous group file
// given.
func (h *Handler) Transition(ctx context.Context, prevGroup *key.Group) error {
	ctx, span := tracer.NewSpan(ctx, "h.Transition")
	defer span.End()

	targetTime := h.conf.Group.TransitionTime
	tRound := common.CurrentRound(targetTime, h.conf.Group.Period, h.conf.Group.GenesisTime)
	tTime := common.TimeOfRound(h.conf.Group.Period, h.conf.Group.GenesisTime, tRound)
	if tTime != targetTime {
		h.l.Fatalw("", "transition_time", "invalid_offset", "expected_time", tTime, "got_time", targetTime)
		return nil
	}

	go h.run(targetTime)

	// we run the sync up until (inclusive) one round before the transition
	h.l.Debugw("", "new_node", "following chain", "to_round", tRound-1)
	//nolint:govet // We don't want to call the cancel explicitly, it's not lost we're relying on the deadline
	ctx, _ = context.WithDeadline(ctx, time.Unix(targetTime, 0).Add(-h.conf.Group.Period))
	h.chain.RunSync(ctx, tRound-1, toPeers(prevGroup.Nodes))

	return nil
}

// TransitionNewGroup prepares the node to transition to the new group
func (h *Handler) TransitionNewGroup(ctx context.Context, newShare *key.Share, newGroup *key.Group) {
	if h == nil {
		return
	}

	_, span := tracer.NewSpan(ctx, "h.TransitionNewGroup")
	defer span.End()

	targetTime := newGroup.TransitionTime
	tRound := common.CurrentRound(targetTime, h.conf.Group.Period, h.conf.Group.GenesisTime)
	tTime := common.TimeOfRound(h.conf.Group.Period, h.conf.Group.GenesisTime, tRound)
	if tTime != targetTime {
		h.l.Fatalw("", "transition_time", "invalid_offset", "expected_time", tTime, "got_time", targetTime)
		return
	}
	h.l.Infow("Preparing transition to new group", "at_round", tRound)
	// register a callback such that when the round happening just before the
	// transition is stored, then it switches the current share to the new one
	targetRound := tRound - 1
	h.chain.AddCallback("transition", func(b *common.Beacon, closed bool) {
		if closed ||
			b.Round < targetRound {
			return
		}
		h.crypto.SetInfo(newGroup, newShare)
		h.thresholdMonitor.Update(newGroup.Threshold, newGroup.Len())
		h.chain.RemoveCallback("transition")
	})
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

// run will wait until it is supposed to start
func (h *Handler) run(startTime int64) {
	// we cannot re-start a stopped handler
	if h.IsStopped() {
		return
	}
	h.Lock()
	h.running = true
	h.Unlock()

	chanTick := h.ticker.ChannelAt(startTime)
	h.l.Infow("starting handler run", "startTime", startTime, "current time", h.conf.Clock.Now().Unix())

	var current roundInfo
	setServing := sync.Once{}

	for {
		select {
		case <-h.ctx.Done():
			h.l.Debugw("", "beacon_loop", "finished", "err", h.ctx.Err())
			return
		case current = <-chanTick:
			func() {
				ctx, span := tracer.NewSpan(h.ctx, "h.run.chanTick")
				defer span.End()
				span.SetAttributes(
					attribute.Int64("round", int64(current.round)),
				)

				setServing.Do(func() {
					h.Lock()
					h.serving = true
					h.Unlock()
				})

				lastBeacon, err := h.chain.Last(ctx)
				if err != nil {
					span.RecordError(err)
					h.l.Errorw("", "beacon_loop", "loading_last", "err", err)
					return
				}
				h.l.Debugw("", "beacon_loop", "new_round", "round", current.round, "lastbeacon", lastBeacon.Round)
				h.broadcastNextPartial(ctx, current, lastBeacon)
				// if the next round of the last beacon we generated is not the round we
				// are now, that means there is a gap between the two rounds. In other
				// words, the chain has halted for that amount of rounds or our
				// network is not functioning properly.
				if lastBeacon.Round+1 < current.round {
					// We also launch a sync with the other nodes. If there is one node
					// that has a higher beacon, we'll build on it next epoch. If
					// nobody has a higher beacon, then this one will be next if the
					// network conditions allow for it.
					// TODO find a way to start the catchup as soon as the runsync is
					// done. Not critical but leads to faster network recovery.
					h.l.Debugw("", "beacon_loop", "run_sync_catchup", "last_is", lastBeacon, "should_be", current.round)
					h.chain.RunSync(ctx, current.round, nil)
				}
			}()
		case b := <-h.chain.AppendedBeaconNoSync():
			ctx, span := tracer.NewSpan(h.ctx, "h.run.appendBeaconNoSync")
			span.SetAttributes(
				attribute.Int64("round", int64(b.Round)),
			)

			h.l.Debugw("", "beacon_loop", "catchupmode", "last_is", b.Round, "current", current.round, "catchup_launch", b.Round < current.round)
			if b.Round < current.round {
				// When network is down, all alive nodes will broadcast their
				// signatures periodically with the same period. As soon as one
				// new beacon is created,i.e. network is up again, this channel
				// will be triggered and we enter fast mode here.
				// Since that last node is late, nodes must now hurry up to do
				// the next beacons in time -> we run the next beacon now
				// already. If that next beacon is created soon after, this
				// channel will trigger again etc. until we arrive at the correct
				// round.
				go func(c roundInfo, latest common.Beacon) {
					defer span.End()

					h.l.Debugw("sleeping now", "beacon_loop", "catchupmode",
						"last_is", latest.Round,
						"sleep_for", h.conf.Group.CatchupPeriod)

					h.conf.Clock.Sleep(h.conf.Group.CatchupPeriod)

					select {
					case <-ctx.Done():
						return
					default:
					}

					h.l.Debugw("broadcast next partial", "beacon_loop", "catchupmode",
						"last_is", latest.Round)
					h.broadcastNextPartial(ctx, c, &latest)
				}(current, *b)
			} else {
				span.End()
			}
		}
	}
}

func (h *Handler) broadcastNextPartial(ctx context.Context, current roundInfo, upon *common.Beacon) {
	ctx, span := tracer.NewSpan(ctx, "h.broadcastNextPartial")
	defer span.End()

	previousSig := upon.Signature
	round := upon.Round + 1
	beaconID := common.GetCanonicalBeaconID(h.conf.Group.ID)
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

	msg := h.crypto.DigestBeacon(&common.Beacon{
		Round:       round,
		PreviousSig: previousSig,
	})

	currSig, err := h.crypto.SignPartial(msg)
	if err != nil {
		span.RecordError(err)
		h.l.Fatalw("err creating partial signature", "err", err, "round", round)
		return
	}
	h.l.Debugw("", "broadcast_partial", round, "prev_sig", shortSigStr(previousSig), "msg_sign", shortSigStr(msg))
	metadata := proto.NewMetadata(h.version.ToProto())
	metadata.BeaconID = beaconID

	packet := &proto.PartialBeaconPacket{
		Round:             round,
		PreviousSignature: previousSig,
		PartialSig:        currSig,
		Metadata:          metadata,
	}

	h.chain.NewValidPartial(ctx, h.addr, packet)
	for _, id := range h.crypto.GetGroup().Nodes {
		select {
		case <-ctx.Done():
			return
		default:
		}

		idt := id.Identity
		if h.addr == id.Address() {
			continue
		}
		go func(i key.Identity) {
			ctx, span := tracer.NewSpan(ctx, "h.broadcastNextPartial.SendTo")
			defer span.End()

			select {
			case <-ctx.Done():
				return
			default:
			}

			h.l.Debugw("sending partial", "round", round, "to", i.Address())
			span.SetAttributes(
				attribute.Int64("round", int64(round)),
				attribute.String("addr", i.Address()),
			)

			err := h.client.PartialBeacon(ctx, &i, packet)
			if err != nil {
				h.thresholdMonitor.ReportFailure(beaconID, i.Address())
				span.RecordError(err)
				h.l.Errorw("error sending partial", "round", round, "err", err, "to", i.Address())
				return
			}
			metrics.SuccessfulPartial(beaconID, i.Address())
		}(*idt)
	}
}

// Stop the beacon loop from aggregating  further randomness, but it
// finishes the one it is aggregating currently.
func (h *Handler) Stop(ctx context.Context) {
	_, span := tracer.NewSpan(ctx, "h.Stop")
	defer span.End()

	h.Lock()
	defer h.Unlock()
	if h.stopped {
		return
	}
	h.ctxCancel()

	h.ticker.Stop()
	h.chain.Stop()
	h.thresholdMonitor.Stop()

	h.stopped = true
	h.running = false
	h.l.Infow("beacon handler stopped", "time", h.conf.Clock.Now())
}

// StopAt will stop the handler at the given time. It is useful when
// transitioning for a resharing.
func (h *Handler) StopAt(ctx context.Context, stopTime int64) error {
	ctx, span := tracer.NewSpan(ctx, "h.StopAt")
	defer span.End()

	now := h.conf.Clock.Now().Unix()

	if stopTime <= now {
		// actually we can stop in the present but with "Stop"
		return errors.New("can't stop in the past or present")
	}
	duration := time.Duration(stopTime-now) * time.Second

	h.l.Debug("stop_at", stopTime, "sleep_for", duration.Seconds())
	h.conf.Clock.Sleep(duration)
	h.Stop(ctx)
	return nil
}

// AddCallback is a proxy method to register a callback on the backend store
func (h *Handler) AddCallback(ctx context.Context, id string, fn CallbackFunc) {
	_, span := tracer.NewSpan(ctx, "h.AddCallback")
	defer span.End()

	h.chain.AddCallback(id, fn)
}

// RemoveCallback is a proxy method to remove a callback on the backend store
func (h *Handler) RemoveCallback(ctx context.Context, id string) {
	_, span := tracer.NewSpan(ctx, "h.RemoveCallback")
	defer span.End()

	h.chain.RemoveCallback(id)
}

// GetConfg returns the conf used by the handler
func (h *Handler) GetConfg(ctx context.Context) *Config {
	_, span := tracer.NewSpan(ctx, "h.GetConfg")
	defer span.End()

	return h.conf
}

// ValidateChain asks the chain store to ask the sync manager to check the chain store up to the given beacon,
// in order to find invalid beacons and it returns the list of round numbers for which the beacons
// were corrupted / invalid / not found in the store.
// Note: it does not attempt to correct or fetch these faulty beacons.
func (h *Handler) ValidateChain(ctx context.Context, upTo uint64, cb func(r, u uint64)) ([]uint64, error) {
	ctx, span := tracer.NewSpan(ctx, "h.ValidateChain")
	defer span.End()

	return h.chain.ValidateChain(ctx, upTo, cb)
}

// CorrectChain tells the sync manager to fetch the invalid beacon from its peers.
func (h *Handler) CorrectChain(ctx context.Context, faultyBeacons []uint64, peers []net.Peer, cb func(r, u uint64)) error {
	ctx, span := tracer.NewSpan(ctx, "h.CorrectChain")
	defer span.End()

	return h.chain.RunReSync(ctx, faultyBeacons, peers, cb)
}

func shortSigStr(sig []byte) string {
	maxi := 3
	if len(sig) < maxi {
		maxi = len(sig)
	}
	return hex.EncodeToString(sig[0:maxi])
}
