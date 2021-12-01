package sync

import (
	"context"
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
}

// BeatInfo contains information relayed by the heartbeat logic. The peer who
// replied and the last beacon that he has.
type BeatInfo struct {
	Peer      *key.Identity
	LastRound uint64
}

// Heartbeat contacts other nodes in the network and relay their latest beacon
type Heartbeat interface {
	UpdateGroup([]*key.Identity)
	Beats() chan BeatInfo
	Stop()
}

type heartbeat struct {
	c        *HeartbeatConfig
	stop     chan bool
	newGroup chan []*key.Identity
	beats    chan BeatInfo
}

func NewHeartbeat(c *HeartbeatConfig) Heartbeat {
	h := &heartbeat{
		c:        c,
		stop:     make(chan bool, 1),
		newGroup: make(chan []*key.Identity, 1),
		// TODO: It might work for now but wont for bigger size
		beats: make(chan BeatInfo, 100),
	}
	go h.run()
	return h
}

func (h *heartbeat) Stop() {
	close(h.stop)
}

func (h *heartbeat) Beats() chan BeatInfo {
	return h.beats
}

func (h *heartbeat) UpdateGroup(g []*key.Identity) {
	h.newGroup <- g
}

func (h *heartbeat) run() {
	var currentGroup []*key.Identity
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
			ctx, cancel = context.WithCancel(context.Background())
			go h.fetch(ctx, currentGroup)
		case <-h.c.Clock.After(h.c.Frequency):
			if currentGroup == nil {
				h.c.Log.Debugw("heartbeat", "empty group - skipping beat()")
				continue
			}
		case <-h.stop:
			h.c.Log.Debugw("heartbeat", "stops")
		}
	}
}

func (h *heartbeat) fetch(ctx context.Context, group []*key.Identity) {
	for i := range rand.Perm(len(group)) {
		// TODO what happens when stream breaks - does it try again?
		stream, err := h.c.Client.PublicRandStream(ctx, group[i], &drand.PublicRandRequest{})
		if err != nil {
			h.c.Log.Error("heartbeat", "can not contact node", err)
			continue
		}
		go func(id *key.Identity, ch chan *drand.PublicRandResponse) {
			for rand := range ch {
				beat := BeatInfo{
					Peer:      id,
					LastRound: rand.GetRound(),
				}
				select {
				case h.beats <- beat:
					continue
				default:
					h.c.Log.Debugw("heartbeat", "beat channel full")
				}
			}
		}(group[i], stream)
	}
}
