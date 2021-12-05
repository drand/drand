package gossip

import (
	"container/ring"
	"fmt"
	"math/rand"

	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
)

type Config struct {
	Log log.Logger
	// the list of neighboors we're talking to
	Neighboors []net.Peer
	// how many nodes do we send to
	Factor int
	// used by the gossiper to send a msg
	Send func(Packet) error
	// Maximum retention buffer size. This is the maximum number of packets ID
	// we keep track of, to not re-send again. ADvice is to set it to the max number
	// of messages you expect to receive in a short period.
	BufferSize int
	// size of the channel where we can send packets to gossip - define rate
	// limit. Advice is to set it to the number of messages you expect to send
	// in a short period.
	RateLimit int
}

const defaultFactor = 12
const defaultBufferSize = 100
const defaultRateLimit = 20

func (c *Config) fillDefault() {
	if c.Factor == 0 {
		c.Factor = defaultFactor
	}
	if c.BufferSize == 0 {
		c.BufferSize = defaultBufferSize
	}
	if c.RateLimit == 0 {
		c.RateLimit = defaultRateLimit
	}
}

type Packet struct {
	Peer net.Peer
	Msg  MsgWithID
}

type MsgWithID interface {
	String() string
}

type Gossiper interface {
	// Send a new message through the gossip network. calling twice with same
	// message will simply send the packet again, but it will not be gossiped by
	// others (unless after some time where the first message is
	// "dropped" by the gossiper).
	Gossip(MsgWithID)
	// Method to call to pass messages to the gossiper
	NewIncoming(Packet)
	// Delivery is a chan that returns the packet ready for application
	// processing
	Delivery() chan Packet
	Stop()
}

type netGossip struct {
	c *Config
	// channel that receives the msgs to gossip from application
	newToSend chan MsgWithID
	// channel that receives the msgs from other nodes in gossip layer
	toGossip chan Packet
	// channel that delivers packet to the application
	delivery chan Packet
	// internal channel that receives packet to send to network and actually
	// sends them
	gossiping chan MsgWithID
	// priority channel queue when application wants to send msg to network: it
	// is sent immediatly, and does not need to wait before the rest of the
	// gossiping messages are sent
	priority chan MsgWithID
	stop     chan bool
}

// PickKNeighbors returns k randomly chosen peers from the list. If us is
// non-nil, it will exclude "us" from the list of chosen list.
func PickKNeighbors(nodes []net.Peer, k int, us net.Peer) []net.Peer {
	chosen := make([]net.Peer, 0, k)
	for _, j := range rand.Perm(len(nodes)) {
		if len(chosen) == k {
			break
		}
		if us != nil && us.Address() == nodes[j].Address() {
			continue
		}
		chosen = append(chosen, nodes[j])
	}
	return chosen
}

func NewGossiper(c *Config) Gossiper {
	c.fillDefault()
	ng := &netGossip{
		c: c,

		newToSend: make(chan MsgWithID, c.RateLimit),
		toGossip:  make(chan Packet, c.RateLimit*c.Factor),
		delivery:  make(chan Packet, c.RateLimit),
		gossiping: make(chan MsgWithID, c.RateLimit*c.Factor),
		priority:  make(chan MsgWithID, c.RateLimit),
		stop:      make(chan bool, 1),
	}
	go ng.logicLoop()
	go ng.sendLoop(ng.gossiping)
	go ng.sendLoop(ng.priority)
	return ng
}

func (g *netGossip) Gossip(i MsgWithID) {
	g.newToSend <- i
}

func (g *netGossip) NewIncoming(p Packet) {
	g.toGossip <- p
}

func (g *netGossip) Delivery() chan Packet {
	return g.delivery
}

func (g *netGossip) Stop() {
	close(g.stop)
	close(g.gossiping)
	close(g.priority)
}

func (g *netGossip) logicLoop() {
	set := newRingSet(g.c.BufferSize)
	for {
		select {
		case <-g.stop:
			return
		case msg := <-g.newToSend:
			set.tryInsert(msg.String())
			g.priority <- msg
		case p := <-g.toGossip:
			// check if we havent received it yet
			isNew := set.tryInsert(p.Msg.String())
			if !isNew {
				continue
			}
			// deliver and gossip
			g.delivery <- p
			// try to gossip - if we can't we should continue the logic loop
			// continue sowe can always send packets out on the priority lane.
			// In these cases, it is acceptable to drop a packet meant to be for
			// gossiping
			select {
			case g.gossiping <- p.Msg:
			default:
				g.c.Log.Debug("gossip", "gossip channel full")
			}
		}
	}
}

func (g *netGossip) sendLoop(over chan MsgWithID) {
	// for each packet, we send to the neighbors
	for msg := range over {
		for _, n := range g.c.Neighboors {
			packet := Packet{Peer: n, Msg: msg}
			fmt.Println("sending packet", packet)
			g.c.Send(packet)
		}
	}
}

type id = string

// set is a simple interface to keep tracks of all the packet hashes that we
type set interface {
	// stores the given id in the set if it is not presetn already. It returns
	// false if it was already present, true otherwise.
	tryInsert(id) bool
}

type ringSet struct {
	set      map[id]bool
	arrivals *ring.Ring
	max      int
}

func newRingSet(size int) set {
	return &ringSet{
		set:      make(map[id]bool),
		arrivals: ring.New(size),
		max:      size,
	}
}

func (f *ringSet) tryInsert(i id) bool {
	_, ok := f.set[i]
	if ok {
		return false
	}
	// check the length if we need to adjust
	if len(f.set)+1 > f.max {
		oldest := f.arrivals.Value.(id)
		delete(f.set, oldest)
	}
	// register the new entry
	f.set[i] = true
	// replace the oldest value by the newest
	f.arrivals.Value = i
	// go the next oldest value,first in the ring now
	f.arrivals = f.arrivals.Next()
	return true
}
