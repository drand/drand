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
	pdkg "github.com/drand/drand/protobuf/crypto/dkg"
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
// origin for the same phases, up to a threshold number of times.
// The reason why the implementation doesn't restrict to
// one node is that attacker can abuse of this to split the nodes into two: once
// a group of honest nodes has accepted a message (i.e. it received the same
// message from a threshold of nodes), it will not participate to
// rebroadcast another one and therefore the group will not be able to tell the
// DKG library that a given node is misbehaving.
//
// NOTE: this implementation while in practice is largely sufficient, in theory
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
	l       log.Logger
	us      *key.Pair
	nodes   []*key.Node
	dealers []*key.Node
	holders []*key.Node
	client  net.ProtocolClient
	// deals must be rebroadcasted by at least a threshold of share holders
	deals *store
	// responses must be rebroadcasted by at least a threshold of dealers
	// as well as a threshold of share holders (can be different)
	resps *store
	// justifications need to be rebroadcasted by at least a threshold of share
	// holders
	justs *store
}

func newBroadcastChannel(l log.Logger, client net.ProtocolClient, us *key.Pair, to []*key.Node) *broadcastChannel {
	return &broadcastChannel{
		us:     us,
		nodes:  to,
		client: client,
		l:      l,
	}
}

func (b *broadcastChannel) PushDeals(bundle *dkg.DealBundle) {
	pdeal := dealToProto(bundle)
	b.l.Info("push", "deal", "index", bundle.DealerIndex, "hash", fmt.Sprintf("%x", bundle.Hash()))
	b.dealCh <- *bundle
	go b.broadcastPacket(pdeal, "deal")
}

func (b *dkgBoard) PushResponses(bundle *dkg.ResponseBundle) {
	presp := respToProto(bundle)
	b.respCh <- *bundle
	go b.broadcastPacket(presp, "response")
}

func (b *dkgBoard) PushJustifications(bundle *dkg.JustificationBundle) {
	pjust := justifToProto(bundle)
	b.justCh <- *bundle
	go b.broadcastPacket(pjust, "justification")
}

func (b *broadcastChannel) Broadcast(c context.Context, p *proto.BroadcastPacket) (*proto.Empty, error) {
	var netAddr = net.RemoteAddress(c)
	// extract issuer and verify signature is correct
	node, dealer := b.findNode(p.GetTransmitter())
	if node == nil {
		b.l.Debug("broadcast", "unknown_issuer", "from", netAddr)
		return errors.New("unknown issuer")
	}
	msg := hashofDKGPacket(p.GetDKG())
	if err := BroadcastAuthScheme.Verify(node.Key, msg, p.GetSignature()); err != nil {
		b.l.Debug("broadcast", "invalid_signature", "from", netAddr)
		return errors.New("invalid signature")
	}

	// dispatch to the correct store
	var store *store
	var spacket interface{}
	switch packet := p.GetBundle().(type) {
	case *pdkg.Packet_Deal:
		spacket = packet.GetDeal()
		store = b.deals
	case *pdkg.Packet_Response:
		spacket = packet.Response
		store = b.resps
	case *pdkg.Packet_Justification:
		spacket = packet.Justification
		store = b.justs
	default:
		b.l.Debug("broadcast", "invalid_packet", "from", addr, "packet", fmt.Sprintf("%+v", p))
		return errors.New("invalid packet")
	}

	shouldBroadcast, shouldAccept := store.push(node, spacket)
	if shouldBroadcast {
		// retransmit the packet as well
		return b.broadcast(p)
	} else if shouldAccept {
		b.accept(packet)
	}
	return nil
}

// broadcast signs the packet and randomizes the order of the nodes to which it
// sends the packet to
func (b *broadcastChannel) broadcast(p *pdkg.Packet) {
	signature, err := BroadcastAuthScheme.Sign(b.us.Key, hashOfDKGPacket(p))
	if err != nil {
		b.l.Error("broadcast", "unable_to_sign", "err", err)
		return
	}
	b := &proto.BroadcastPacket{
		DKG:         p,
		Signature:   signature,
		Transmitter: b.us.Public.Address(),
		// TODO Add dealer?
	}
	for _, i := range newRand().Perm(len(b.to)) {
		to := b.nodes[i]
		if to.Address() == b.us.Public.Address() {
			continue
		}
		b.client.Broadcast(context.Background(), to, b)
	}
}

func (b *broadcastChannel) accept(p interface{}) {
	switch packet := p.(type) {
	case *dkg.DealBundle:
		b.dealCh <- packet
	case *dkg.ResponseBundle:
		b.respCh <- packet
	case *dkg.JustificationBundle:
		b.justCh <- packet
	}
}

// findNode returns the node associated with that address and true if it is a
// dealer or false otherwise (share holder).
func (b *broadcastChannel) findNode(addr string) (*key.Node, bool) {
	if n := findNodeIn(b.dealers, addr); n != nil {
		return n, true
	}
	return findNodeIn(b.holders, addr)
}

func findNodeIn(list []*key.Node, addr string) *key.Node {
	for _, n := range list {
		if n.Address() == addr {
			return n
		}
	}
	return nil
}

type store struct {
	packets map[string][]*packetCounter
	thr1    int
	thr2    int
}

func (s *store) push(from *key.Node, h []byte, packet interface{}) bool {
	counters, ok := s.packets[from]
	var counter *counter
	for _, c := range counters {
		if bytes.Equal(h, c.hash) {
			counter = c
		}
	}
	if counter == nil {
		if len(counters) < s.thr1+s.thr2 {
			counter = newCounter()
			s.packets[from] = append(s.packets[from], counter)
		}
	}

	return true
}

type packetCounter struct {
	issuer uint32
	packet interface{}
	rcvd   []uint32
}

func (p *packetCounter) process(issuer uint32, p interface{}) {
	for _, r := range p.rcvd {
		if r == issuer {
			return
		}
	}
	p.rcvd = append(p.rcvd, issuer)
}

func hashOfDKGPacket(p *proto.BroadcastPacket) []byte {
	if p == nil {
		return nil
	}
	var h = sha256.New()
	switch packet := p.GetBundle().(type) {
	case *pdkg.Packet_Deal:
		h.Write(packet.Deal.Hash())
		h.Write(packet.Deal.GetSignature())
	case *pdkg.Packet_Response:
		h.Write(packet.Response.Hash())
		h.Write(packet.Response.GetSignature())
	case *pdkg.Packet_Justification:
		h.Write(packet.Justification.Hash())
		h.Write(packet.Justification.GetSignature())
	}
	return h.Sum(nil)
}

func newRand() *rand.Rand {
	var isource int64
	if err := binary.Read(crand.Reader, binary.BigEndian, &isource); err != nil {
		panic(err)
	}
	rand.New(rand.NewSource(isource))
}
