package dkg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	vss_proto "github.com/dedis/drand/protobuf/crypto/share/vss"
	dkg_proto "github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/kyber"
	"github.com/dedis/kyber/share/dkg/pedersen"
	"github.com/dedis/kyber/share/vss/pedersen"
	"github.com/nikkolasg/slog"
	"google.golang.org/grpc/peer"
)

type Suite = dkg.Suite

const DefaultTimeout = time.Duration(1) * time.Minute

// Config holds all necessary informations to run a dkg protocol. This config is
// transformed to be passed down to the kyber dkg library.
type Config struct {
	Suite    Suite
	Key      *key.Pair
	NewNodes *key.Group
	OldNodes *key.Group

	Share     *key.Share
	Threshold int
}

// Share represents the private information that a node holds after a successful
// DKG. This information MUST stay private !
type Share = dkg.DistKeyShare

// Handler is the stateful struct that runs a DKG with the peers
type Handler struct {
	net           Network     // network to send data out
	conf          *Config     // configuration given at init time
	cdkg          *dkg.Config // dkg config
	private       *key.Pair   // private key
	nidx          int         // the index of the private/public key pair in the new list
	oidx          int
	newNode       bool                       // true if this node belongs in the new group or not
	oldNode       bool                       // true if this node belongs to the oldNode list
	state         *dkg.DistKeyGenerator      // dkg stateful struct
	n             int                        // number of participants
	tmpResponses  map[uint32][]*dkg.Response // temporary buffer of responses
	sentDeals     bool                       // true if the deals have been sent already
	dealProcessed int                        // how many deals have we processed so far
	respProcessed int                        // how many responses have we processed so far
	done          bool                       // is the protocol done
	shareCh       chan Share                 // share gets sent over shareCh when ready
	errCh         chan error                 // any fatal error for the protocol gets sent over

	sync.Mutex
}

// NewHandler returns a fresh dkg handler using this private key.
func NewHandler(n Network, c *Config) (*Handler, error) {
	if c.OldNodes == nil {
		c.OldNodes = c.NewNodes
	}
	var share *dkg.DistKeyShare
	if c.Share != nil {
		s := dkg.DistKeyShare(*c.Share)
		share = &s
	}
	var dpub []kyber.Point
	if c.OldNodes.PublicKey != nil {
		dpub = c.OldNodes.PublicKey.Coefficients
	}
	cdkg := &dkg.Config{
		Suite:        c.Suite.(dkg.Suite),
		Longterm:     c.Key.Key,
		OldNodes:     c.OldNodes.Points(),
		NewNodes:     c.NewNodes.Points(),
		PublicCoeffs: dpub,
		Share:        share,
	}
	state, err := dkg.NewDistKeyHandler(cdkg)
	if err != nil {
		return nil, fmt.Errorf("dkg: error using dkg library: %s", err)
	}

	var newNode bool
	var oldNode bool
	nidx, found := c.NewNodes.Index(c.Key.Public)
	if found {
		newNode = true
	}
	oidx, found := c.OldNodes.Index(c.Key.Public)
	if found {
		oldNode = true
	}
	return &Handler{
		conf:         c,
		cdkg:         cdkg,
		private:      c.Key,
		state:        state,
		net:          n,
		nidx:         nidx,
		oidx:         oidx,
		newNode:      newNode,
		oldNode:      oldNode,
		tmpResponses: make(map[uint32][]*dkg.Response),
		n:            len(cdkg.NewNodes),
		shareCh:      make(chan Share, 1),
		errCh:        make(chan error, 1),
	}, nil
}

// Process process an incoming message from the network.
func (h *Handler) Process(c context.Context, packet *dkg_proto.DKGPacket) {
	peer, _ := peer.FromContext(c)
	switch {
	case packet.Deal != nil:
		h.processDeal(peer, packet.Deal)
	case packet.Response != nil:
		h.processResponse(peer, packet.Response)
	case packet.Justification != nil:
		panic("not yet implemented")
	}
}

// Start sends the first message to run the protocol
func (h *Handler) Start() {
	h.sentDeals = true
	if err := h.sendDeals(); err != nil {
		h.errCh <- err
	}
}

// WaitShare returns a channel over which the share will be sent over when
// ready.
func (h *Handler) WaitShare() chan Share {
	return h.shareCh
}

// WaitError returns a channel over which any fatal error for the protocol is
// sent to.
func (h *Handler) WaitError() chan error {
	return h.errCh
}

