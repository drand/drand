package core

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"

	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	proto "github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/share/dkg"
)

// broadcastChannel implements  a flavor of the broadcast channel from point to
// point channel protocol described in https://eprint.iacr.org/2011/535.pdf.
// When a participant wants to broadcast a message, it sends it, signed, to all
// other participants. Each participant then rebroadcast the message in turn.
// When a participant received the same message from more than X peers, it
// passes it up to the application (in this case, the DKG library).
// X in this specific case is dependent of the phase of the DKG:
//  - deal phase: X = threshold of share holders since the share holders must have had
//  received the same deals from all dealers
//  - response phase: X = threshold of dealers + threshold of share holders
//  since both group must have the same consistent view of the responses of the share
//  holders
//  - justification phase: X = threshold of share holders since only the share
//  holders care about processing the justifications to finish the protocol.
// This implementation randmizes the order of nodes to which send the packet to
// each time (see below for more information).
//
// Note however, the protocol when naively implemented is susceptible to to
// amplification types of vulnerability where one node simply sends many packets
// that each produces O(n^2) traffic, leading to a potential DOS of the network.
// This implementation accepts to rebroadcast different messages for the same
// origin for the same phases, up to a fixed number of times (number of nodes).
// The reason why the implementation doesn't restrict to
// one node is that attacker can abuse of this to split the nodes into two: once
// a group of honest nodes has accepted a message (i.e. it received the same
// message from a threshold of nodes), it will not participate to
// rebroadcast another one and therefore the group will not be able to tell the
// DKG library that a given node is misbehaving.
//
// NOTE: this implementation while in practice is believed to be sufficient, in theory
// does not prevent a certain kind of attacks that happens on the boundary of a
// phase transition. Namely, if malicious actor sends a one packet p1 to two
// honest nodes and another packet p2 to two other honest nodes at the same
// time, _just_ before the next transition happen, it can happen that both group
// of honest nodes will "accept" two different packets at the end of the phase
// and will reject the other one at the beginning of the next phase (due to the
// DKG "rules"). The condition for that to happen is that the first group of honest
// nodes reboradcast very quickly p1 between themselves such that they accept p1
// but didn't receive yet enough "confirmation" from the other group about p2 to
// accept it before the end of the phase. It requires hard to measure
// assumptions about the network topology and latencies. As well, the risk is
// high: if both group accept both p1 and p2 the malicious actor is evicted from
// the DKG. This implementation randomizes the transmission of blocks each time
// to drastically reduces the risk of pulling out such attacks.
type broadcastChannel struct {
	c  *broadcastConfig
	l  log.Logger
	to []*key.Node
	// to keep track of deal messages
	// XXX: we could avoid having one store only however, that would necessitate
	// to allow for more transmission possible in one given round
	deals *store
	// to keep track of response messages
	resps *store
	// to keep track of justification messages
	justs *store
}

// Packet is an interface that represents the packets that gets broadcasted by
// this protocol. These packets must be authenticated so this interface allows
// to (1) get the information needed to verify the authenticity and to get the
// identifier of the creator of the packet - information that is required to
// implement the broadcast protocol.
type Packet = dkg.Packet

// PacketVerifier is the function that can verify the authenticity of a packet.
type PacketVerifier func(Packet) error

// PacketCallback is called when a packet is ready to be accepted by the
// application logic.
type PacketCallback func(Packet)

type broadcastConfig struct {
	l                 log.Logger
	client            net.ProtocolClient
	us                *key.Pair
	role              role
	dealers           []*key.Node
	sharers           []*key.Node
	dealThr           int
	shareThr          int
	maxPacketPerPhase int
	verifier          PacketVerifier
	accepter          PacketCallback
}

func newBroadcastChannel(c *broadcastConfig) *broadcastChannel {
	bc := &broadcastChannel{
		l:  c.l,
		c:  c,
		to: nodeUnion(c.dealers, c.sharers),
	}
	_, bc.role = bc.findNode(c.us)
	return bc
}

