package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	pdkg "github.com/drand/drand/protobuf/crypto/dkg"
	proto "github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/share/dkg"
	"google.golang.org/grpc/peer"
)

// dkgInfo is a simpler wrapper that keeps the relevant config and logic
// necessary during the DKG protocol.
type dkgInfo struct {
	target  *key.Group
	board   *dkgBoard
	phaser  *dkg.TimePhaser
	conf    *dkg.Config
	proto   *dkg.Protocol
	started bool
}

// dkgBoard is a struct that implements a dkg.Board: it is the interface between
// the network and the crypto library whose main taks is to convert dkg packets
// from/to protobuf structures and send/receive packets from network.
type dkgBoard struct {
	l         log.Logger
	dealCh    chan dkg.AuthDealBundle
	respCh    chan dkg.AuthResponseBundle
	justCh    chan dkg.AuthJustifBundle
	client    net.ProtocolClient
	nodes     []*key.Node
	isReshare bool
	// TODO XXX simply for debugging
	pub *key.Identity
}

// newBoard is to be used when starting a new DKG protocol from scratch
func newBoard(l log.Logger, client net.ProtocolClient, group *key.Group) *dkgBoard {
	return initBoard(l, client, group.Nodes)
}

func initBoard(l log.Logger, client net.ProtocolClient, nodes []*key.Node) *dkgBoard {
	return &dkgBoard{
		l:      l,
		dealCh: make(chan dkg.AuthDealBundle, len(nodes)),
		respCh: make(chan dkg.AuthResponseBundle, len(nodes)),
		justCh: make(chan dkg.AuthJustifBundle, len(nodes)),
		client: client,
		nodes:  nodes,
	}
}

// newReshareBoard is to be used when running a resharing protocol
func newReshareBoard(l log.Logger, client net.ProtocolClient, oldGroup, newGroup *key.Group, pub *key.Identity) *dkgBoard {
	// takes all nodes and new nodes, without duplicates
	var nodes []*key.Node
	tryAppend := func(n *key.Node) {
		var found bool
		for _, seen := range nodes {
			if seen.Identity.Equal(n.Identity) {
				found = true
				break
			}
		}
		if !found {
			nodes = append(nodes, n)
		}
	}
	for _, n := range oldGroup.Nodes {
		tryAppend(n)
	}
	for _, n := range newGroup.Nodes {
		tryAppend(n)
	}

	board := initBoard(l, client, nodes)
	board.isReshare = true
	board.pub = pub
	return board
}

func (b *dkgBoard) FreshDKG(c context.Context, p *proto.DKGPacket) (*proto.Empty, error) {
	return new(proto.Empty), b.dispatch(c, p.Dkg)
}

func (b *dkgBoard) ReshareDKG(c context.Context, p *proto.ResharePacket) (*proto.Empty, error) {
	return new(proto.Empty), b.dispatch(c, p.Dkg)
}

func (b *dkgBoard) PushDeals(bundle dkg.AuthDealBundle) {
	pdeal := dealToProto(&bundle)
	fmt.Printf("-- PUSHING Deal: index %d - pub %s - hash %x - sig: %x\n", bundle.Bundle.DealerIndex, b.pub, bundle.Bundle.Hash(), bundle.Signature)
	go b.broadcastPacket(pdeal, "deal")
}

func (b *dkgBoard) PushResponses(bundle dkg.AuthResponseBundle) {
	presp := respToProto(&bundle)
	go b.broadcastPacket(presp, "response")
}

func (b *dkgBoard) PushJustifications(bundle dkg.AuthJustifBundle) {
	pjust := justifToProto(&bundle)
	go b.broadcastPacket(pjust, "justification")
}

func (b *dkgBoard) dispatch(c context.Context, p *pdkg.Packet) error {
	var addr = "unknown"
	peer, ok := peer.FromContext(c)
	if ok {
		addr = peer.Addr.String()
	}
	var err error
	switch packet := p.GetBundle().(type) {
	case *pdkg.Packet_Deal:
		err = b.dispatchDeal(addr, packet.Deal, p.GetSignature())
	case *pdkg.Packet_Response:
		b.dispatchResponse(addr, packet.Response, p.GetSignature())
	case *pdkg.Packet_Justification:
		err = b.dispatchJustification(addr, packet.Justification, p.GetSignature())
	default:
		b.l.Debug("board", "invalid_packet", "from", addr, "packet", fmt.Sprintf("%+v", p))
		err = errors.New("invalid_packet")
	}
	return err
}

func (b *dkgBoard) dispatchDeal(p string, d *pdkg.DealBundle, sig []byte) error {
	bundle, err := protoToDeal(d)
	if err != nil {
		b.l.Debug("board", "invalid_deal", "from", p, "err", err)
		return fmt.Errorf("invalid deal: %s", err)
	}
	authBundle := dkg.AuthDealBundle{
		Bundle:    bundle,
		Signature: sig,
	}
	b.l.Debug("board", "received_deal", "from", p, "dealer_index", bundle.DealerIndex)
	b.dealCh <- authBundle
	return nil
}

func (b *dkgBoard) dispatchResponse(p string, r *pdkg.ResponseBundle, sig []byte) {
	authBundle := dkg.AuthResponseBundle{
		Bundle:    protoToResp(r),
		Signature: sig,
	}
	b.l.Debug("board", "received_responses", "from", p, "share_index", authBundle.Bundle.ShareIndex)
	b.respCh <- authBundle
}

func (b *dkgBoard) dispatchJustification(p string, j *pdkg.JustifBundle, sig []byte) error {
	bundle, err := protoToJustif(j)
	if err != nil {
		b.l.Debug("board", "invalid_justif", "from", p, "err", err)
		return fmt.Errorf("invalid justif: %s", err)
	}
	authBundle := dkg.AuthJustifBundle{
		Bundle:    bundle,
		Signature: sig,
	}
	b.l.Debug("board", "received_justifications", "from", p, "dealer_index", authBundle.Bundle.DealerIndex)
	b.justCh <- authBundle
	return nil
}

func (b *dkgBoard) IncomingDeal() <-chan dkg.AuthDealBundle {
	return b.dealCh
}

func (b *dkgBoard) IncomingResponse() <-chan dkg.AuthResponseBundle {
	return b.respCh
}

func (b *dkgBoard) IncomingJustification() <-chan dkg.AuthJustifBundle {
	return b.justCh
}

// broadcastPacket broads the given packet to ALL nodes in the list of ids he's
// has.
// NOTE: For simplicity, there is a minor cost here that it sends our own
// packet via a connection instead of using channel. Could be changed later on
// if required.
func (b *dkgBoard) broadcastPacket(packet *pdkg.Packet, t string) {
	if b.isReshare {
		rpacket := &proto.ResharePacket{
			Dkg: packet,
		}
		for _, node := range b.nodes {
			_, err := b.client.ReshareDKG(context.Background(), node, rpacket)
			if err != nil {
				b.l.Debug("board_reshare", "broadcast_packet", "to", node.Address(), "err", err)
				continue
			}
			b.l.Debug("board_reshare", "broadcast_packet", "to", node.Address(), "type", t)
		}
	} else {
		rpacket := &proto.DKGPacket{
			Dkg: packet,
		}
		for _, node := range b.nodes {
			_, err := b.client.FreshDKG(context.Background(), node, rpacket)
			if err != nil {
				b.l.Debug("board", "broadcast_packet", "to", node.Address(), "err", err)
				continue
			}
			b.l.Debug("board", "broadcast_packet", "to", node.Address(), "type", t)
		}
	}
}