// QualifiedGroup returns the group of qualified participants,i.e. the list of
// participants that successfully finished the DKG round without any blaming
// from any other participants. This group must be saved to be re-used later on
// in case of a renewal for the share.
// XXX For the moment it's only taking the new set of nodes completely.
func (h *Handler) QualifiedGroup() *key.Group {
	//quals := h.state.QUAL()
	return key.NewGroup(h.conf.NewNodes.Identities(), h.conf.Threshold)
}

func (h *Handler) processDeal(p *peer.Peer, pdeal *dkg_proto.Deal) {
	h.Lock()
	h.dealProcessed++
	deal := &dkg.Deal{
		Index:     pdeal.Index,
		Signature: pdeal.Signature,
		Deal: &vss.EncryptedDeal{
			DHKey:     pdeal.Deal.Dhkey,
			Signature: pdeal.Deal.Signature,
			Nonce:     pdeal.Deal.Nonce,
			Cipher:    pdeal.Deal.Cipher,
		},
	}
	defer h.processTmpResponses(deal)
	defer h.Unlock()
	slog.Debugf("dkg: %s processing deal from %s (%d processed)", h.addr(), h.raddr(deal.Index, true), h.dealProcessed)
	slog.Debugf("dkg: after processing deal -> h.sentDeals %v && h.oldNode %v", h.sentDeals, h.oldNode)
	resp, err := h.state.ProcessDeal(deal)
	if err != nil {
		slog.Infof("dkg: error processing deal: %s", err)
		return
	}

	if !h.sentDeals && h.oldNode {
		slog.Debugf("dkg: %d sending deals out there", h.oidx)
		h.sentDeals = true
		go func() {
			if err := h.sendDeals(); err != nil {
				slog.Debugf("dkg: %d error sending deals ! %v", h.oidx, err)
				h.errCh <- err
			}
		}()
		slog.Debugf("dkg: sent all deals")
	}

	if h.newNode {
		out := &dkg_proto.DKGPacket{
			Response: &dkg_proto.Response{
				Index: resp.Index,
				Response: &vss_proto.Response{
					SessionId: resp.Response.SessionID,
					Index:     resp.Response.Index,
					Status:    resp.Response.Status,
					Signature: resp.Response.Signature,
				},
			},
		}
		slog.Debugf("dkg: %d broadcasting responses after receiving deal", h.nidx)
		go h.broadcast(out, true)
		slog.Debugf("dkg: broadcasted response")
	}
}

func (h *Handler) processTmpResponses(deal *dkg.Deal) {
	h.Lock()
	defer h.checkCertified()
	defer h.Unlock()
	resps, ok := h.tmpResponses[deal.Index]
	if !ok {
		return
	}
	slog.Debug("dkg: processing ", len(resps), " out-of-order responses for dealer", deal.Index)
	delete(h.tmpResponses, deal.Index)
	for _, r := range resps {
		_, err := h.state.ProcessResponse(r)
		if err != nil {
			slog.Debugf("dkg: err process temp response: %s", err)
		}
	}
}

func (h *Handler) processResponse(p *peer.Peer, presp *dkg_proto.Response) {
	h.Lock()
	defer h.checkCertified()
	defer h.Unlock()
	h.respProcessed++

	resp := &dkg.Response{
		Index: presp.Index,
		Response: &vss.Response{
			SessionID: presp.Response.SessionId,
			Index:     presp.Response.Index,
			Status:    presp.Response.Status,
			Signature: presp.Response.Signature,
		},
	}
	j, err := h.state.ProcessResponse(resp)
	slog.Debugf("dkg: processing response(%d so far) from %s", h.respProcessed, p.Addr)
	if err != nil {
		if strings.Contains(err.Error(), "no deal for it") {
			h.tmpResponses[resp.Index] = append(h.tmpResponses[resp.Index], resp)
			slog.Debugf("dkg: %s storing future response for unknown deal (from %s) %d", h.addr(), p.Addr, resp.Index)
			return
		}
		slog.Infof("dkg: error process response: %s", err)
		slog.Debugf(" -- dkg %d (newNode?%v) response about deal %d from verifier/node %d", h.nidx, h.oldNode, resp.Index, resp.Response.Index)
		return
	}
	if j != nil && h.oldNode {
		// XXX TODO
		slog.Debugf("dkg: broadcasting justification")
		/*packet := &dkg_proto.Packet{*/
		//Justification: &dkg_proto.Justification{
		//Index: j.Index,
		//Justification: &vss.Justification{
		//SessionID: j.Justification.Index,
		//Index: j.Justification.Index,
		//Signature: j.Justification.Signature,

		//}
		//},
		//}
		/*go h.broadcast(packet)*/
	}
	slog.Debugf("dkg: %d processResponse(%d/%d) from %s --> Certified() ? %v --> done ? %v", h.nidx, h.respProcessed, h.n*(h.n-1), p.Addr, h.state.Certified(), h.done)

}