func (b *broadcastChannel) PushDeals(bundle *dkg.DealBundle) {
	b.deals.push(bundle.Issuer(), b.role, b.c.us.Public.Address(), deal)
	b.broadcast(bundle)
}

func (b *dkgBoard) PushResponses(bundle *dkg.ResponseBundle) {
	b.deals.push(bundle.Issuer(), b.role, b.c.us.Public.Address(), deal)
	b.broadcast(bundle)
}

func (b *dkgBoard) PushJustifications(bundle *dkg.JustificationBundle) {
	b.deals.push(bundle.Issuer(), b.role, b.c.us.Public.Address(), deal)
	b.broadcast(bundle)
}

func (b *broadcastChannel) Broadcast(c context.Context, p *proto.BroadcastPacket) (*proto.Empty, error) {
	var netAddr = net.RemoteAddress(c)
	dkgPacket, err := protoToDKGPacket(p.GetDKG())
	if err != nil {
		b.l.Debug("broadcast", "invalid_dkg_packet", "from", netAddr, "packet", fmt.Sprintf("%+v", p))
		return nil, errors.New("invalid packet")
	}
	// dispatch to the correct store
	var store *store
	switch dkgPacket.(type) {
	case *dkg.DealBundle:
		store = b.deals
	case *dkg.ResponseBundle:
		store = b.resps
	case *dkg.JustificationBundle:
		store = b.justs
	default:
		b.l.Debug("broadcast", "invalid_packet", "from", netAddr, "packet", fmt.Sprintf("%+v", dkgPacket))
		return nil, errors.New("invalid packet")
	}

	// first verify if the inner packet has a valid signature - this is needed
	// to make sure we only store authentic packets and dont filter out packets
	// claiming to be from honest nodes.
	if err := b.c.verifier(dkgPacket); err != nil {
		b.l.Debug("broadcast", "invalid_dkg_packet", "from", netAddr, "packet", fmt.Sprintf("%+v", p))
		return nil, errors.New("invalid dkg signature")
	}

	// then extract transmitter address and if verify signature is correct
	transmitter, role := b.findNode(p.GetTransmitter())
	if transmitter == nil {
		b.l.Debug("broadcast", "unknown_issuer", "from", netAddr)
		return nil, errors.New("unknown issuer")
	}

	hash := hashOfDKGPacket(dkgPacket)
	if err := DKGAuthScheme.Verify(transmitter.Key, hash, p.GetSignature()); err != nil {
		b.l.Debug("broadcast", "invalid_signature", "from", netAddr)
		return nil, errors.New("invalid signature")
	}
	shouldBroadcast, shouldAccept := store.push(issuer, transmitter, dealer, hash, spacket)
	if shouldBroadcast {
		// retransmit the packet as well
		return nil, b.broadcast(p)
	} else if shouldAccept {
		b.c.accepter(packet)
	}
	return new(drand.Empty), nil
}

// broadcast signs the packet and randomizes the order of the nodes to which it
// sends the packet to
func (b *broadcastChannel) broadcast(p dkg.Packet) {
	signature, err := DKGAuthScheme.Sign(b.us.Key, hashOfDKGPacket(p))
	if err != nil {
		b.l.Error("broadcast", "unable_to_sign", "err", err)
		return
	}
	b := &proto.BroadcastPacket{
		DKG:         dkgPacketToProto(p),
		Signature:   signature,
		Transmitter: b.us.Public.Address(),
	}

	var good int
	for _, i := range newRand().Perm(len(b.to)) {
		to := b.nodes[i]
		if to.Address() == b.us.Public.Address() {
			continue
		}
		if err := b.client.Broadcast(context.Background(), to, b); err != nil {
			b.l.Debug("broadcast", "unable_to_send", "err", err)
		} else {
			good++
		}
	}
	b.l.Debug("broadcast", "send_packet", "success_broadcast", fmt.Sprintf("%d/%d", good, len(b.to)))
}

func (b *broadcastChannel) accept(p dkg.Packet) {
	switch packet := p.(type) {
	case *dkg.DealBundle:
		b.dealCh <- packet
	case *dkg.ResponseBundle:
		b.respCh <- packet
	case *dkg.JustificationBundle:
		b.justCh <- packet
	}
}

