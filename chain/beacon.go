package chain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	json "github.com/nikkolasg/hexjson"

	"github.com/drand/drand/key"
	"github.com/drand/kyber"
)

// Beacon holds the randomness as well as the info to verify it.
type Beacon struct {
	// PreviousSig is the previous signature generated
	PreviousSig []byte
	// Round is the round number this beacon is tied to
	Round uint64
	// Signature is the BLS deterministic signature
	Signature []byte
	// SignatureV2 is the signature over simply the Round
	SignatureV2 []byte `json:",omitempty"`
}

// Equal indicates if two beacons are equal
func (b *Beacon) Equal(b2 *Beacon) bool {
	return bytes.Equal(b.PreviousSig, b2.PreviousSig) &&
		b.Round == b2.Round &&
		bytes.Equal(b.Signature, b2.Signature) &&
		bytes.Equal(b.SignatureV2, b2.SignatureV2)
}

// Marshal provides a JSON encoding of a beacon
func (b *Beacon) Marshal() ([]byte, error) {
	return json.Marshal(b)
}

// Unmarshal decodes a beacon from JSON
func (b *Beacon) Unmarshal(buff []byte) error {
	return json.Unmarshal(buff, b)
}

// IsV2 returns true if the beacon contains a v2 signature
func (b *Beacon) IsV2() bool {
	return len(b.SignatureV2) > 0
}

// Randomness returns the hashed signature. It is an example that uses sha256,
// but it could use blake2b for example.
func (b *Beacon) Randomness() []byte {
	return RandomnessFromSignature(b.Signature)
}

func (b *Beacon) RandomnessV2() []byte {
	return RandomnessFromSignatureV2(b.SignatureV2)
}

// GetRound provides the round of the beacon
func (b *Beacon) GetRound() uint64 {
	return b.Round
}

// RandomnessFromSignature derives the round randomness from its signature
func RandomnessFromSignature(sig []byte) []byte {
	out := sha256.Sum256(sig)
	return out[:]
}

// RandomnessFromSignature derives the round randomness from its signatureV2
func RandomnessFromSignatureV2(sig []byte) []byte {
	out := sha256.Sum256(sig)
	return out[:]
}

func (b *Beacon) String() string {
	return fmt.Sprintf("{ round: %d, sig: %s, sig2: %s, prevSig: %s }", b.Round, shortSigStr(b.Signature), shortSigStr(b.SignatureV2), shortSigStr(b.PreviousSig))
}

// VerifyBeacon returns an error if the given beacon does not verify given the
// public key. The public key "point" can be obtained from the
// `key.DistPublic.Key()` method. The distributed public is the one written in
// the configuration file of the network.
// NOTE this method is meant to be deprecated in the future - if available,
// always prefer to use signature v2 and VerifyBeaconV2
func VerifyBeacon(pubkey kyber.Point, b *Beacon) error {
	prevSig := b.PreviousSig
	round := b.Round
	msg := Message(round, prevSig)
	return key.Scheme.VerifyRecovered(pubkey, msg, b.Signature)
}

func VerifyBeaconV2(pubkey kyber.Point, b *Beacon) error {
	round := b.Round
	msg := MessageV2(round)
	return key.Scheme.VerifyRecovered(pubkey, msg, b.SignatureV2)
}

// Message returns a slice of bytes as the message to sign or to verify
// alongside a beacon signature.
// H ( prevSig || currRound)
func Message(currRound uint64, prevSig []byte) []byte {
	h := sha256.New()
	_, _ = h.Write(prevSig)
	_, _ = h.Write(RoundToBytes(currRound))
	return h.Sum(nil)
}

func MessageV2(currRound uint64) []byte {
	h := sha256.New()
	_, _ = h.Write(RoundToBytes(currRound))
	return h.Sum(nil)
}

func shortSigStr(sig []byte) string {
	max := 3
	if len(sig) < max {
		max = len(sig)
	}
	return hex.EncodeToString(sig[0:max])
}
