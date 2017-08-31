package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"gopkg.in/dedis/kyber.v1/share"
	"gopkg.in/dedis/kyber.v1/share/pedersen/dkg"

	"github.com/BurntSushi/toml"
	"github.com/dedis/drand/bls"
	"github.com/nikkolasg/slog"
)

// How much time can a signature timestamp differ from our local time
var maxTimestampDelta = 10 * time.Second

// BeaconSignature is the final reconstructed BLS signature that is saved in the
// filesystem.
type BeaconSignature struct {
	Request   *BeaconRequest
	Signature string
}

// blsBeacon holds the logic to initiate, and react to the TBLS protocol, as
// well as being able to holds the list of full signatures in the filesystem.
type blsBeacon struct {
	r         *Router
	dks       *dkg.DistKeyShare
	group     *Group
	pub       *share.PubPoly
	threshold int
	sync.Mutex

	pendingSigs map[string][]*bls.ThresholdSig

	sigFolder string
}

func newBlsBeacon(dks *dkg.DistKeyShare, r *Router, group *Group, sigFolder string) *blsBeacon {
	return &blsBeacon{
		r:         r,
		group:     group,
		sigFolder: sigFolder,
		dks:       dks,
		pub:       share.NewPubPoly(g2, g2.Point().Base(), dks.Commitments()),
		threshold: len(dks.Commitments()),
	}
}

// processBeaconPacket looks if the packet is a signature request or a signature
// reply and acts accordingly.
func (b *blsBeacon) processBeaconPacket(pub *Public, msg *BeaconPacket) {
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
func (b *blsBeacon) processBeaconRequest(pub *Public, msg *BeaconRequest) {
	// 1
	now := time.Now()
	leaderTime := time.Unix(msg.Timestamp, 0)
	if now.Sub(leaderTime) > maxTimestampDelta {
		slog.Info("blsbeacon received out-of-range timestamp signature request: ", now.Sub(leaderTime))
		return
	}
	// 2-
	newMessage := message(msg.PreviousSig, leaderTime.Unix())
	thresholdSign := bls.ThresholdSign(pairing, b.dks.PriShare(), newMessage)
	b.Lock()
	defer b.Unlock()
	digestM := digest(newMessage)
	b.pendingSigs[digestM] = append(b.pendingSigs[digestM], thresholdSign)
	packet := &DrandPacket{
		Beacon: &BeaconPacket{
			Reply: &BeaconReply{
				Request:   msg,
				Signature: thresholdSign,
			},
		},
	}
	// 3-
	if err := b.r.Broadcast(b.group, packet); err != nil {
		slog.Info("blsBeacon error broadcast partial signature: ", err)
	}
}

// processBeaconSignature does the following:
// 1- checks if the given partial signature is valid.
// 2- check if we already recovered the full signatures (by looking at the
// signature folder)
// 3- Saves it in memory and if there is enough threshold partial signatures for
// the message, it reconstructs the full bls signature and saves it to a file.
func (b *blsBeacon) processBeaconSignature(pub *Public, sig *BeaconReply) {
	b.Lock()
	defer b.Unlock()
	// 1-
	msg := message(sig.Request.PreviousSig, sig.Request.Timestamp)
	if !bls.ThresholdVerify(pairing, b.pub, msg, sig.Signature) {
		slog.Info("blsBeacon received invalid partial signature")
		return
	}

	// 2-
	fname := b.toFilename(sig.Request.Timestamp)
	if fileExists(b.sigFolder, fname) {
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

	slog.Debug("blsBeacon: full signature recovery")
	fullSig, err := bls.AggregateSignatures(pairing, b.pub, msg, b.pendingSigs[d], len(b.group.List), b.threshold)
	if err != nil {
		slog.Info("blsBeacon: full signature recovery failed for ts %d: %s", sig.Request.Timestamp, err)
		return
	}
	delete(b.pendingSigs, d)

	if err := NewBeaconSignature(sig.Request, fullSig).Save(fname); err != nil {
		slog.Infof("blsBeacon: error saving signature %s: %s", fname, err)
		return
	}
	slog.Print("blsBeacon: reconstructed and save full signature to %s", fname)
}

// toFilename returns the filename where a signature having the given timestamp
// is stored.
func (b *blsBeacon) toFilename(ts int64) string {
	return path.Join(b.sigFolder, fmt.Sprintf("%s.sig", ts))
}

func NewBeaconSignature(req *BeaconRequest, signature []byte) *BeaconSignature {
	b64sig := base64.StdEncoding.EncodeToString(signature)
	return &BeaconSignature{
		Request:   req,
		Signature: b64sig,
	}
}

// Save stores the beacon signature into the given filename overwriting any
// previous files if existing.
func (b *BeaconSignature) Save(file string) error {
	fd, err := os.Create(file)
	if err != nil {
		return err
	}
	defer fd.Close()
	return toml.NewEncoder(fd).Encode(b)
}

func (b *BeaconSignature) RawSig() []byte {
	s, err := base64.StdEncoding.DecodeString(b.Signature)
	if err != nil {
		panic("beacon signature have invalid base64 encoded ! File corrupted ? Attack ? God ? Pesto ?")
	}
	return s
}

// message returns the message out of the signature and the timestamp as what
// gets signed during a round of the TBLS protocol.
func message(previousSig []byte, ts int64) []byte {
	var buff bytes.Buffer
	binary.Write(&buff, binary.LittleEndian, ts)
	buff.Write(previousSig)
	return buff.Bytes()
}

// digest returns a compact representation of the given message
func digest(msg []byte) string {
	return string(pairing.Hash().Sum(msg))
}
