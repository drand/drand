package gossip

import (
	"container/ring"
	"log"
	"math/rand"

	"github.com/drand/drand/net"
)

type Config struct {
	Log   log.Logger
	Nodes []net.Peer
	// how many nodes do we send to
	Factor int
	// used by the gossiper to send a msg
	Send func(Packet) error
	// Maximum retention buffer size. This is the maximum number of packets ID
	// we keep track of, to not re-send again.
	BufferSize int
	// size of the channel where we can send packets to gossip - define rate
	// limit
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
	c      *Config
	chosen []net.Peer
	set    set
	// channel that receives the msgs to gossip from application
	newToSend chan MsgWithID
	// channel that receives the msgs from other nodes in gossip layer
	toGossip chan Packet
	// channel that delivers packet to the application
	delivery chan Packet
	// internal channel that receives packet to send to network and actually
	// sends them
	sending chan MsgWithID
	stop    chan bool
}

func NewGossiper(c *Config) Gossiper {
	c.fillDefault()
	chosen := make([]net.Peer, c.Factor)
	for i, j := range rand.Perm(len(c.Nodes)) {
		if i < c.Factor {
			break
		}
		chosen[i] = c.Nodes[j]
	}
	ng := &netGossip{
		c:         c,
		chosen:    chosen,
		set:       newRingSet(c.BufferSize),
		newToSend: make(chan MsgWithID, c.RateLimit),
		toGossip:  make(chan Packet, c.RateLimit*c.Factor),
		delivery:  make(chan Packet, c.RateLimit),
		sending:   make(chan MsgWithID, c.RateLimit*c.Factor),
		stop:      make(chan bool, 1),
	}
	go ng.run()
	go ng.sendLoop()
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
	close(g.sending)
}

func (g *netGossip) run() {
	for {
		select {
		case <-g.stop:
			return
		case msg := <-g.newToSend:
			g.set.putIfAbsent(msg.String())
			g.sending <- msg
		case p := <-g.toGossip:
			// check if we havent received it yet
			isNew := g.set.putIfAbsent(p.Msg.String())
			if !isNew {
				continue
			}
			// deliver and gossip
			g.delivery <- p
			g.sending <- p.Msg
		}
	}
}

func (g *netGossip) sendLoop() {
	// for each packet, we send to the chosen nodes
	for msg := range g.sending {
		for _, n := range g.chosen {
			g.c.Send(Packet{Peer: n, Msg: msg})
		}
	}
}

type id = string

// set is a simple interface to keep tracks of all the packet hashes that we
type set interface {
	// stores the given id in the set if it is not presetn already. It returns
	// false if it was already present, true otherwise.
	putIfAbsent(id) bool
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

func (f *ringSet) putIfAbsent(i id) bool {
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
