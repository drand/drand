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
	dkg_crypto "github.com/dedis/drand/protobuf/crypto/share/dkg"
	"github.com/dedis/drand/protobuf/crypto/share/vss"
	dkg_proto "github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/kyber"
	"github.com/dedis/kyber/share/dkg/pedersen"
	"github.com/nikkolasg/slog"
	"google.golang.org/grpc/peer"
)

// Config is given to a DKG handler and contains all needed parameters to
// successfully run the DKG protocol.
type Config struct {
	Suite     dkg.Suite       // which crypto group to use for this DKG run
	List      []*key.Identity // the list of participants
	Threshold int             // the threshold of active participants needed
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
	list := conf.List
	t := conf.Threshold
	points := make([]kyber.Point, len(list), len(list))
	myIdx := -1
	myPoint := priv.Public.Key
	for i := range list {
		point := list[i].Key
		points[i] = point
		if point.Equal(myPoint) {
			myIdx = i
		}
	}
	if myIdx == -1 {
		return nil, errors.New("dkg: no nublic key corresponding in the given list")
	}
	state, err := dkg.NewDistKeyGenerator(conf.Suite, priv.Key, points, t)
	if err != nil {
		return nil, fmt.Errorf("dkg: error using dkg library: %s", err)
	}
	return &Handler{
		conf:         conf,
		state:        state,
		net:          n,
		tmpResponses: make(map[uint32][]*dkg.Response),
		idx:          myIdx,
		n:            len(list),
		shareCh:      make(chan Share, 1),
		errCh:        make(chan error, 1),
	}, nil
}

// Process process an incoming message from the network.
func (h *Handler) Process(c context.Context, packet *Packet) {
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
	// XXX catch the error
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

func (h *Handler) processDeal(p *peer.Peer, deal *dkg.Deal) {
	h.Lock()
	h.dealProcessed++
	slog.Debugf("dkg: processing deal from %s (%d processed)", p.Addr, h.dealProcessed)
	resp, err := h.state.ProcessDeal(deal)
	defer h.processTmpResponses(deal)
	defer h.Unlock()
	if err != nil {
		slog.Infof("dkg: error processing deal: %s", err)
		return
	}

	if !h.sentDeals {
		h.sendDeals()
		h.sentDeals = true
		slog.Debugf("dkg: sent all deals")
	}
	out := &Packet{
		Response: resp,
	}
	h.broadcast(out)
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

func (h *Handler) processResponse(p *peer.Peer, resp *dkg.Response) {
	h.Lock()
	defer h.checkCertified()
	defer h.Unlock()
	h.respProcessed++
	j, err := h.state.ProcessResponse(resp)
	slog.Debugf("dkg: processing response(%d so far) from %s", h.respProcessed, p.Addr)
	if err != nil {
		if strings.Contains(err.Error(), "no deal for it") {
			h.tmpResponses[resp.Index] = append(h.tmpResponses[resp.Index], resp)
			slog.Debug("dkg: storing future response for unknown deal ", resp.Index)
			return
		}
		slog.Infof("dkg: error process response: %s", err)
		return
	}
	if j != nil {
		slog.Debugf("dkg: broadcasting justification")
		packet := &Packet{
			Justification: j,
		}
		go h.broadcast(packet)
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
		id := h.conf.List[i]
		packet := &dkg_proto.DKGPacket{
			Packet: &dkg_crypto.Deal{
				Index: deal.Index,
				Deal: &vss.EncryptedDeal{
					DhKey:      deal.Deal.DHKey,
					Signature:  deal.Deal.Signature,
					Nonce:      deal.Deal.Nonce,
					Ciphertext: deal.Deal.Cipher,
				},
			},
		}

		//fmt.Printf("%s sending deal to %s\n", d.addr, pub.Address)
		if err := h.net.Send(id, packet); err != nil {
			slog.Debugf("dkg: failed to send deal to %s: %s", pub.Address, err)
		} else {
			good++
		}
	}
	if good < h.conf.Threshold {
		return fmt.Errorf("dkg: could only send deals to %d / %d (threshold %d)", good, h.n, h.conf.Threshold)
	}
	slog.Infof("dkg: sent deals successfully to %d nodes", good-1)
	return nil
}

func (h *Handler) broadcast(p *Packet) {
	var good int
	for i, id := range h.conf.List {
		if i == h.idx {
			continue
		}
		if err := h.net.Send(id, p); err != nil {
			slog.Debugf("dkg: error sending packet to %s: %s", id.Address, err)
		}
		good++
	}
	if good < h.conf.Threshold {
		h.errCh <- errors.New("dkg: broadcast not successful")
	}
	slog.Debugf("dkg: broadcast done")
}

// Network is used by the Handler to send a DKG protocol packet over the network.
type Network interface {
	Send(net.Peer, *dkg_proto.DKGPacket) error
}

func validateConf(conf *Config) error {
	// XXX TODO
	return nil
}
