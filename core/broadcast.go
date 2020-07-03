package core

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

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
//
// Note however, the protocol when naively implemented is susceptible to to
// amplification types of vulnerability where one node simply sends many packets
// that each produces O(n^2) traffic, leading to a potential DOS of the network.
// To counter that, this implementation uses the fact that a participant is only
// supposed to send _one_ message in each step. If a participant receives a
// different message from the same node, then it is ignored and will not be
// re-broadcasted. Messages are verified (signature) and compared via their hashes.
type broadcastChannel struct {
	l       log.Logger
	us      *key.Pair
	nodes   []*key.Node
	dealers []*key.Node
	holders []*key.Node
	client  net.ProtocolClient
	// deals must be rebroadcasted by at least a threshold of share holders
	deals *counter
	// responses must be rebroadcasted by at least a threshold of dealers
	// as well as a threshold of share holders (can be different)
	resps *counter
	// justifications need to be rebroadcasted by at least a threshold of share
	// holders
	justs *counter
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
	node, dealer := b.findNode(p.GetIssuer())
	if node == nil {
		b.l.Debug("broadcast_packet", "unknown_issuer", "from", netAddr)
		return errors.New("unknown issuer")
	}
	msg := hashOfBroadcastPacket(p)
	if err := BroadcastAuthScheme.Verify(node.Key, msg, p.GetSignature()); err != nil {
		b.l.Debug("broadcast_packet", "invalid_signature", "from", netAddr)
		return errors.New("invalid signature")
	}
	//
	var err error
	switch packet := p.GetBundle().(type) {
	case *pdkg.Packet_Deal:
		err = b.dispatchDeal(addr, packet.Deal)
	case *pdkg.Packet_Response:
		b.dispatchResponse(addr, packet.Response)
	case *pdkg.Packet_Justification:
		err = b.dispatchJustification(addr, packet.Justification)
	default:
		b.l.Debug("board", "invalid_packet", "from", addr, "packet", fmt.Sprintf("%+v", p))
		err = errors.New("invalid_packet")
	}
	return err
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
	packets map[uint32]*packetCounter
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

func hashOfBroadcastPacket(p *proto.BroadcastPacket) []byte {
	var h = sha256.New()
	switch packet := p.GetDKG().GetBundle().(type) {
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
