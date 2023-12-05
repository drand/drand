package common

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/drand/drand/crypto"
)

// DefaultBeaconID is the value used when beacon id has an empty value. This
// value should not be changed for backward-compatibility reasons
const DefaultBeaconID = "default"

// DefaultChainHash is the value used when chain hash has an empty value on requests
// from clients. This value should not be changed for
// backward-compatibility reasons
const DefaultChainHash = "default"

// MultiBeaconFolder is the name of the folder where the multi-beacon data is stored
const MultiBeaconFolder = "multibeacon"

// LogsToSkip is used to reduce log verbosity when doing bulk processes, issuing logs only every LogsToSkip steps
// this is currently set so that when processing past beacons it will give a log every ~2 seconds
const LogsToSkip = 300

// IsDefaultBeaconID indicates if the beacon id received is the default one or not.
// There is a direct relationship between an empty string and the reserved id "default".
// Internally, empty string is translated to "default" so we can create the beacon folder
// with a valid name.
func IsDefaultBeaconID(beaconID string) bool {
	return beaconID == DefaultBeaconID || beaconID == ""
}

// CompareBeaconIDs indicates if two different beacon ids are equivalent or not.
// It handles default values too.
func CompareBeaconIDs(id1, id2 string) bool {
	if IsDefaultBeaconID(id1) && IsDefaultBeaconID(id2) {
		return true
	}

	if id1 != id2 {
		return false
	}

	return true
}

// GetCanonicalBeaconID returns the correct beacon id.
func GetCanonicalBeaconID(id string) string {
	if IsDefaultBeaconID(id) {
		return DefaultBeaconID
	}
	return id
}

// Beacon holds the randomness as well as the info to verify it.
type Beacon struct {
	// PreviousSig is the previous signature generated
	PreviousSig []byte `json:"previous_signature,omitempty"`
	// Round is the round number this beacon is tied to
	Round uint64 `json:"round,omitempty"`
	// Signature is the BLS deterministic signature as per the crypto.Scheme used
	Signature []byte `json:"signature,omitempty"`
}

// Equal indicates if two beacons are equal
func (b *Beacon) Equal(b2 *Beacon) bool {
	return bytes.Equal(b.PreviousSig, b2.PreviousSig) &&
		b.Round == b2.Round &&
		bytes.Equal(b.Signature, b2.Signature)
}

// Marshal provides a JSON encoding of a beacon. Careful, this is not the one rendered by the public endpoints.
func (b *Beacon) Marshal() ([]byte, error) {
	return json.Marshal(b)
}

// Unmarshal decodes a beacon from JSON
func (b *Beacon) Unmarshal(buff []byte) error {
	return json.Unmarshal(buff, b)
}

// Randomness returns the hashed signature. The choice of the hash determines the size of the output.
func (b *Beacon) Randomness() []byte {
	return crypto.RandomnessFromSignature(b.Signature)
}

func (b *Beacon) GetRandomness() []byte {
	return b.Randomness()
}

// GetPreviousSignature returns the previous signature if it's non-nil or nil otherwise
func (b *Beacon) GetPreviousSignature() []byte {
	return b.PreviousSig
}

// GetSignature returns the signature if it's non-nil or nil otherwise
func (b *Beacon) GetSignature() []byte {
	return b.Signature
}

// GetRound provides the round of the beacon
func (b *Beacon) GetRound() uint64 {
	return b.Round
}

func (b *Beacon) String() string {
	return fmt.Sprintf("{ round: %d, sig: %s, prevSig: %s }", b.Round, shortSigStr(b.Signature), shortSigStr(b.PreviousSig))
}

func shortSigStr(sig []byte) string {
	if sig == nil {
		return "nil"
	}
	if len(sig) == 0 {
		return ""
	}

	max := 3
	if len(sig) < max {
		max = len(sig)
	}
	return hex.EncodeToString(sig[0:max])
}
