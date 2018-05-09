package beacon

import (
	"context"
	"errors"
	"sync"
	"time"

	proto "github.com/dedis/drand/protobuf/drand"
	"github.com/dedis/kyber/share"
	"github.com/dedis/kyber/sign/tbls"

	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	"github.com/nikkolasg/slog"
)

// How much time can a signature timestamp differ from our local time
var maxTimestampDelta = 10 * time.Second

// Handler holds the logic to initiate, and react to the TBLS protocol. Each time
// a full signature can be recosntructed, it saves it to the given Store.
type Handler struct {
	// to communicate with other drand peers
	client net.Client
	// where to store the new randomness beacon
	store Store
	// to sign beacons
	share *key.Share
	// to verify incoming beacons
	group *key.Group
	// to verify incoming beacons with tbls
	pub *share.PubPoly
	sync.Mutex

	ticker *time.Ticker
	close  chan bool
}

// NewHandler returns a fresh handler ready to serve and create randomness
// beacon
func NewHandler(c net.Client, sh *key.Share, group *key.Group, s Store) *Handler {
	return &Handler{
		client: c,
		group:  group,
		share:  sh,
		pub:    share.NewPubPoly(key.G2, key.G2.Point().Base(), sh.Commits),
		store:  s,
		close:  make(chan bool),
	}
}

// ProcessBeacon receives a request for a beacon partial signature. It replies
// successfully with a valid partial signature over the given beacon packet
// information if the following is true:
// 1- the time for the request is not than a certain time in the past
// 2- the partial signature in the embedded response is valid. This proves that
// the requests comes from a qualified node from the DKG phase.
func (b *Handler) ProcessBeacon(c context.Context, p proto.BeaconRequest) (*proto.BeaconResponse, error) {
	// 1
	now := time.Now()
	leaderTime := time.Unix(int64(p.Timestamp), 0)
	// XXX Check for future time as well!!
	if now.Sub(leaderTime) > maxTimestampDelta {
		slog.Info("blsbeacon received out-of-range timestamp signature request: ", now.Sub(leaderTime))
		return nil, errors.New("beacon: out-of-time timestamp")
	}
	// 2-
	msg := Message(p.PreviousSig, p.Timestamp)
	if err := tbls.Verify(key.Pairing, b.pub, msg, p.PartialSig); err != nil {
		slog.Debugf("beacon: received invalid signature request")
		return nil, err
	}

	signature, err := tbls.Sign(key.Pairing, b.share.Share, msg)
	if err != nil {
		return nil, err
	}

	resp := &proto.BeaconResponse{
		PartialSig: signature,
	}
	return resp, nil
}

// RandomBeacon starts periodically the TBLS protocol. The seed is the first
// message signed alongside with the current timestamp. All subsequent
// signatures are chained:
// s_i+1 = SIG(s_i || timestamp)
func (b *Handler) Loop(seed []byte, period time.Duration) {
	b.Lock()
	b.ticker = time.NewTicker(period)
	b.Unlock()
	var counter uint64 = 1
	var failed uint64
	// to protect the prevSig mutation
	var mut sync.Mutex
	var prevSig []byte = seed
	//for wt := range b.ticker.C {
	closingCh := make(chan bool)
	fn := func(round uint64, closeCh chan bool) {
		now := uint64(time.Now().Unix())
		slog.Debugf("beacon: time %d : round %d", now, round)
		msg := Message(prevSig, uint64(now))
		signature, err := tbls.Sign(key.Pairing, b.share.Share, msg)
		if err != nil {
			slog.Debugf("beacon: err creating beacon: %s", err)
			return
		}
		var sigs [][]byte
		sigs = append(sigs, signature)
		request := &proto.BeaconRequest{
			PreviousSig: prevSig,
			Timestamp:   now,
			PartialSig:  signature,
		}
		respCh := make(chan *proto.BeaconResponse, b.group.Len())
		// send all requests in parallel
		for _, id := range b.group.Nodes {
			// this go routine sends the packet to one node. It will always
			// return assuming there's a timeout on the connection
			go func(i *key.Identity) {
				resp, err := b.client.NewBeacon(id, request)
				if err != nil {
					slog.Debugf("beacon: err receiving beacon response: %s", err)
					return
				}
				if err := tbls.Verify(key.Pairing, b.pub, msg, resp.PartialSig); err != nil {
					slog.Debugf("beacon: invalid beacon response: %s", err)
					return
				}
				respCh <- resp
			}(id.Identity)
		}
		// wait for a threshold of replies or if the timeout occured
		for sigCount := 0; sigCount < b.group.Threshold; sigCount++ {
			select {
			case resp := <-respCh:
				sigs = append(sigs, resp.PartialSig)
			case <-closeCh:
				// it's already time to go to the next, there has been not
				// enough time or nodes are too slow. In any case it's a
				// problem.
				// XXX should be accessed in thread safe manner but highly
				// unlikely that the rounds are that short in practice...
				failed++
				slog.Infof("beacon: quitting prematurely round %d (%d failed).", round, failed)
				slog.Infof("beacon: There might be a problem with the nodes")
				return
			}
		}
		finalSig, err := tbls.Recover(key.Pairing, b.pub, msg, sigs, b.group.Threshold, b.group.Len())
		if err != nil {
			slog.Infof("beacon: could not reconstruct final beacon: %s", err)
			return
		}

		beacon := &Beacon{
			PreviousSig: prevSig,
			Timestamp:   now,
			Signature:   finalSig,
		}
		if err := b.store.Put(beacon); err != nil {
			slog.Infof("beacon: error storing beacon randomness: %s", err)
			return
		}
		mut.Lock()
		prevSig = finalSig
		mut.Unlock()
		slog.Infof("beacon: time %d : round %d finished", now, round)
	}

	// run the loop !
	for {
		select {
		case <-b.ticker.C:
			counter++
			// close the previous operations if still running
			close(closingCh)
			closeCh := make(chan bool)
			// start the new one
			go fn(counter, closeCh)
		case <-b.close:
			return
		}
	}
	slog.Info("beacon: stopped loop")
}

func (h *Handler) Stop() {
	h.Lock()
	defer h.Unlock()
	if h.ticker == nil {
		return
	}
	h.ticker.Stop()
	close(h.close)
	slog.Info("beacon: shutting down")
}