// checkCertified checks if there has been enough responses and if so, creates
// the distributed key share, and sends it along the channel returned by
// WaitShare.
func (h *Handler) checkCertified() {
	h.Lock()
	defer h.Unlock()
	if !h.state.Certified() || h.done {
		return
	}
	h.done = true
	slog.Infof("dkg: own share %d certified!", h.nidx)
	dks, err := h.state.DistKeyShare()
	if err != nil {
		return
	}
	share := Share(*dks)
	h.shareCh <- share
}

// sendDeals tries to send the deals to each of the nodes.
// It returns an error if a number of node superior to the threshold have not
// received the deal. It is basically a no-go.
func (h *Handler) sendDeals() error {
	deals, err := h.state.Deals()
	if err != nil {
		return err
	}
	var good = 1
	slog.Debugf("dkg: %d starting sending deals to new participants", h.oidx)
	ids := h.conf.NewNodes.Identities()
	for i, deal := range deals {
		if i == h.nidx && h.newNode {
			fmt.Printf("dkg %d (%s) has deal for idx %d\n", h.nidx, h.conf.Key.Public.Key.String(), i)
			panic("end of the universe")
		}
		id := ids[i]
		packet := &dkg_proto.DKGPacket{
			Deal: &dkg_proto.Deal{
				Index:     deal.Index,
				Signature: deal.Signature,
				Deal: &vss_proto.EncryptedDeal{
					Dhkey:     deal.Deal.DHKey,
					Signature: deal.Deal.Signature,
					Nonce:     deal.Deal.Nonce,
					Cipher:    deal.Deal.Cipher,
				},
			},
		}

		slog.Debugf("dkg: %d sending deal to %d", h.oidx, i)
		if err := h.net.Send(id, packet); err != nil {
			slog.Printf("dkg: failed to send deal to %s: %s", id.Address(), err)
		} else {
			slog.Debugf("dkg: %d sending deal to %d -- END", h.oidx, i)
			good++
		}
	}
	if good < h.conf.Threshold {
		return fmt.Errorf("dkg: could only send deals to %d / %d (threshold %d)", good, h.n, h.conf.Threshold)
	}
	slog.Infof("dkg: %d sent deals successfully to %d nodes", h.oidx, good-1)
	return nil
}

// The following packets must be sent to the following nodes:
// - Deals are sent to the new nodes only
// - Responses are sent to to both new nodes and old nodes but *only once per
// node*
// - Justification are sent to the new nodes only
func (h *Handler) broadcast(p *dkg_proto.DKGPacket, toOldNodes bool) {
	var sent = make(map[string]bool)
	var good, oldGood int
	for i, id := range h.conf.NewNodes.Identities() {
		if toOldNodes {
			sent[id.Key.String()] = true
		}
		if h.newNode && h.nidx == i {
			continue
		}
		if err := h.net.Send(id, p); err != nil {
			slog.Debugf("dkg: error sending packet to %s: %s", id.Address(), err)
			continue
		}
		slog.Debugf("dkg: %s broadcast: sent packet to %s", h.addr(), id.Address())
		good++
	}

	if toOldNodes {
		for _, id := range h.conf.OldNodes.Identities() {
			// dont send twice to same address
			_, present := sent[id.Key.String()]
			if present {
				continue
			}
			if err := h.net.Send(id, p); err != nil {
				slog.Debugf("dkg: error sending packet to %s: %s", id.Address(), err)
				continue
			}
			slog.Debugf("dkg: %s broadcast: sent packet to %s", h.addr(), id.Address())
			oldGood++
		}

	}
	if good < h.conf.Threshold {
		h.errCh <- errors.New("dkg: broadcast not successful")
	}
	slog.Debugf("dkg: broadcast done")
}

func (h *Handler) addr() string {
	return h.private.Public.Address()
}

func (h *Handler) raddr(i uint32, oldNodes bool) string {
	if oldNodes {
		return h.conf.OldNodes.Public(int(i)).Address()
	} else {
		return h.conf.NewNodes.Public(int(i)).Address()
	}
}

// Network is used by the Handler to send a DKG protocol packet over the network.
// XXX Not really needed, should use the net/protobuf interface instead
type Network interface {
	Send(net.Peer, *dkg_proto.DKGPacket) error
}
