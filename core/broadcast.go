package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/share/dkg"
)

// broadcast implements a very simple broadcasting mechanism: for each new packet
// seen, rebroadcast it once. While this protocol is simple to implement, it
// does not guarantees anything about the timing of which nodes is going to
// accept packets, with Byzantine adversaries. However, an attacker that wants
// to split the nodes into two groups such that they accept different deals need
// to be able to reliably know the network topology and be able to send the
// deals close enough to the next phase to each node such that they won't be
// able to send it to their other nodes in time.
//
// There are other broadcast protocols that are resilient against Byzantine
// behaviors but these require a higher threshold and they still do not protect
// against these kinds of "epoch boundary" attack. For example
// https://eprint.iacr.org/2011/535.pdf suggests a protocol where each
// rebroadcast until a certain threshold happens. That protocol is secure
// against byzantine behavior if number of malicious actors is less than 1/3 of
// the total number of participants. As well, and the most problematic point
// here, it does not protect against epoch boundary attacks since a group of
// nodes can "accept" a packet right before the next phase starts and the rest
// of the node don't accept it because it's too late. Note that even though the
// DKG library allows to use fast sync the fast sync mode.
type broadcast struct {
	sync.Mutex
	own    string
	to     []*key.Node
	client net.ProtocolClient
	l      log.Logger
	// list of messages already retransmitted comparison by hash
	hashes set
	dealCh chan dkg.DealBundle
	respCh chan dkg.ResponseBundle
	justCh chan dkg.JustificationBundle
	verif  verifier
}

type packet = dkg.Packet

var _ dkg.Board = (*broadcast)(nil)

// verifier is a type for  a function that can verify the validity of a dkg
// Packet, namely that the signature is correct.
type verifier func(packet) error

func newBroadcast(l log.Logger, c net.ProtocolClient, own string, to []*key.Node, v verifier) *broadcast {
	return &broadcast{
		l:      l,
		own:    own,
		client: c,
		to:     to,
		dealCh: make(chan dkg.DealBundle, len(to)),
		respCh: make(chan dkg.ResponseBundle, len(to)),
		justCh: make(chan dkg.JustificationBundle, len(to)),
		hashes: new(arraySet),
		verif:  v,
	}
}

func (b *broadcast) PushDeals(bundle *dkg.DealBundle) {
	b.dealCh <- *bundle
	go b.sendout(bundle)
}

func (b *broadcast) PushResponses(bundle *dkg.ResponseBundle) {
	b.respCh <- *bundle
	go b.sendout(bundle)
}

func (b *broadcast) PushJustifications(bundle *dkg.JustificationBundle) {
	b.justCh <- *bundle
	go b.sendout(bundle)
}

func (b *broadcast) BroadcastDKG(c context.Context, p *drand.DKGPacket) (*drand.Empty, error) {
	b.Lock()
	defer b.Unlock()
	addr := net.RemoteAddress(c)
	dkgPacket, err := protoToDKGPacket(p.GetDkg())
	if err != nil {
		b.l.Debug("broadcast", "received invalid packet", "from", addr, "err", err)
		return nil, errors.New("invalid packet")
	}

	hash := hash(dkgPacket.Hash())
	if b.hashes.exists(hash) {
		// if we already seen this one, no need to verify even because that
		// means we already broadcasted it
		return new(drand.Empty), nil
	}
	if err := b.verif(dkgPacket); err != nil {
		b.l.Debug("broadcast", "received invalid signature", "from", addr)
		return nil, errors.New("invalid packet")
	}

	b.l.Debug("broadcast", "received new packet", "from", addr, "type", fmt.Sprintf("%T", dkgPacket))
	// we register we saw that packet and we broadcast it
	b.hashes.put(hash)
	go b.sendout(dkgPacket)
	b.dispatch(dkgPacket)
	return new(drand.Empty), nil
}

func (b *broadcast) dispatch(p packet) {
	switch pp := p.(type) {
	case *dkg.DealBundle:
		b.dealCh <- *pp
	case *dkg.ResponseBundle:
		b.respCh <- *pp
	case *dkg.JustificationBundle:
		b.justCh <- *pp
	}
}

func (b *broadcast) sendout(p packet) {
	dkgproto, err := dkgPacketToProto(p)
	if err != nil {
		b.l.Error("broadcast", "can't send packet", "err", err)
		return
	}
	proto := &drand.DKGPacket{
		Dkg: dkgproto,
	}
	var good int
	for _, n := range b.to {
		if n.Address() == b.own {
			continue
		}

		err := b.client.BroadcastDKG(context.Background(), n, proto)
		if err != nil {
			b.l.Debug("broadcast", "sending out", "error to", n.Address(), "err:", err)
		} else {
			good++
		}
	}
	b.l.Debug("broadcast", "sending out", "success", fmt.Sprintf("%d/%d", good, len(b.to)-1))
}

func (b *broadcast) IncomingDeal() <-chan dkg.DealBundle {
	return b.dealCh
}

func (b *broadcast) IncomingResponse() <-chan dkg.ResponseBundle {
	return b.respCh
}

func (b *broadcast) IncomingJustification() <-chan dkg.JustificationBundle {
	return b.justCh
}

type hash []byte

// set is a simple interface to keep tracks of all the packet hashes that we
// have rebroadcast already
// TODO: check if having a map makes more sense.
type set interface {
	put(hash)
	exists(hash) bool
}

type arraySet struct {
	hashes [][]byte
}

func (a *arraySet) put(hash hash) {
	for _, h := range a.hashes {
		if bytes.Equal(h, hash) {
			return
		}
	}
	a.hashes = append(a.hashes, hash)
}

func (a *arraySet) exists(hash hash) bool {
	for _, h := range a.hashes {
		if bytes.Equal(h, hash) {
			return true
		}
	}
	return false
}
