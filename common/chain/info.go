package chain

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"github.com/drand/drand/crypto"
	"os"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/drand/drand/common"
	"github.com/drand/drand/common/key"
	"github.com/drand/drand/common/log"
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
	log         log.Logger
}

// NewChainInfo makes a chain Info from a group.
func NewChainInfo(l log.Logger, g *key.Group) *Info {
	schemeName := g.Scheme.Name
	if sch, err := crypto.GetSchemeByIDWithDefault(schemeName); err == nil {
		// if there is an error we keep the provided name, otherwise we set it
		schemeName = sch.Name
	}
	return &Info{
		ID:          g.ID,
		Period:      g.Period,
		Scheme:      schemeName,
		PublicKey:   g.PublicKey.Key(),
		GenesisTime: g.GenesisTime,
		GenesisSeed: g.GetGenesisSeed(),
		log:         l,
	}
}

// Hash returns the canonical hash representing the chain information. A hash is
// consistent throughout the entirety of a chain, regardless of the network
// composition, the actual nodes, generating the randomness.
func (c *Info) Hash() []byte {
	h := sha256.New()
	_ = binary.Write(h, binary.BigEndian, uint32(c.Period.Seconds()))
	_ = binary.Write(h, binary.BigEndian, c.GenesisTime)

	buff, err := c.PublicKey.MarshalBinary()
	if err != nil {
		c.log.Warnw("", "info", "failed to hash pubkey", "err", err)
	}

	_, _ = h.Write(buff)
	_, _ = h.Write(c.GenesisSeed)

	// To keep backward compatibility
	if !common.IsDefaultBeaconID(c.ID) {
		_, _ = h.Write([]byte(c.ID))
	}

	return h.Sum(nil)
}

// HashString returns the value of Hash in string format
func (c *Info) HashString() string {
	return hex.EncodeToString(c.Hash())
}

// Equal indicates if two Chain Info objects are equivalent
func (c *Info) Equal(c2 *Info) bool {
	return c.GenesisTime == c2.GenesisTime &&
		c.Period == c2.Period &&
		c.PublicKey.Equal(c2.PublicKey) &&
		bytes.Equal(c.GenesisSeed, c2.GenesisSeed) &&
		common.CompareBeaconIDs(c.ID, c2.ID)
}

// GetSchemeName returns the scheme name used
func (c *Info) GetSchemeName() string {
	return c.Scheme
}

// InfoFromGroupTOML reads a drand group TOML file and returns the chain info.
func InfoFromGroupTOML(l log.Logger, filePath string) (*Info, error) {
	gt := &key.GroupTOML{}
	_, err := toml.DecodeFile(filePath, gt)
	if err != nil {
		return nil, err
	}
	g := &key.Group{}
	err = g.FromTOML(gt)
	if err != nil {
		return nil, err
	}
	return NewChainInfo(l, g), nil
}

func InfoFromChainInfoJSON(filePath string) (*Info, error) {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return InfoFromJSON(bytes.NewBuffer(b))
}
