package main

import (
	"fmt"
	"sync"
	"time"

	kyber "gopkg.in/dedis/kyber.v1"
	"gopkg.in/dedis/kyber.v1/share"

	"github.com/dedis/drand/bls"
	"github.com/nikkolasg/slog"
)

// How much time can a signature timestamp differ from our local time
var maxTimestampDelta = 10 * time.Second

// Beacon holds the logic to initiate, and react to the TBLS protocol. Each time
// a full signature can be recosntructed, it saves it to the given Store.
type Beacon struct {
	r         *Router
	share     *Share
	group     *Group
	pub       *share.PubPoly
	store     Store
	threshold int
	sync.Mutex

	pendingSigs map[string][]*bls.ThresholdSig

	newSig chan []byte
	ticker *time.Ticker
}

// newBlsBeacon
func newBlsBeacon(sh *Share, group *Group, r *Router, s Store) *Beacon {
	return &Beacon{
		r:           r,
		group:       group,
		share:       sh,
		pub:         share.NewPubPoly(g2, g2.Point().Base(), sh.Commits),
		threshold:   len(sh.Commits),
		pendingSigs: make(map[string][]*bls.ThresholdSig),
		store:       s,
	}
}

// RandomBeacon starts periodically the TBLS protocol. The seed is the first
// message signed alongside with the current timestamp. All subsequent
// signatures are chained:
// s_i+1 = SIG(s_i || timestamp)
// For the moment, each resulting signature is stored in a file named
// beacons/<timestamp>.sig (because of FileStore).
func (b *Beacon) Start(seed []byte, period time.Duration) {
	b.Lock()
	b.newSig = make(chan []byte, 1)
	b.ticker = time.NewTicker(period)
	b.Unlock()

	var counter uint64 = 1
	var failed uint64
	b.newSig <- seed
	for wt := range b.ticker.C {
		oldSig := <-b.newSig
		t := wt.Unix()
		slog.Debugf("beacon: (round %d) BIP time %d", counter, t)
		now := time.Now().Unix()
		partial := b.genPartialSignature(oldSig, now)
		request := &BeaconRequest{
			PreviousSig: oldSig,
			Timestamp:   now,
		}
		packet := &DrandPacket{
			Beacon: &BeaconPacket{
				Request: request,
			},
		}
		if err := b.r.Broadcast(b.group, packet); err != nil {
			failed++
			slog.Infof("beacon: start round %d failed (%d total failed)", counter, failed)
			continue
		}

		// send our own contribution
		packet = &DrandPacket{
			Beacon: &BeaconPacket{
				Reply: &BeaconReply{
					Request:   request,
					Signature: partial,
				},
			},
		}
		if err := b.r.Broadcast(b.group, packet); err != nil {
			slog.Infof("beacon: round %d failed to send own contribution: %s", counter, err)
		}
		slog.Infof("beacon: (round %d) %d launched", counter, t)
		counter++
	}
	fmt.Println(" << beacon ticker OUT >>")
}

// processBeaconPacket looks if the packet is a signature request or a signature
// reply and acts accordingly.
func (b *Beacon) processBeaconPacket(pub *Public, msg *BeaconPacket) {
	switch {
	case msg.Request != nil:
		b.processBeaconRequest(pub, msg.Request)
	case msg.Reply != nil:
		b.processBeaconSignature(pub, msg.Reply)
	default:
		slog.Info("beacon received unknown bls beacon message")
	}
}

