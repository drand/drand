package gossip

import (
	"bytes"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/stretchr/testify/require"
)

type network struct {
	nodes  []*node
	sendCb func(deliveredPacket)
}

func (n *network) Send(p Packet) error {
	for _, node := range n.nodes {
		if node.Address() != p.Peer.Address() {
			continue
		}
		node.receivePacket(p)
		if n.sendCb != nil {
			n.sendCb(deliveredPacket{n: node, p: p})
		}
	}
	return nil
}

func NewNetwork(n int, factor, bufferSize, rateLimit int) *network {
	l := log.DefaultLogger()
	peers := make([]net.Peer, n)
	for i := 0; i < n; i++ {
		peers[i] = net.CreatePeer(fmt.Sprintf("node-%d", i), true)
	}
	net := new(network)
	nodes := make([]*node, n)
	for i := 0; i < n; i++ {
		nodes[i] = &node{
			Peer: peers[i],
			Gossiper: NewGossiper(&Config{
				Log:        l,
				Factor:     factor,
				Neighboors: PickKNeighbors(peers, factor, peers[i]),
				Send:       net.Send,
				BufferSize: bufferSize,
				RateLimit:  rateLimit,
			}),
		}
	}
	net.nodes = nodes
	return net
}

type deliveredPacket struct {
	n *node
	p Packet
}

func (n *network) collectAllDelivered(stop chan bool) chan deliveredPacket {
	globalChan := make(chan deliveredPacket, len(n.nodes))
	for i := range n.nodes {
		go func(nn *node) {
			for {
				select {
				case p := <-nn.Gossiper.Delivery():
					globalChan <- deliveredPacket{
						n: nn,
						p: p,
					}
				case <-stop:
					return
				}
			}
		}(n.nodes[i])
	}
	return globalChan
}

func (n *network) stop() {
	for _, node := range n.nodes {
		node.Stop()
	}
}

type node struct {
	net.Peer
	Gossiper
	sync.Mutex
	// packets delivered to this node before gossip sees them
	packets []Packet
	// packets delivered by gossip
	delivered []Packet
}

func (n *node) receivePacket(p Packet) {
	n.Lock()
	defer n.Unlock()
	n.packets = append(n.packets, p)
	n.NewIncoming(p) // should be origin address here but it doesnt matter in test
}

type message = bytes.Buffer

func TestGossipNormal(t *testing.T) {
	n := 20
	factor := 5
	network := NewNetwork(n, factor, n, 0)
	defer network.stop()
	gossipCh := make(chan deliveredPacket, n)
	network.sendCb = func(d deliveredPacket) {
		gossipCh <- d
	}
	// make first node gossip a message
	msg := new(message)
	msg.WriteString("Hello Gossipers")
	stop := make(chan bool, 1)
	global := network.collectAllDelivered(stop)
	go network.nodes[0].Gossip(msg)
	// make sure that every node has been delivered the message and only once
	var rcvdPacket = make(map[string]bool)
	// make sure that every node received the packet from every of their peers
	var gossipPacket = make(map[string]int)
	// each peer should receive a certain number of packets when gossiping a
	// packet - it's not always "factor" because the choice is random
	var expGossip = make(map[string]int)
	for _, n := range network.nodes {
		chosen := n.Gossiper.(*netGossip).c.Neighboors
		for _, n2 := range chosen {
			expGossip[n2.Address()] += 1
		}
	}

	enoughDelivered := func() bool { return len(rcvdPacket) == n-1 }
	enoughGossiped := func() bool {
		if len(gossipPacket) < n {
			return false
		}
		for n, exp := range expGossip {
			if gossipPacket[n] != exp {
				return false
			}
		}
		return true
	}
	for !enoughDelivered() && !enoughGossiped() {
		select {
		case dp := <-global:
			if _, ok := rcvdPacket[dp.n.Address()]; ok {
				require.True(t, false, "node got delivered same packet twice")
			}
			rcvdPacket[dp.n.Address()] = true
		case dp := <-gossipCh:
			gossipPacket[dp.n.Address()] += 1
		case <-time.After(5 * time.Second):
			close(stop)
			require.True(t, false, "no messages received anymore %d", len(rcvdPacket))
		}
	}
}

func TestPickNeighbors(t *testing.T) {
	n := 10
	peers := make([]net.Peer, n)
	for i := 0; i < n; i++ {
		peers[i] = net.CreatePeer(fmt.Sprintf("node-%d", i), true)
	}
	factor := 5
	require.Len(t, PickKNeighbors(peers, factor, nil), factor)
	require.Len(t, PickKNeighbors(peers, factor, peers[0]), factor)
	require.Len(t, PickKNeighbors(peers, factor, peers[2]), factor)
	require.NotContains(t, PickKNeighbors(peers, factor, peers[2]), peers[2])
}

func TestRingSet(t *testing.T) {
	n := 10
	s := newRingSet(n)
	require.True(t, s.tryInsert("1"))
	require.False(t, s.tryInsert("1"))
	// check that entries gets overwritten- we writethe next n entries
	for i := 2; i < n+2; i++ {
		require.True(t, s.tryInsert(strconv.Itoa(i)))
	}
	// given there is n new entries, 1 should be a new stored entry
	require.True(t, s.tryInsert("1"))
}
