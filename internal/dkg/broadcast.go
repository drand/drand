package dkg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"

	commonutils "github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/common/tracer"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/net"
	"github.com/drand/drand/v2/internal/util"
	pdkg "github.com/drand/drand/v2/protobuf/dkg"
	"github.com/drand/drand/v2/protobuf/drand"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share/dkg"
)

// Broadcast is an interface that represents the minimum functionality required
// by drand to both (1) be the interface between drand and the dkg logic and (2)
// implement the broadcasting mechanism.
type Broadcast interface {
	dkg.Board
	BroadcastDKG(ctx context.Context, p *pdkg.DKGPacket) error
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
	ctx context.Context
	sync.Mutex
	l        log.Logger
	version  commonutils.Version
	beaconID string
	// responsible for sending out the messages
	dispatcher *dispatcher
	// list of messages already retransmitted comparison by hash
	hashes    set
	dealCh    chan dkg.DealBundle
	respCh    chan dkg.ResponseBundle
	justCh    chan dkg.JustificationBundle
	scheme    *crypto.Scheme
	config    dkg.Config
	isStopped bool
}

type packet = dkg.Packet

var _ Broadcast = (*echoBroadcast)(nil)

func newEchoBroadcast(
	ctx context.Context,
	client net.DKGClient,
	l log.Logger,
	version commonutils.Version,
	beaconID string,
	own string,
	to []*pdkg.Participant,
	scheme *crypto.Scheme,
	config *dkg.Config,
) (*echoBroadcast, error) {
	if len(to) == 0 {
		return nil, errors.New("cannot create a broadcaster with no participants")
	}
	// copy the config to avoid races
	c := *config
	return &echoBroadcast{
		ctx:        ctx,
		l:          l.Named("echoBroadcast"),
		version:    version,
		beaconID:   beaconID,
		dispatcher: newDispatcher(ctx, client, l, to, own),
		dealCh:     make(chan dkg.DealBundle, len(to)),
		respCh:     make(chan dkg.ResponseBundle, len(to)),
		justCh:     make(chan dkg.JustificationBundle, len(to)),
		hashes:     new(arraySet),
		scheme:     scheme,
		config:     c,
		isStopped:  false,
	}, nil
}

func (b *echoBroadcast) PushDeals(bundle *dkg.DealBundle) {
	ctx, span := tracer.NewSpan(b.ctx, "b.PushDeals")
	defer span.End()

	b.dealCh <- *bundle
	b.Lock()
	defer b.Unlock()
	h := hash(bundle.Hash())
	b.l.Infow("push broadcast", "deal", fmt.Sprintf("%x", h[:5]))
	b.sendout(ctx, h, bundle, true, b.beaconID)
}

func (b *echoBroadcast) PushResponses(bundle *dkg.ResponseBundle) {
	ctx, span := tracer.NewSpan(b.ctx, "b.PushResponses")
	defer span.End()

	b.respCh <- *bundle
	b.Lock()
	defer b.Unlock()
	h := hash(bundle.Hash())
	b.l.Debugw("push", "response", bundle.String())
	b.sendout(ctx, h, bundle, true, b.beaconID)
}

func (b *echoBroadcast) PushJustifications(bundle *dkg.JustificationBundle) {
	ctx, span := tracer.NewSpan(b.ctx, "b.PushJustifications")
	defer span.End()

	b.justCh <- *bundle
	b.Lock()
	defer b.Unlock()
	h := hash(bundle.Hash())
	b.l.Debugw("push", "justification", fmt.Sprintf("%x", h[:5]))
	b.sendout(ctx, h, bundle, true, b.beaconID)
}

