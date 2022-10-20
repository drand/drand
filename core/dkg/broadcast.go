package dkg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/drand/drand/crypto"
	"math/rand"
	"sync"

	"github.com/drand/drand/protobuf/common"
	pdkg "github.com/drand/drand/protobuf/crypto/dkg"
	"github.com/drand/kyber"

	commonutils "github.com/drand/drand/common"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/share/dkg"
)

// Broadcast is an interface that represents the minimum functionality required
// by drand to both (1) be the interface between drand and the dkg logic and (2)
// implement the broadcasting mechanism.
type Broadcast interface {
	dkg.Board
	BroadcastDKG(c context.Context, p *drand.DKGPacket) error
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
	verif  verifyPacket
	scheme *crypto.Scheme
}

type packet = dkg.Packet

var _ Broadcast = (*echoBroadcast)(nil)

// verifyPacket is a type for  a function that can verify the validity of a dkg
// Packet, namely that the signature is correct.
type verifyPacket func(packet) error

func newEchoBroadcast(
	l log.Logger,
	version commonutils.Version,
	beaconID string,
	own string,
	to []*drand.Participant,
	scheme *crypto.Scheme,
) (*echoBroadcast, error) {
	if len(to) == 0 {
		return nil, errors.New("cannot create a broadcaster with no participants")
	}
	dispatcher, err := newDispatcher(l, to, own)
	if err != nil {
		return nil, err
	}
	return &echoBroadcast{
		l:          l.Named("echoBroadcast"),
		version:    version,
		beaconID:   beaconID,
		dispatcher: dispatcher,
		dealCh:     make(chan dkg.DealBundle, len(to)),
		respCh:     make(chan dkg.ResponseBundle, len(to)),
		justCh:     make(chan dkg.JustificationBundle, len(to)),
		hashes:     new(arraySet),
		scheme:     scheme,
	}, nil
}

func (b *echoBroadcast) PushDeals(bundle *dkg.DealBundle) {
	b.dealCh <- *bundle
	b.Lock()
	defer b.Unlock()
	h := hash(bundle.Hash())
	b.l.Infow("push broadcast", "deal", fmt.Sprintf("%x", h[:5]))
	b.sendout(h, bundle, true, b.beaconID)
}

func (b *echoBroadcast) PushResponses(bundle *dkg.ResponseBundle) {
	b.respCh <- *bundle
	b.Lock()
	defer b.Unlock()
	h := hash(bundle.Hash())
	b.l.Debugw("push", "response", bundle.String())
	b.sendout(h, bundle, true, b.beaconID)
}

func (b *echoBroadcast) PushJustifications(bundle *dkg.JustificationBundle) {
	b.justCh <- *bundle
	b.Lock()
	defer b.Unlock()
	h := hash(bundle.Hash())
	b.l.Debugw("push", "justification", fmt.Sprintf("%x", h[:5]))
	b.sendout(h, bundle, true, b.beaconID)
}

func (b *echoBroadcast) BroadcastDKG(c context.Context, p *drand.DKGPacket) error {
	b.Lock()
	defer b.Unlock()

	addr := net.RemoteAddress(c)
	dkgPacket, err := protoToDKGPacket(p.GetDkg(), b.scheme)
	if err != nil {
		b.l.Errorw("received invalid packet DKGPacket", "from", addr, "err", err)
		return errors.New("invalid DKGPacket")
	}

	hash := hash(dkgPacket.Hash())
	if b.hashes.exists(hash) {
		// if we've already seen this one, no need to verify even because that
		// means we already broadcasted it
		b.l.Debugw("ignoring duplicate packet", "index", dkgPacket.Index(), "from", addr, "type", fmt.Sprintf("%T", dkgPacket))
		return nil
	}
	if err := b.verif(dkgPacket); err != nil {
		b.l.Errorw("received invalid signature", "from", addr, "signature", dkgPacket.Sig(), "scheme", b.scheme, "err", err)
		return errors.New("invalid DKGPacket")
	}

	b.l.Debugw("received new packet to echoBroadcast", "from", addr, "packet index", dkgPacket.Index(), "type", fmt.Sprintf("%T", dkgPacket))
	b.sendout(hash, dkgPacket, false, b.beaconID) // we're using the rate limiting
	b.passToApplication(dkgPacket)
	return nil
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
		b.l.Errorw("application channel full")
	}
}

