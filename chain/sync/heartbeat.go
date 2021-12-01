package sync

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	clock "github.com/jonboulle/clockwork"
)

// HeartbeatConfig holds all the required information for the heartbeat to
// proceed.
type HeartbeatConfig struct {
	// Logger where heartbeat will log important event such as network down
	Log log.Logger
	// Clock to regulate the heartbeats
	Clock clock.Clock
	// Client used by the heartbeat logic to contact other nodes
	Client net.PublicClient
	// Frequency at which the heartbeat should contact other nodes
	Frequency time.Duration
	// Aggregator to collect and aggregate beats -
	Aggregator BeatAggregator
}

// Aggregator is used to collect all individual beats during each period and
// will output the aggregated one at each tick.
// All methods are called synchronously.
type BeatAggregator interface {
	// PushBeat is called when a new beat is received from the heartbeat logic.
	PushBeat(BeatInfo)
	// AggregatedBeats returns a struct that contains an certain aggregation of
	// all beats received - all the previous beats should probably be erased.
	AggregatedBeats() interface{}
}

// BeatInfo contains information relayed by the heartbeat logic. The peer who
// replied and the last beacon that he has.
type BeatInfo struct {
	Peer  *key.Identity
	Round uint64
}

// Heartbeat contacts other nodes in the network and relay their latest beacon
type Heartbeat interface {
	UpdateGroup([]*key.Identity)
	Beats() chan interface{}
	Stop()
}

type heartbeat struct {
	c        *HeartbeatConfig
	stop     chan bool
	newGroup chan []*key.Identity
	beats    chan BeatInfo
	aggBeats chan interface{}
	newFetch chan fetchInfo
}

// TODO: ?
const workers = 4
const maxQueueLength = 100

// MaxHeartbeatTime sets how long we'll wait after a new connection to receive
// new beacons from one peer
var MaxSyncWaitTime = 2 * time.Second

func NewHeartbeat(c *HeartbeatConfig) Heartbeat {
	h := &heartbeat{
		c:        c,
		stop:     make(chan bool, 1),
		newGroup: make(chan []*key.Identity, 1),
		// TODO: It might work for now but wont for bigger size
		beats:    make(chan BeatInfo, 1),
		aggBeats: make(chan interface{}, 1),
		newFetch: make(chan fetchInfo, maxQueueLength),
	}
	go h.run()
	for i := 0; i < workers; i++ {
		go h.worker()
	}
	return h
}

func (h *heartbeat) Stop() {
	close(h.stop)
}

func (h *heartbeat) Beats() chan interface{} {
	return h.aggBeats
}

func (h *heartbeat) UpdateGroup(g []*key.Identity) {
	h.newGroup <- g
}

func (h *heartbeat) run() {
	var currentGroup []*key.Identity
	var agg = h.c.Aggregator
	ctx, cancel := context.WithCancel(context.Background())
	for {
		select {
		case ng := <-h.newGroup:
			if ng == nil {
				h.c.Log.Error("heartbeat", "received nil group")
				continue
			}
			currentGroup = ng
			cancel() // we cancel current sync operations
		case beat := <-h.beats:
			agg.PushBeat(beat)
		case <-h.c.Clock.After(h.c.Frequency):
			if currentGroup == nil {
				h.c.Log.Debugw("heartbeat", "empty group - skipping beat()")
				continue
			}
			aggBeat := agg.AggregatedBeats()
			select {
			case h.aggBeats <- aggBeat:
				h.c.Log.Debugw("heartbeat", fmt.Sprintf("tick aggregated beat %v", aggBeat))
			default:
				h.c.Log.Debugw("heartbeat", "aggregated heartbeat full")
			}
			ctx, cancel = context.WithCancel(context.Background())
			// launh a new fetch
			for i := range rand.Perm(len(currentGroup)) {
				h.newFetch <- fetchInfo{ctx: ctx, id: currentGroup[i]}
			}
		case <-h.stop:
			h.c.Log.Debugw("heartbeat", "stops")
		}
	}
}

type fetchInfo struct {
	ctx context.Context
	id  *key.Identity
}

func (h *heartbeat) worker() {
	select {
	case fi := <-h.newFetch:
		h.fetch(fi)
	case <-h.stop:
		return
	}
}

func (h *heartbeat) fetch(fi fetchInfo) {
	// only wait a certain time for each trial
	localCtx, cancel := context.WithTimeout(fi.ctx, MaxSyncWaitTime)
	defer cancel()
	// TODO what happens when stream breaks - does it try again?
	r, err := h.c.Client.PublicRand(localCtx, fi.id, &drand.PublicRandRequest{})
	if err != nil {
		h.c.Log.Error("heartbeat", "can not contact node", err)
		return
	}
	beat := BeatInfo{
		Peer:  fi.id,
		Round: r.GetRound(),
	}
	select {
	case h.beats <- beat:
	default:
		h.c.Log.Debugw("heartbeat", "beat channel full")
	}
}

type maxRoundAgg struct {
	max uint64
}

// NewMaxAggregator returns an aggregator that stores the highest round seen on
// the network per beat. It returns an uint64.
func NewMaxAggregator() BeatAggregator {
	return &maxRoundAgg{}
}

func (m *maxRoundAgg) PushBeat(b BeatInfo) {
	if m.max < b.Round {
		m.max = b.Round
	}
}

func (m *maxRoundAgg) AggregatedBeats() interface{} {
	max := m.max
	m.max = 0
	return max
}
