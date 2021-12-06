package gossip

import (
	"container/ring"
	"fmt"
	"math/rand"
	"sync"

	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
)

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
	// NewPeers will make the gossiper resample a new list of peers All new
	// "Gossip()" messages will be delivered to a subset of this new list. Due
	// to asynchronous nature of gossiper, it makes no guarantee about messages
	// curently being gossiped.
	NewPeers([]net.Peer)
	Stop()
}

type Config struct {
	Log log.Logger
	// the full list of nodes gossiping will pick from - our own identity should
	// be excluded from
	List []net.Peer
	// how many nodes do we send to
	Factor int
	// used by the gossiper to send a msg
	Send func(Packet) error
	// Maximum retention buffer size. This is the maximum number of packets ID
	// we keep track of, to not re-send again. ADvice is to set it to the max number
	// of different messages you expect to see in the network in a period
	BufferSize int
	// size of the channel where we can send packets to gossip - define rate
	// limit. Advice is to set it to the number of messages you expect to gossip
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

// Packet is the generic struct that contains the origin/destination and the
// associated packet
type Packet struct {
	Peer net.Peer
	Msg  MsgWithID
}

// MsgWithID is the interface to differentiate messages between themselves.
type MsgWithID interface {
	String() string
}

type netGossip struct {
	c *Config
	// list of peers we send messages to
	neighbors *peerList
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

// pickKNeighbors returns k randomly chosen peers from the list. If us is
// non-nil, it will exclude "us" from the list of chosen list.
func pickKNeighbors(nodes []net.Peer, k int) []net.Peer {
	chosen := make([]net.Peer, 0, k)
	for _, j := range rand.Perm(len(nodes)) {
		if len(chosen) == k {
			break
		}
		chosen = append(chosen, nodes[j])
	}
	return chosen
}

func NewGossiper(c *Config) Gossiper {
	c.fillDefault()
	ng := &netGossip{
		c:         c,
		neighbors: &peerList{n: pickKNeighbors(c.List, c.Factor)},
		newToSend: make(chan MsgWithID, c.RateLimit),
		// expect BufferSize different messages to gossip
		toGossip: make(chan Packet, c.BufferSize),
		// only RateLimit deliveries max
		delivery: make(chan Packet, c.RateLimit),
		// each msg to re-gossip - only up to BufferSize
		gossiping: make(chan MsgWithID, c.RateLimit*c.Factor),
		// only send RateLimit max msg in a period
		priority: make(chan MsgWithID, c.RateLimit),
		stop:     make(chan bool, 1),
	}
	go ng.logicLoop()
	// XXX can spawn multiple of those if we want
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

func (g *netGossip) NewPeers(list []net.Peer) {
	g.neighbors.Update(pickKNeighbors(list, g.c.Factor))
}

func (g *netGossip) Stop() {
	close(g.stop)
	close(g.gossiping)
	close(g.priority)
	close(g.delivery)
}

func (g *netGossip) logicLoop() {
	set := newRingSet(g.c.BufferSize)
	for {
		select {
		case <-g.stop:
			return
		case msg := <-g.newToSend:
			// no checking if present or not = we can always resend a packet we
			// already sent recently
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
		for _, n := range g.neighbors.Nodes() {
			packet := Packet{Peer: n, Msg: msg}
			fmt.Println("SEND PACKET to -> ", n, "while neighbors: ", g.neighbors)
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

type peerList struct {
	sync.RWMutex
	n []net.Peer
}

func (p *peerList) Update(newList []net.Peer) {
	p.Lock()
	defer p.Unlock()
	p.n = newList
}

func (p *peerList) Nodes() []net.Peer {
	p.Lock()
	defer p.Unlock()
	return p.n
}
