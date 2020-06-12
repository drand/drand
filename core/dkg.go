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
	dealCh    chan dkg.DealBundle
	respCh    chan dkg.ResponseBundle
	justCh    chan dkg.JustificationBundle
	client    net.ProtocolClient
	nodes     []*key.Node
	isReshare bool
	public    *key.Identity
}

var _ dkg.Board = (*dkgBoard)(nil)

// newBoard is to be used when starting a new DKG protocol from scratch
func newBoard(l log.Logger, client net.ProtocolClient, public *key.Identity, group *key.Group) *dkgBoard {
	return initBoard(l, client, public, group.Nodes)
}

func initBoard(l log.Logger, client net.ProtocolClient, public *key.Identity, nodes []*key.Node) *dkgBoard {
	return &dkgBoard{
		l:      l,
		dealCh: make(chan dkg.DealBundle, len(nodes)),
		respCh: make(chan dkg.ResponseBundle, len(nodes)),
		justCh: make(chan dkg.JustificationBundle, len(nodes)),
		client: client,
		nodes:  nodes,
		public: public,
	}
}

// newReshareBoard is to be used when running a resharing protocol
func newReshareBoard(l log.Logger, client net.ProtocolClient, public *key.Identity, oldGroup, newGroup *key.Group) *dkgBoard {
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

	board := initBoard(l, client, public, nodes)
	board.isReshare = true
	return board
}

func (b *dkgBoard) FreshDKG(c context.Context, p *proto.DKGPacket) (*proto.Empty, error) {
	return new(proto.Empty), b.dispatch(c, p.Dkg)
}

func (b *dkgBoard) ReshareDKG(c context.Context, p *proto.ResharePacket) (*proto.Empty, error) {
	return new(proto.Empty), b.dispatch(c, p.Dkg)
}

func (b *dkgBoard) PushDeals(bundle *dkg.DealBundle) {
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

func (b *dkgBoard) dispatch(c context.Context, p *pdkg.Packet) error {
	var addr = "unknown"
	peer, ok := peer.FromContext(c)
	if ok {
		addr = peer.Addr.String()
	}
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

func (b *dkgBoard) dispatchDeal(p string, d *pdkg.DealBundle) error {
	bundle, err := protoToDeal(d)
	if err != nil {
		b.l.Debug("board", "invalid_deal", "from", p, "err", err)
		return fmt.Errorf("invalid deal: %s", err)
	}
	b.l.Debug("board", "received_deal", "from", p, "dealer_index", bundle.DealerIndex)
	b.dealCh <- *bundle
	return nil
}

func (b *dkgBoard) dispatchResponse(p string, r *pdkg.ResponseBundle) {
	bundle := protoToResp(r)
	b.l.Debug("board", "received_responses", "from", p, "share_index", bundle.ShareIndex)
	b.respCh <- *bundle
}

func (b *dkgBoard) dispatchJustification(p string, j *pdkg.JustificationBundle) error {
	bundle, err := protoToJustif(j)
	if err != nil {
		b.l.Debug("board", "invalid_justif", "from", p, "err", err)
		return fmt.Errorf("invalid justif: %s", err)
	}
	b.l.Debug("board", "received_justifications", "from", p, "dealer_index", bundle.DealerIndex)
	b.justCh <- *bundle
	return nil
}

func (b *dkgBoard) IncomingDeal() <-chan dkg.DealBundle {
	return b.dealCh
}

func (b *dkgBoard) IncomingResponse() <-chan dkg.ResponseBundle {
	return b.respCh
}

func (b *dkgBoard) IncomingJustification() <-chan dkg.JustificationBundle {
	return b.justCh
}

// broadcastPacket broads the given packet to ALL nodes in the list of ids he's
// has.
func (b *dkgBoard) broadcastPacket(packet *pdkg.Packet, t string) {
	if b.isReshare {
		rpacket := &proto.ResharePacket{
			Dkg: packet,
		}
		for _, node := range b.nodes {
			if node.Address() == b.public.Address() {
				continue
			}
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
			if node.Address() == b.public.Address() {
				continue
			}
			_, err := b.client.FreshDKG(context.Background(), node, rpacket)
			if err != nil {
				b.l.Debug("board", "broadcast_packet", "to", node.Address(), "err", err)
				continue
			}
			b.l.Debug("board", "broadcast_packet", "to", node.Address(), "type", t)
		}
	}
}
