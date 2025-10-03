package chain

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/kyber"
)

// Info represents the public information that is necessary for a client to
// verify any beacon present in a randomness chain.
type Info struct {
	PublicKey   kyber.Point   `json:"public_key"`
	ID          string        `json:"id"`
	Period      time.Duration `json:"period"`
	Scheme      string        `json:"scheme"`
	GenesisTime int64         `json:"genesis_time"`
	GenesisSeed []byte        `json:"group_hash"`
	// CatchupPeriod is the node sleep delay during catchup mode
	CatchupPeriod time.Duration `json:"-"`
}

// NewChainInfo makes a chain Info from a group.
func NewChainInfo(g *key.Group) *Info {
	return &Info{
		ID:            g.ID,
		Period:        g.Period,
		Scheme:        g.Scheme.Name,
		PublicKey:     g.PublicKey.Key(),
		GenesisTime:   g.GenesisTime,
		GenesisSeed:   g.GetGenesisSeed(),
		CatchupPeriod: g.CatchupPeriod,
	}
}

// Hash returns the canonical hash representing the chain information. A hash is
// consistent throughout the entirety of a chain, regardless of the network
// composition, the actual nodes, generating the randomness.
func (i *Info) Hash() []byte {
	h := sha256.New()
	_ = binary.Write(h, binary.BigEndian, uint32(i.Period.Seconds()))
	_ = binary.Write(h, binary.BigEndian, i.GenesisTime)

	buff, err := i.PublicKey.MarshalBinary()
	if err != nil {
		log.DefaultLogger().Errorw("chain info: failed to hash pubkey", "err", err)
	}

	_, _ = h.Write(buff)
	_, _ = h.Write(i.GenesisSeed)

	// To keep backward compatibility
	if !common.IsDefaultBeaconID(i.ID) {
		_, _ = h.Write([]byte(i.ID))
	}

	return h.Sum(nil)
}

// HashString returns the value of Hash in string format
func (i *Info) HashString() string {
	return hex.EncodeToString(i.Hash())
}

// Equal indicates if two Chain Info objects are equivalent
func (i *Info) Equal(i2 *Info) bool {
	return i.GenesisTime == i2.GenesisTime &&
		i.Period == i2.Period &&
		i.PublicKey.Equal(i2.PublicKey) &&
		bytes.Equal(i.GenesisSeed, i2.GenesisSeed) &&
		common.CompareBeaconIDs(i.ID, i2.ID) &&
		i.Scheme == i2.Scheme
}

// GetSchemeName returns the scheme name used
func (i *Info) GetSchemeName() string {
	return i.Scheme
}

// UnmarshalJSON implements the json Unmarshaler interface for Info
func (i *Info) UnmarshalJSON(data []byte) error {
	var v2Str struct {
		PublicKey    common.HexBytes `json:"public_key"`
		ID           string          `json:"beacon_id"`
		Period       uint64          `json:"period"`
		Scheme       string          `json:"scheme"`
		GenesisTime  int64           `json:"genesis_time"`
		GenesisSeed  common.HexBytes `json:"genesis_seed"`
		ChainHash    string          `json:"chain_hash"`
		OldSchemeID  string          `json:"schemeID"`
		OldGroupHash common.HexBytes `json:"groupHash"`
		OldMetadata  *struct {
			OldBeaconID string `json:"beaconID"`
		} `json:"metadata"`
	}

	err := json.Unmarshal(data, &v2Str)
	if err != nil {
		return fmt.Errorf("not a v2 info string: %w", err)
	}

	i.GenesisSeed = v2Str.GenesisSeed
	i.GenesisTime = v2Str.GenesisTime
	i.Scheme = v2Str.Scheme
	i.Period = time.Duration(v2Str.Period) * time.Second
	i.ID = v2Str.ID

	// support old scheme name
	if v2Str.OldSchemeID != "" && i.Scheme == "" {
		i.Scheme = v2Str.OldSchemeID
		i.GenesisSeed = v2Str.OldGroupHash
		if v2Str.OldMetadata != nil && v2Str.OldMetadata.OldBeaconID != "" {
			i.ID = v2Str.OldMetadata.OldBeaconID
		}
	}

	sch, err := crypto.GetSchemeByID(i.Scheme)
	if err != nil {
		return fmt.Errorf("invalid scheme advertised: %w", err)
	}
	pk := sch.KeyGroup.Point()
	err = pk.UnmarshalBinary(v2Str.PublicKey)
	if err != nil {
		return fmt.Errorf("invalid public key %q: %w", sch.Name, err)
	}
	i.PublicKey = pk

	if v2Str.ChainHash != "" {
		if i.HashString() != v2Str.ChainHash {
			return fmt.Errorf("chain hash mismatch: %s != %s", i.HashString(), v2Str.ChainHash)
		}
	}

	return nil
}

// MarshalJSON implements the json Marshaler interface for Info, we don't use a pointer receiver
// because otherwise it wouldn't properly marshal plain Info objects.
//
//nolint:gocritic
func (i Info) MarshalJSON() ([]byte, error) {
	var v2Str struct {
		PublicKey   string          `json:"public_key"`
		ID          string          `json:"beacon_id"`
		Period      uint64          `json:"period"`
		Scheme      string          `json:"scheme"`
		GenesisTime int64           `json:"genesis_time"`
		GenesisSeed common.HexBytes `json:"genesis_seed"`
		ChainHash   string          `json:"chain_hash"`
	}

	v2Str.ID = i.ID
	v2Str.Scheme = i.Scheme
	v2Str.Period = uint64(i.Period.Seconds())
	v2Str.GenesisSeed = i.GenesisSeed
	v2Str.GenesisTime = i.GenesisTime
	v2Str.ChainHash = i.HashString()

	rawPk, err := i.PublicKey.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to marshal public key: %w", err)
	}
	v2Str.PublicKey = hex.EncodeToString(rawPk)

	return json.Marshal(v2Str)
}
