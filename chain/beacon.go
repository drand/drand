package chain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	json "github.com/nikkolasg/hexjson"
)

// Beacon holds the randomness as well as the info to verify it.
type Beacon struct {
	// PreviousSig is the previous signature generated
	PreviousSig []byte
	// Round is the round number this beacon is tied to
	Round uint64
	// Signature is the BLS deterministic signature over Round || PreviousRand
	Signature []byte
}

// Equal indicates if two beacons are equal
func (b *Beacon) Equal(b2 *Beacon) bool {
	return bytes.Equal(b.PreviousSig, b2.PreviousSig) &&
		b.Round == b2.Round &&
		bytes.Equal(b.Signature, b2.Signature)
}

// Marshal provides a JSON encoding of a beacon
func (b *Beacon) Marshal() ([]byte, error) {
	return json.Marshal(b)
}

// Unmarshal decodes a beacon from JSON
func (b *Beacon) Unmarshal(buff []byte) error {
	return json.Unmarshal(buff, b)
}

// Randomness returns the hashed signature. It is an example that uses sha256,
// but it could use blake2b for example.
func (b *Beacon) Randomness() []byte {
	return RandomnessFromSignature(b.Signature)
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

func (b *Beacon) String() string {
	return fmt.Sprintf("{ round: %d, sig: %s, prevSig: %s }", b.Round, shortSigStr(b.Signature), shortSigStr(b.PreviousSig))
}

func shortSigStr(sig []byte) string {
	max := 3
	if len(sig) < max {
		max = len(sig)
	}
	return hex.EncodeToString(sig[0:max])
}