func (b *echoBroadcast) BroadcastDKG(ctx context.Context, p *pdkg.DKGPacket) error {
	ctx, span := tracer.NewSpan(ctx, "b.BroadcastDKG")
	defer span.End()

	b.Lock()
	defer b.Unlock()

	addr := net.RemoteAddress(ctx)
	dkgPacket, err := protoToDKGPacket(p.GetDkg(), b.scheme)
	if err != nil {
		b.l.Errorw("received invalid packet DKGPacket", "from", addr, "err", err)
		err := errors.New("invalid DKGPacket")
		span.RecordError(err)
		return err
	}

	hash := hash(dkgPacket.Hash())
	if b.hashes.exists(hash) {
		// if we've already seen this one, no need to verify even because that
		// means we already broadcasted it
		b.l.Debugw("ignoring duplicate packet", "index", dkgPacket.Index(), "from", addr, "type", fmt.Sprintf("%T", dkgPacket))
		return nil
	}

	dkgConfig := b.config
	if err := dkg.VerifyPacketSignature(&dkgConfig, dkgPacket); err != nil {
		b.l.Errorw("received invalid signature", "from", addr, "signature", dkgPacket.Sig(), "scheme", b.scheme, "err", err)
		err := errors.New("invalid DKGPacket")
		span.RecordError(err)
		return err
	}

	b.l.Debugw("received new packet to echoBroadcast", "from", addr, "packet index", dkgPacket.Index(), "type", fmt.Sprintf("%T", dkgPacket))
	b.sendout(ctx, hash, dkgPacket, false, b.beaconID) // we're using the rate limiting
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
func (b *echoBroadcast) sendout(ctx context.Context, h []byte, p packet, bypass bool, beaconID string) {
	ctx, span := tracer.NewSpan(ctx, "b.sendout")
	defer span.End()

	if b.isStopped {
		return
	}

	dkgproto, err := dkgPacketToProto(p, beaconID)
	if err != nil {
		b.l.Errorw("can't send packet", "err", err)
		return
	}
	// we register we saw that packet and we broadcast it
	b.hashes.put(h)

	proto := &pdkg.DKGPacket{Dkg: dkgproto}
	if bypass {
		// in a routine cause we don't want to block the processing of the DKG
		// as well - that's ok since we are only expecting to send 3 packets out
		// at most.
		go b.dispatcher.broadcastDirect(ctx, proto)
	} else {
		b.dispatcher.broadcast(ctx, proto)
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
	b.Lock()
	b.isStopped = true
	b.Unlock()
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

type broadcastPacket = *pdkg.DKGPacket

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
	return nodes * 3 //nolint:mnd
}

// dispatcher maintains a list of worker assigned one destination and pushes the
// message to send to the right worker
type dispatcher struct {
	sync.Mutex
	senders []*sender
}

func newDispatcher(ctx context.Context, dkgClient net.DKGClient, l log.Logger, to []*pdkg.Participant, us string) *dispatcher {
	ctx, span := tracer.NewSpan(ctx, "newDispatcher")
	defer span.End()

	var senders = make([]*sender, 0, len(to)-1)
	queue := senderQueueSize(len(to))
	for _, node := range to {
		if node.Address == us {
			continue
		}
		sender := newSender(dkgClient, node, l, queue)
		go sender.run(ctx)
		senders = append(senders, sender)
	}
	return &dispatcher{
		senders: senders,
	}
}

// broadcast uses the regular channel limitation for messages coming from other
// nodes.
func (d *dispatcher) broadcast(ctx context.Context, p broadcastPacket) {
	ctx, span := tracer.NewSpanFromContext(context.Background(), ctx, "d.broadcast")
	defer span.End()

	for _, i := range rand.Perm(len(d.senders)) {
		d.senders[i].sendPacket(ctx, p)
	}
}

// broadcastDirect directly send to the other peers - it is used only for our
// own packets so we're not bound to congestion events.
func (d *dispatcher) broadcastDirect(ctx context.Context, p broadcastPacket) {
	ctx, span := tracer.NewSpan(ctx, "d.broadcastDirect")
	defer span.End()

	for _, i := range rand.Perm(len(d.senders)) {
		d.senders[i].sendDirect(ctx, p)
	}
}

func (d *dispatcher) stop() {
	for _, sender := range d.senders {
		sender.stop()
	}
}

type sender struct {
	l      log.Logger
	client net.DKGClient
	to     *pdkg.Participant
	newCh  chan broadcastPacket
}

func newSender(client net.DKGClient, to *pdkg.Participant, l log.Logger, queueSize int) *sender {
	return &sender{
		l:      l.Named("Sender"),
		client: client,
		to:     to,
		newCh:  make(chan broadcastPacket, queueSize),
	}
}

func (s *sender) sendPacket(ctx context.Context, p broadcastPacket) {
	_, span := tracer.NewSpan(ctx, "s.sendPacket")
	defer span.End()

	select {
	case s.newCh <- p:
	default:
		s.l.Errorw("sender queue full", "endpoint", s.to.Address)
	}
}

func (s *sender) run(ctx context.Context) {
	ctx, span := tracer.NewSpanFromContext(context.Background(), ctx, "s.run")
	defer span.End()

	for newPacket := range s.newCh {
		s.sendDirect(ctx, newPacket)
	}
}

func (s *sender) sendDirect(ctx context.Context, newPacket broadcastPacket) {
	ctx, span := tracer.NewSpanFromContext(context.Background(), ctx, "s.sendDirect")
	defer span.End()

	node := util.ToPeer(s.to)
	_, err := s.client.BroadcastDKG(ctx, node, newPacket)
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
	packet.Metadata = &drand.Metadata{
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
	packet.Metadata = &drand.Metadata{
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
	packet.Metadata = &drand.Metadata{
		BeaconID: beaconID,
	}
	return packet
}