// sendout converts the packet to protobuf and pass the packet to the dispatcher
// so it is broadcasted out to all nodes. sendout requires the echoBroadcast
// lock. If bypass is true, the message is directly sent to the peers, bypassing
// the rate limiting in place.
func (b *echoBroadcast) sendout(h []byte, p packet, bypass bool, beaconID string) {
	dkgproto, err := dkgPacketToProto(p, beaconID)
	if err != nil {
		b.l.Errorw("can't send packet", "err", err)
		return
	}
	// we register we saw that packet and we broadcast it
	b.hashes.put(h)

	proto := &drand.DKGPacket{Dkg: dkgproto, Metadata: &drand.DKGMetadata{BeaconID: beaconID}}
	if bypass {
		// in a routine cause we don't want to block the processing of the DKG
		// as well - that's ok since we are only expecting to send 3 packets out
		// at most.
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
	// we have 3 steps
	return nodes * 3 //nolint:gomnd
}

// dispatcher maintains a list of worker assigned one destination and pushes the
// message to send to the right worker
type dispatcher struct {
	sync.Mutex
	senders []*sender
}

func newDispatcher(l log.Logger, to []*drand.Participant, us string) (*dispatcher, error) {
	var senders = make([]*sender, 0, len(to)-1)
	queue := senderQueueSize(len(to))
	for _, node := range to {
		if node.Address == us {
			continue
		}
		sender, err := newSender(l, node, queue)
		if err != nil {
			return nil, err
		}
		go sender.run()
		senders = append(senders, sender)
	}
	return &dispatcher{
		senders: senders,
	}, nil
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
	client drand.DKGClient
	to     *drand.Participant
	newCh  chan broadcastPacket
}

func newSender(l log.Logger, to *drand.Participant, queueSize int) (*sender, error) {
	client, err := net.NewDKGClient(to.Address, to.Tls)
	if err != nil {
		return nil, err
	}
	return &sender{
		l:      l.Named("Sender"),
		client: client,
		to:     to,
		newCh:  make(chan broadcastPacket, queueSize),
	}, nil
}

func (s *sender) sendPacket(p broadcastPacket) {
	select {
	case s.newCh <- p:
	default:
		s.l.Errorw("sender queue full", "endpoint", s.to.Address)
	}
}

func (s *sender) run() {
	for newPacket := range s.newCh {
		s.sendDirect(newPacket)
	}
}

func (s *sender) sendDirect(newPacket broadcastPacket) {
	_, err := s.client.BroadcastDKG(context.Background(), newPacket)
	if err != nil {
		s.l.Errorw("error while sending out", "to", s.to.Address, "err:", err)
	} else {
		s.l.Debugw("sending out", "to", s.to.Address)
	}
}

func (s *sender) stop() {
	close(s.newCh)
}

func protoToDKGPacket(d *pdkg.Packet, sch *crypto.Scheme) (dkg.Packet, error) {
	switch packet := d.GetBundle().(type) {
	case *pdkg.Packet_Deal:
		return protoToDeal(packet.Deal, sch)
	case *pdkg.Packet_Response:
		return protoToResp(packet.Response), nil
	case *pdkg.Packet_Justification:
		return protoToJustif(packet.Justification, sch)
	default:
		return nil, errors.New("unknown packet")
	}
}

func dkgPacketToProto(p dkg.Packet, beaconID string) (*pdkg.Packet, error) {
	switch inner := p.(type) {
	case *dkg.DealBundle:
		return dealToProto(inner, beaconID), nil
	case *dkg.ResponseBundle:
		return respToProto(inner, beaconID), nil
	case *dkg.JustificationBundle:
		return justifToProto(inner, beaconID), nil
	default:
		return nil, errors.New("invalid dkg packet")
	}
}

func protoToDeal(d *pdkg.DealBundle, sch *crypto.Scheme) (*dkg.DealBundle, error) {
	bundle := new(dkg.DealBundle)
	bundle.DealerIndex = d.DealerIndex
	publics := make([]kyber.Point, 0, len(d.Commits))
	for _, c := range d.Commits {
		coeff := sch.KeyGroup.Point()
		if err := coeff.UnmarshalBinary(c); err != nil {
			return nil, fmt.Errorf("invalid public coeff:%w", err)
		}
		publics = append(publics, coeff)
	}
	bundle.Public = publics
	deals := make([]dkg.Deal, 0, len(d.Deals))
	for _, dd := range d.Deals {
		deal := dkg.Deal{
			EncryptedShare: dd.EncryptedShare,
			ShareIndex:     dd.ShareIndex,
		}
		deals = append(deals, deal)
	}
	bundle.Deals = deals
	bundle.SessionID = d.SessionId
	bundle.Signature = d.Signature
	return bundle, nil
}

func protoToResp(r *pdkg.ResponseBundle) *dkg.ResponseBundle {
	resp := new(dkg.ResponseBundle)
	resp.ShareIndex = r.ShareIndex
	resp.Responses = make([]dkg.Response, 0, len(r.Responses))
	for _, rr := range r.Responses {
		response := dkg.Response{
			DealerIndex: rr.DealerIndex,
			Status:      rr.Status,
		}
		resp.Responses = append(resp.Responses, response)
	}
	resp.SessionID = r.SessionId
	resp.Signature = r.Signature
	return resp
}

func protoToJustif(j *pdkg.JustificationBundle, sch *crypto.Scheme) (*dkg.JustificationBundle, error) {
	just := new(dkg.JustificationBundle)
	just.DealerIndex = j.DealerIndex
	just.Justifications = make([]dkg.Justification, len(j.Justifications))
	for i, j := range j.Justifications {
		share := sch.KeyGroup.Scalar()
		if err := share.UnmarshalBinary(j.Share); err != nil {
			return nil, fmt.Errorf("invalid share: %w", err)
		}
		justif := dkg.Justification{
			ShareIndex: j.ShareIndex,
			Share:      share,
		}
		just.Justifications[i] = justif
	}
	just.SessionID = j.SessionId
	just.Signature = j.Signature
	return just, nil
}

func dealToProto(d *dkg.DealBundle, beaconID string) *pdkg.Packet {
	packet := new(pdkg.Packet)
	bundle := new(pdkg.DealBundle)
	bundle.DealerIndex = d.DealerIndex
	bundle.Deals = make([]*pdkg.Deal, len(d.Deals))
	for i, deal := range d.Deals {
		pdeal := &pdkg.Deal{
			ShareIndex:     deal.ShareIndex,
			EncryptedShare: deal.EncryptedShare,
		}
		bundle.Deals[i] = pdeal
	}

	bundle.Commits = make([][]byte, len(d.Public))
	for i, coeff := range d.Public {
		cbuff, _ := coeff.MarshalBinary()
		bundle.Commits[i] = cbuff
	}
	bundle.Signature = d.Signature
	bundle.SessionId = d.SessionID
	packet.Bundle = &pdkg.Packet_Deal{Deal: bundle}
	packet.Metadata = &common.Metadata{
		BeaconID: beaconID,
	}
	return packet
}

func respToProto(r *dkg.ResponseBundle, beaconID string) *pdkg.Packet {
	packet := new(pdkg.Packet)
	bundle := new(pdkg.ResponseBundle)
	bundle.ShareIndex = r.ShareIndex
	bundle.Responses = make([]*pdkg.Response, len(r.Responses))
	for i, resp := range r.Responses {
		presp := &pdkg.Response{
			DealerIndex: resp.DealerIndex,
			Status:      resp.Status,
		}
		bundle.Responses[i] = presp
	}
	bundle.SessionId = r.SessionID
	bundle.Signature = r.Signature
	packet.Bundle = &pdkg.Packet_Response{Response: bundle}
	packet.Metadata = &common.Metadata{
		BeaconID: beaconID,
	}
	return packet
}

func justifToProto(j *dkg.JustificationBundle, beaconID string) *pdkg.Packet {
	packet := new(pdkg.Packet)
	bundle := new(pdkg.JustificationBundle)
	bundle.DealerIndex = j.DealerIndex
	bundle.Justifications = make([]*pdkg.Justification, len(j.Justifications))
	for i, just := range j.Justifications {
		shareBuff, _ := just.Share.MarshalBinary()
		pjust := &pdkg.Justification{
			ShareIndex: just.ShareIndex,
			Share:      shareBuff,
		}
		bundle.Justifications[i] = pjust
	}
	bundle.SessionId = j.SessionID
	bundle.Signature = j.Signature
	packet.Bundle = &pdkg.Packet_Justification{Justification: bundle}
	packet.Metadata = &common.Metadata{
		BeaconID: beaconID,
	}
	return packet
}
