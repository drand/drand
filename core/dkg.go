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
	clock "github.com/jonboulle/clockwork"
	"google.golang.org/grpc/peer"
)

type dkgHandler struct {
	l      log.Logger
	board  *dkgBoard
	phaser *dkg.Phaser
	conf   *dkg.Config
	clock  clock.Clock
}

func newDkgHandler(l log.Logger, client net.ProtocolClient, clock clock.Clock, conf *dkg.Config) *dkgHandler {
	board := newBoard(l, client)

	phaser := dkg.NewTimePhaserFunc()
}

// dkgBoard is a struct that implements a dkg.Board: it is the interface between
// the network and the crypto library whose main taks is to convert dkg packets
// from/to protobuf structures and send/receive packets from network.
type dkgBoard struct {
	l      log.Logger
	dealCh chan *dkg.AuthDealBundle
	respCh chan *dkg.AuthResponseBundle
	justCh chan *dkg.AuthJustifBundle
	client net.ProtocolClient
}

func newBoard(l log.Logger, client net.ProtocolClient) *dkgBoard {
	return &dkgBoard{
		l:      l,
		dealCh: make(chan *dkg.AuthDealBundle, 10),
		respCh: make(chan *dkg.AuthResponseBundle, 10),
		justCh: make(chan *dkg.AuthJustifBundle, 10),
		client: client,
		nodes:  nodes,
	}
}

func newReshareBoard(l log.Logger, client net.ProtocolClient, nodes []*key.Identity) *dkgBoard {
	board := newBoard(l, client, nodes)
	board.isReshare = true
	return board
}

func (b *dkgBoard) FreshDKG(c context.Context, p *proto.DKGPacket) (*proto.Empty, error) {
	return new(proto.Empty), b.dispatch(c, p.Dkg)
}

func (b *dkgBoard) ReshareDKG(c context.Context, p *proto.ResharePacket) (*proto.Empty, error) {
	b.dispatch(c, p.Dkg)
}

func (b *dkgBoard) PushDeals(bundle AuthDealBundle) {
	pdeal := dealToProto(&bundle)
	go b.broadcastPacket(pdeal)
}

func (b *dkgBoard) PushResponses(bundle AuthResponseBundle) {
	presp := respToProto(&bundle)
	go b.broadcastPacket(presp)
}

func (b *dkgBoard) PushJustifications(bundle AuthJustifBundle) {
	pjust := justToProto(&bundle)
	go b.broadcastPacket(pjust)
}

func (b *dkgBoard) dispatch(c context.Context, p *pdkg.Packet) {
	var addr = "unknown"
	peer, ok := peer.FromContext(c)
	if ok {
		addr = peer.Addr.String()
	}
	var err error
	switch bundle := p.GetBundle().(type) {
	case *pdkg.Packet_Deal:
		err = b.dispatchDeal(addr, bundle, p.GetSignature())
	case *pdkg.Packet_Response:
		err = b.dispatchResponse(addr, bundle, p.GetSignature())
	case *pdkg.Packet_Justification:
		err = b.dispatchJustification(addr, bundle, p.GetSignature())
	case nil:
		fallthrough
	default:
		b.l.Debug("board", "invalid_packet", "from", addr)
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
	authBundle := &dkg.AuthDealBundle{
		Bundle:    bundle,
		Signature: sig,
	}
	b.l.Debug("board", "received_deal", "from", p, "dealer_index", bundle.DealerIndex)
	b.dealCh <- authBundle
}

func (b *dkgBoard) dispatchResponse(p string, b *pdkg.ResponseBundle, sig []byte) {
	authBundle := &dkg.AuthResponseBundle{
		Bundle:    protoToResp(b),
		Signature: sig,
	}
	b.l.Debug("board", "received_responses", "from", p, "share_index", bundle.ShareIndex)
	b.respCh <- authBundle
}

func (b *dkgBoard) dispatchJustification(p string, b *pdkg.JustifBundle, sig []byte) error {
	bundle, err := protoToJustif(b)
	if err != nil {
		b.l.Debug("board", "invalid_justif", "from", p, "err", err)
		return fmt.Errorf("invalid justif: %s", err)
	}
	authBundle := &dkg.AuthJustifBundle{
		Bundle:    bundle,
		Signature: sig,
	}
	b.l.Debug("board", "received_justifications", "from", p, "dealer_index", bundle.DealerIndex)
	b.justCh <- authBundle
}

func (b *dkgBoard) broadcastPacket(packet *pdkg.Packet) {
	if b.isReshare {
		packet := proto.ResharePacket{
			Dkg: packet,
		}
		for _node := range b.nodes {
			err := b.client.ReshareDKG(context.Backgroun(), id, packet)
			if err != nil {
				b.l.Debug("board_reshare", "broadcast_packet", "to", node.Address(), "err", err)
				continue
			}
			b.l.Debug("board_reshare", "broadcast_packet", "to", node.Address(), "success")
		}
	} else {
		packet := proto.DKGPacket{
			Dkg: packet,
		}
		for _, node := range b.nodes {
			err := b.client.FreshDKG(context.Background(), id, packet)
			if err != nil {
				b.l.Debug("board", "broadcast_packet", "to", node.Address(), "err", err)
				continue
			}
			b.l.Debug("board", "broadcast_packet", "to", node.Address(), "success")
		}
	}
}