// processBeaconRequest process the beacon packet in two steps:
// 1- verify that the new timestamp is close enough to our time
// 2- generates and saves a new threshold partial signature for
//    the new message m_i = H(sig_i-1 || timestamp)
// 3- broadcast that partial signature to the whole group
func (b *Beacon) processBeaconRequest(pub *Public, msg *BeaconRequest) {
	// 1
	now := time.Now()
	leaderTime := time.Unix(msg.Timestamp, 0)
	if now.Sub(leaderTime) > maxTimestampDelta {
		slog.Info("blsbeacon received out-of-range timestamp signature request: ", now.Sub(leaderTime))
		return
	}
	// 2-
	sig := b.genPartialSignature(msg.PreviousSig, msg.Timestamp)
	if !bls.ThresholdVerify(pairing, b.pub, msg.Message(), sig) {
		panic("aie")
	}
	packet := &DrandPacket{
		Beacon: &BeaconPacket{
			Reply: &BeaconReply{
				Request:   msg,
				Signature: sig,
			},
		},
	}
	// 3-
	go func() {
		slog.Debugf("beacon %s: sending REPLY to from %s", b.r.addr, pub.Address)
		if err := b.r.Broadcast(b.group, packet); err != nil {
			slog.Info("blsBeacon error broadcast partial signature: ", err)
		}
	}()
}

// processBeaconSignature does the following:
// 1- checks if the given partial signature is valid.
// 2- check if we already recovered the full signatures (by looking at the
// signature folder)
// 3- Saves it in memory and if there is enough threshold partial signatures for
// the message, it reconstructs the full bls signature and saves it to a file.
func (b *Beacon) processBeaconSignature(pub *Public, sig *BeaconReply) {
	b.Lock()
	defer b.Unlock()
	// 1-
	msg := sig.Request.Message()
	if !bls.ThresholdVerify(pairing, b.pub, msg, sig.Signature) {
		slog.Info("blsBeacon ", b.share.Share.I, "received invalid partial signature from", pub.Address)
		return
	}

	// 2-
	if b.store.SignatureExists(sig.Request.Timestamp) {
		slog.Infof("blsBeacon already reconstructed signature %d", sig.Request.Timestamp)
		return
	}

	d := digest(msg)
	for _, s := range b.pendingSigs[d] {
		if s.Index == sig.Signature.Index {
			slog.Debug("blsbeacon already received partial signature for same message")
			return
		}
	}
	b.pendingSigs[d] = append(b.pendingSigs[d], sig.Signature)

	// 3-
	if len(b.pendingSigs[d]) < b.threshold {
		slog.Debugf("blsBeacon: not enough partial signature yet %d/%d", len(b.pendingSigs[d]), b.threshold)
		return
	}

	slog.Debugf("blsBeacon: full signature recovery for sig: %d", sig.Request.Timestamp)
	fullSig, err := bls.AggregateSignatures(pairing, b.pub, msg, b.pendingSigs[d], len(b.group.Nodes), b.threshold)
	if err != nil {
		slog.Info("blsBeacon: full signature recovery failed for ts %d: %s", sig.Request.Timestamp, err)
		return
	}
	// dispatch to the beacon controller
	if b.newSig != nil {
		b.newSig <- fullSig
	}

	delete(b.pendingSigs, d)

	if b.store.SaveSignature(NewBeaconSignature(sig.Request, fullSig)); err != nil {
		slog.Infof("blsBeacon: error saving signature: %s", err)
		return
	}
	slog.Print("blsBeacon: reconstructed and save full signature for ", sig.Request.Timestamp)
}

func (b *Beacon) Stop() {
	b.Lock()
	defer b.Unlock()
	if b.ticker == nil {
		return
	}
	b.ticker.Stop()
	slog.Info("root beacon STOP.")
}

func (b *Beacon) genPartialSignature(oldSig []byte, time int64) *bls.ThresholdSig {
	newMessage := BeaconRequest{oldSig, time}.Message()
	thresholdSign := bls.ThresholdSign(pairing, b.share.Share, newMessage)
	b.Lock()
	defer b.Unlock()
	digestM := digest(newMessage)
	b.pendingSigs[digestM] = append(b.pendingSigs[digestM], thresholdSign)
	return thresholdSign
}

// XXX use with precautions XXX
func toBytes(p kyber.Point) []byte {
	buff, _ := p.MarshalBinary()
	return buff
}

// digest returns a compact representation of the given message
func digest(msg []byte) string {
	return string(pairing.Hash().Sum(msg))
}