// findNode returns the node associated with that address and the role
// assicoated: dealer, holder or both
func (b *broadcastChannel) findNode(addr string) (node *key.Node, r role) {
	if n = findNodeIn(b.c.dealers, addr); n != nil {
		r = dealerRole
	}
	if n2 := findNodeIn(b.c.holders, addr); n2 != nil {
		if r == dealerRole {
			r = dealerHolder
		} else {
			r = holderRole
		}
	}
	return n, r
}

func findNodeIn(list []*key.Node, addr string) *key.Node {
	for _, n := range list {
		if n.Address() == addr {
			return n
		}
	}
	return nil
}

// store responsability is to keep all the different counters of the different
// packets that are being broadcasted by the protocol
type store struct {
	packets  map[uint32][]*packetCounter
	max      int
	l        log.Logger
	dealThr  int
	shareThr int
}

func newStore(l log.Logger, maxIssuer, dealThr, shareThr int) *store {
	return &store{
		l:        l,
		packets:  make(map[uint32][]*packetCounter),
		max:      maxIssuer,
		dealThr:  dealThr,
		shareThr: shareThr,
	}
}

func (s *store) push(issuer uint32, role role, transmitter string, h []byte, packet Packet) (bool, bool) {
	// find the right counter for this issuer
	counters := s.packets[issuer]
	var counter *counter
	for _, c := range counters {
		if bytes.Equal(h, c.hash) {
			counter = c
		}
	}

	if counter == nil {
		if len(counters) < max {
			counter = newCounter(s.dealThr, s.shareThr, h, packet)
			s.packets[from] = append(s.packets[from], counter)
		} else {
			s.l.Info("broadcast", "FULL_CACHE (DOS)", "index", issuer, "dealer?", dealer, "packet", fmt.Sprintf("%+v", packet))
			return false, false
		}
	}
	return counter.push(transmitter, role)
}

// packetCounter  role is to maintain a record of which node broadcasted this
// node and to tell the upper logic when this packet is ready to be accepted by
// the application (dkg library in this case).
type packetCounter struct {
	l         log.Logger
	hash      []byte
	packet    interface{}
	dealers   []string
	dealThr   int
	holders   []string
	holderThr int
}

func newCounter(dealThr, holderThr int, h []byte, packet Packet) *packetCounter {
	return &packetCounter{
		hash:      h,
		packet:    packet,
		dealThr:   dealThr,
		holderThr: holderThr,
	}
}

// returns
// shouldBroadcast: whether we must broadcast the packet if it's the first time
// we've seen it
// shouldAccept: whether we must pass the packet to the application logic
func (p *packetCounter) push(transmitter string, r role) (bool, bool) {
	list := p.dealers
	if !dealers {
		list = p.holders
	}
	for _, r := range list {
		if r == transmitter {
			return false, false
		}
	}
	list = append(list, issuer)
	if dealers {
		p.dealers = list
	} else {
		p.holders = list
	}

	var shouldBroadcast bool
	if len(p.dealers)+len(p.holders) == 1 {
		// first time we receive this message
		shouldBroadcast = true
	}

	var shouldAccept bool
	if len(p.dealers) >= dealThr && len(p.holders) >= holderThr {
		// seen enough retransmission from dealers and holders to pass the
		// packet to the dkg library
		shouldAccept = true
	}
	return shouldBroadcast, shouldAccept
}

func hashOfDKGPacket(p dkg.Packet) []byte {
	if p == nil {
		return nil
	}
	var h = sha256.New()
	h.Write(p.Hash())
	h.Write(p.Signature())
	return h.Sum(nil)
}

func newRand() *rand.Rand {
	var isource int64
	if err := binary.Read(crand.Reader, binary.BigEndian, &isource); err != nil {
		panic(err)
	}
	rand.New(rand.NewSource(isource))
}

type role int

const (
	dealerRole = iota + 1
	holderRole
	// happens when a node is within both group
	dealerHolder
)
