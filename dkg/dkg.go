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
	"github.com/dedis/kyber/share/dkg/pedersen"
	"github.com/dedis/kyber/share/vss/pedersen"
	"github.com/dedis/kyber/util/random"
	"github.com/nikkolasg/slog"
	"google.golang.org/grpc/peer"
)

type Suite = dkg.Suite

const DefaultTimeout = time.Duration(1) * time.Minute

// Config is given to a DKG handler and contains all needed parameters to
// successfully run the DKG protocol.
type Config struct {
	Suite dkg.Suite // which crypto group to use for this DKG run
	Group *key.Group
	// XXX Currently not in use / tested
	Timeout time.Duration // after timeout, protocol is finished in any cases.
}

// Share represents the private information that a node holds after a successful
// DKG. This information MUST stay private !
type Share = dkg.DistKeyShare

// Handler is the stateful struct that runs a DKG with the peers
type Handler struct {
	net           Network                    // network to send data out
	conf          *Config                    // configuration given at init time
	private       *key.Private               // private key
	idx           int                        // the index of the private/public key pair in the list
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
func NewHandler(priv *key.Private, conf *Config, n Network) (*Handler, error) {
	if err := validateConf(conf); err != nil {
		return nil, err
	}
	t := conf.Group.Threshold
	points := conf.Group.Points()
	myIdx, ok := conf.Group.Index(priv.Public)
	if !ok {
		return nil, errors.New("dkg: no nublic key corresponding in the given list")
	}
	randomSecret := conf.Suite.Scalar().Pick(random.New())
	state, err := dkg.NewDistKeyGenerator(conf.Suite, priv.Key, points, t, randomSecret)
	if err != nil {
		return nil, fmt.Errorf("dkg: error using dkg library: %s", err)
	}
	return &Handler{
		conf:         conf,
		private:      priv,
		state:        state,
		net:          n,
		tmpResponses: make(map[uint32][]*dkg.Response),
		idx:          myIdx,
		n:            conf.Group.Len(),
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
		h.done = true
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

func (h *Handler) processDeal(p *peer.Peer, pdeal *dkg_proto.Deal) {
	h.Lock()
	h.dealProcessed++
	deal := &dkg.Deal{
		Index: pdeal.Index,
		Deal: &vss.EncryptedDeal{
			DHKey:     pdeal.Deal.Dhkey,
			Signature: pdeal.Deal.Signature,
			Nonce:     pdeal.Deal.Nonce,
			Cipher:    pdeal.Deal.Cipher,
		},
	}
	defer h.processTmpResponses(deal)
	defer h.Unlock()
	slog.Debugf("dkg: %s processing deal from %s (%d processed)", h.addr(), h.raddr(deal.Index), h.dealProcessed)
	resp, err := h.state.ProcessDeal(deal)
	if err != nil {
		slog.Infof("dkg: error processing deal: %s", err)
		return
	}

	if !h.sentDeals {
		go h.sendDeals()
		h.sentDeals = true
		slog.Debugf("dkg: sent all deals")
	}
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
	go h.broadcast(out)
	slog.Debugf("dkg: broadcasted response")
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
			slog.Debugf("dkg: err process temp response: ", err)
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
		return
	}
	if j != nil {
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
	slog.Debugf("dkg: processResponse(%d/%d) from %s --> Certified() ? %v --> done ? %v", h.respProcessed, h.n*(h.n-1), p.Addr, h.state.Certified(), h.done)
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
	//slog.Debugf("%s: processResponse(%d) from %s #3", d.addr, d.respProcessed, pub.Address)
	h.done = true
	slog.Infof("dkg: certified!")
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
	for i, deal := range deals {
		if i == h.idx {
			panic("end of the universe")
		}
		id := h.conf.Group.Public(i)
		packet := &dkg_proto.DKGPacket{
			Deal: &dkg_proto.Deal{
				Index: deal.Index,
				Deal: &vss_proto.EncryptedDeal{
					Dhkey:     deal.Deal.DHKey,
					Signature: deal.Deal.Signature,
					Nonce:     deal.Deal.Nonce,
					Cipher:    deal.Deal.Cipher,
				},
			},
		}

		slog.Printf("dkg: %s sending deal to %s", h.addr(), id.Address())
		if err := h.net.Send(id, packet); err != nil {
			slog.Printf("dkg: failed to send deal to %s: %s", id.Address(), err)
		} else {
			good++
		}
		slog.Printf("dkg: %s sending deal to %s STOOOPPPPPPP\n", h.addr(), id.Address())
	}
	if good < h.conf.Group.Threshold {
		return fmt.Errorf("dkg: could only send deals to %d / %d (threshold %d)", good, h.n, h.conf.Group.Threshold)
	}
	slog.Infof("dkg: sent deals successfully to %d nodes", good-1)
	return nil
}

func (h *Handler) broadcast(p *dkg_proto.DKGPacket) {
	var good int
	for i, id := range h.conf.Group.Nodes {
		if i == h.idx {
			continue
		}
		if err := h.net.Send(id, p); err != nil {
			slog.Debugf("dkg: error sending packet to %s: %s", id.Address(), err)
		}
		slog.Debugf("dkg: %s broadcast: sent packet to %s", h.addr(), id.Address())
		good++
	}
	if good < h.conf.Group.Threshold {
		h.errCh <- errors.New("dkg: broadcast not successful")
	}
	slog.Debugf("dkg: broadcast done")
}

func (h *Handler) addr() string {
	return h.private.Public.Address()
}

func (h *Handler) raddr(i uint32) string {
	return h.conf.Group.Public(int(i)).Address()
}

// Network is used by the Handler to send a DKG protocol packet over the network.
type Network interface {
	Send(net.Peer, *dkg_proto.DKGPacket) error
}

func validateConf(conf *Config) error {
	// XXX TODO
	return nil
}
