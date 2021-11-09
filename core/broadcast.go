package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"

	commonutils "github.com/drand/drand/common"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/share/dkg"
)

// Broadcast is an interface that represents the minimum functionality required
// by drand to both (1) be the interface between drand and the dkg logic and (2)
// implement the broadcasting mechanisn.
type Broadcast interface {
	dkg.Board
	BroadcastDKG(c context.Context, p *drand.DKGPacket) (*drand.Empty, error)
	Stop()
}

// echoBroadcast implements a very simple broadcasting mechanism: for each new
// packet seen, rebroadcast it once. While this protocol is simple to implement,
// it does not guarantees anything about the timing of which nodes is going to
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
type echoBroadcast struct {
	sync.Mutex
	l        log.Logger
	version  commonutils.Version
	beaconID string
	// responsible for sending out the messages
	dispatcher *dispatcher
	// list of messages already retransmitted comparison by hash
	hashes set
	dealCh chan dkg.DealBundle
	respCh chan dkg.ResponseBundle
	justCh chan dkg.JustificationBundle
	verif  verifier
}

type packet = dkg.Packet

var _ Broadcast = (*echoBroadcast)(nil)

// verifier is a type for  a function that can verify the validity of a dkg
// Packet, namely that the signature is correct.
type verifier func(packet) error

func newEchoBroadcast(l log.Logger, version commonutils.Version, beaconID string,
	c net.ProtocolClient, own string, to []*key.Node, v verifier) *echoBroadcast {
	return &echoBroadcast{
		l:          l,
		version:    version,
		beaconID:   beaconID,
		dispatcher: newDispatcher(l, c, to, own),
		dealCh:     make(chan dkg.DealBundle, len(to)),
		respCh:     make(chan dkg.ResponseBundle, len(to)),
		justCh:     make(chan dkg.JustificationBundle, len(to)),
		hashes:     new(arraySet),
		verif:      v,
	}
}

func (b *echoBroadcast) PushDeals(bundle *dkg.DealBundle) {
	b.dealCh <- *bundle
	b.Lock()
	defer b.Unlock()
	h := hash(bundle.Hash())
	b.l.Debugw("", "beacon_id", b.beaconID, "echoBroadcast", "push", "deal")
	b.sendout(h, bundle, true)
}

func (b *echoBroadcast) PushResponses(bundle *dkg.ResponseBundle) {
	b.respCh <- *bundle
	b.Lock()
	defer b.Unlock()
	h := hash(bundle.Hash())
	b.l.Debugw("", "beacon_id", b.beaconID, "echoBroadcast", "push", "response", bundle.String())
	b.sendout(h, bundle, true)
}

func (b *echoBroadcast) PushJustifications(bundle *dkg.JustificationBundle) {
	b.justCh <- *bundle
	b.Lock()
	defer b.Unlock()
	h := hash(bundle.Hash())
	b.l.Debugw("", "beacon_id", b.beaconID, "echoBroadcast", "push", "justification")
	b.sendout(h, bundle, true)
}

func (b *echoBroadcast) BroadcastDKG(c context.Context, p *drand.DKGPacket) (*drand.Empty, error) {
	b.Lock()
	defer b.Unlock()

	addr := net.RemoteAddress(c)
	dkgPacket, err := protoToDKGPacket(p.GetDkg())
	beaconID := p.GetMetadata().GetBeaconID()
	if err != nil {
		b.l.Debugw("", "beacon_id", beaconID, "echoBroadcast", "received invalid packet", "from", addr, "err", err)
		return nil, errors.New("invalid packet")
	}

	hash := hash(dkgPacket.Hash())
	if b.hashes.exists(hash) {
		// if we already seen this one, no need to verify even because that
		// means we already broadcasted it
		b.l.Debugw("", "beacon_id", beaconID, "echoBroadcast", "ignoring duplicate packet", "from", addr, "type", fmt.Sprintf("%T", dkgPacket))
		return new(drand.Empty), nil
	}
	if err := b.verif(dkgPacket); err != nil {
		b.l.Debugw("", "beacon_id", beaconID, "echoBroadcast", "received invalid signature", "from", addr)
		return nil, errors.New("invalid packet")
	}

	b.l.Debugw("", "beacon_id", beaconID, "echoBroadcast",
		"received new packet to echoBroadcast", "from", addr, "type", fmt.Sprintf("%T", dkgPacket))
	b.sendout(hash, dkgPacket, false) // we're using the rate limiting
	b.passToApplication(dkgPacket)
	return new(drand.Empty), nil
}

func (b *echoBroadcast) passToApplication(p packet) {
	switch pp := p.(type) {
	case *dkg.DealBundle:
		b.dealCh <- *pp
	case *dkg.ResponseBundle:
		b.respCh <- *pp
	case *dkg.JustificationBundle:
		b.justCh <- *pp
	default:
		b.l.Errorw("", "echoBroadcast", "application channel full")
	}
}

