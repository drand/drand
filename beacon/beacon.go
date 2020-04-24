package beacon

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/kyber"
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

func (b *Beacon) Equal(b2 *Beacon) bool {
	return bytes.Equal(b.PreviousSig, b2.PreviousSig) &&
		b.Round == b2.Round &&
		bytes.Equal(b.Signature, b2.Signature)

}

func (b *Beacon) Marshal() ([]byte, error) {
	return json.Marshal(b)
}

func (b *Beacon) Unmarshal(buff []byte) error {
	return json.Unmarshal(buff, b)
}

// Randomness returns the hashed signature. It is an example that uses sha256,
// but it could use blake2b for example.
func (b *Beacon) Randomness() []byte {
	return RandomnessFromSignature(b.Signature)
}

func (b *Beacon) GetRound() uint64 {
	return b.Round
}

func RandomnessFromSignature(sig []byte) []byte {
	out := sha256.Sum256(sig)
	return out[:]
}

func (b *Beacon) String() string {
	return fmt.Sprintf("{ round: %d, sig: %s, prevSig: %s }", b.Round, shortSigStr(b.Signature), shortSigStr(b.PreviousSig))
}

// VerifyBeacon returns an error if the given beacon does not verify given the
// public key. The public key "point" can be obtained from the
// `key.DistPublic.Key()` method. The distributed public is the one written in
// the configuration file of the network.
func VerifyBeacon(pubkey kyber.Point, b *Beacon) error {
	prevSig := b.PreviousSig
	round := b.Round
	msg := Message(round, prevSig)
	return key.Scheme.VerifyRecovered(pubkey, msg, b.Signature)
}

// Verify is similar to verify beacon but doesn't require to get the full beacon
// structure.
func Verify(pubkey kyber.Point, prevSig, signature []byte, prevRound, round uint64) error {
	return VerifyBeacon(pubkey, &Beacon{
		Round:       round,
		PreviousSig: prevSig,
		Signature:   signature,
	})
}

// Message returns a slice of bytes as the message to sign or to verify
// alongside a beacon signature.
// H ( prevRound || prevSig || currRound)
func Message(currRound uint64, prevSig []byte) []byte {
	var buff bytes.Buffer
	buff.Write(prevSig)
	buff.Write(roundToBytes(currRound))
	h := sha256.New()
	h.Write(buff.Bytes())
	return h.Sum(nil)
}

// TimeOfRound is returning the time the current round should happen
func TimeOfRound(period time.Duration, genesis int64, round uint64) int64 {
	if round == 0 {
		return genesis
	}
	// - 1 because genesis time is for 1st round already
	return genesis + int64((round-1)*uint64(period.Seconds()))
}

func CurrentRound(now int64, period time.Duration, genesis int64) uint64 {
	nextRound, _ := NextRound(now, period, genesis)
	if nextRound <= 1 {
		return nextRound
	}
	return nextRound - 1
}

// NextRound returns the next upcoming round and its UNIX time given the genesis
// time and the period.
// round at time genesis = round 1. Round 0 is fixed.
func NextRound(now int64, period time.Duration, genesis int64) (uint64, int64) {
	if now < genesis {
		return 1, genesis
	}
	fromGenesis := now - genesis
	// we take the time from genesis divided by the periods in seconds, that
	// gives us the number of periods since genesis. We add +1 since we want the
	// next round. We also add +1 because round 1 starts at genesis time.
	nextRound := uint64(math.Floor(float64(fromGenesis)/period.Seconds())) + 1
	nextTime := genesis + int64(nextRound*uint64(period.Seconds()))
	return nextRound + 1, nextTime
}