// sendout converts the packet to protobuf and pass the packet to the dispatcher
// so it is broadcasted out out to all nodes. sendout requires the echoBroadcast
// lock. If bypass is true, the message is directly sent to the peers, bypassing
// the rate limiting in place.
func (b *echoBroadcast) sendout(h []byte, p packet, bypass bool) {
	dkgproto, err := dkgPacketToProto(p)
	if err != nil {
		b.l.Errorw("", "echoBroadcast", "can't send packet", "err", err)
		return
	}
	// we register we saw that packet and we broadcast it
	b.hashes.put(h)

	metadata := common.Metadata{NodeVersion: b.version.ToProto(), BeaconID: b.beaconID}
	proto := &drand.DKGPacket{
		Dkg:      dkgproto,
		Metadata: &metadata,
	}
	if bypass {
		// in a routine cause we don't want to block the processing of the DKG
		// as well - that's ok since we are only expecting to send 3 packets out
		// at the very least.
		go b.dispatcher.broadcastDirect(proto)
	} else {
		b.dispatcher.broadcast(proto)
	}
}

func (b *echoBroadcast) IncomingDeal() <-chan dkg.DealBundle {
	return b.dealCh
}

func (b *echoBroadcast) IncomingResponse() <-chan dkg.ResponseBundle {
	return b.respCh
}

func (b *echoBroadcast) IncomingJustification() <-chan dkg.JustificationBundle {
	return b.justCh
}

func (b *echoBroadcast) Stop() {
	b.dispatcher.stop()
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

type broadcastPacket = *drand.DKGPacket

// maxQueueSize is the maximum queue size we reserve for each destination of
// broadcast.
const maxQueueSize = 1000

// senderQueueSize returns a dynamic queue size depending on the number of nodes
// to contact.
func senderQueueSize(nodes int) int {
	if nodes > maxQueueSize {
		return maxQueueSize
	}
	return nodes
}

// dispatcher maintains a list of worker assigned one destination and pushes the
// message to send to the right worker
type dispatcher struct {
	sync.Mutex
	senders []*sender
}

func newDispatcher(l log.Logger, client net.ProtocolClient, to []*key.Node, us string) *dispatcher {
	var senders = make([]*sender, 0, len(to)-1)
	queue := senderQueueSize(len(to))
	for _, node := range to {
		if node.Address() == us {
			continue
		}
		sender := newSender(l, client, node, queue)
		go sender.run()
		senders = append(senders, sender)
	}
	return &dispatcher{
		senders: senders,
	}
}

// broadcast uses the regular channel limitation for messages coming from other
// nodes.
func (d *dispatcher) broadcast(p broadcastPacket) {
	for _, i := range rand.Perm(len(d.senders)) {
		d.senders[i].sendPacket(p)
	}
}

// broadcastDirect directly send to the other peers - it is used only for our
// own packets so we're not bound to congestion events.
func (d *dispatcher) broadcastDirect(p broadcastPacket) {
	for _, i := range rand.Perm(len(d.senders)) {
		d.senders[i].sendDirect(p)
	}
}

func (d *dispatcher) stop() {
	for _, sender := range d.senders {
		sender.stop()
	}
}

type sender struct {
	l      log.Logger
	client net.ProtocolClient
	to     net.Peer
	newCh  chan broadcastPacket
}

func newSender(l log.Logger, client net.ProtocolClient, to net.Peer, queueSize int) *sender {
	return &sender{
		l:      l,
		client: client,
		to:     to,
		newCh:  make(chan broadcastPacket, queueSize),
	}
}

func (s *sender) sendPacket(p broadcastPacket) {
	beaconID := p.GetMetadata().GetBeaconID()
	select {
	case s.newCh <- p:
	default:
		s.l.Debugw("", "beacon_id", beaconID, "echoBroadcast", "sender queue full", "endpoint", s.to.Address())
	}
}

func (s *sender) run() {
	for newPacket := range s.newCh {
		s.sendDirect(newPacket)
	}
}

func (s *sender) sendDirect(newPacket broadcastPacket) {
	beaconID := newPacket.GetMetadata().GetBeaconID()
	err := s.client.BroadcastDKG(context.Background(), s.to, newPacket)
	if err != nil {
		s.l.Debugw("", "beacon_id", beaconID, "echoBroadcast", "sending out", "error to", s.to.Address(), "err:", err)
	} else {
		s.l.Debugw("", "beacon_id", beaconID, "echoBroadcast", "sending out", "to", s.to.Address())
	}
}

func (s *sender) stop() {
	close(s.newCh)
}
