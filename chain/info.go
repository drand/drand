package chain

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/kyber"
)

// Info represents the public information that is necessary for a client to
// very any beacon present in a randomness chain.
type Info struct {
	PublicKey   kyber.Point   `json:"public_key"`
	Period      time.Duration `json:"period"`
	GenesisTime int64         `json:"genesis_time"`
	GroupHash   []byte        `json:"group_hash"`
}

// NewChainInfo makes a chain Info from a group
func NewChainInfo(g *key.Group) *Info {
	return &Info{
		Period:      g.Period,
		PublicKey:   g.PublicKey.Key(),
		GenesisTime: g.GenesisTime,
		GroupHash:   g.GetGenesisSeed(),
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
		log.DefaultLogger().Warn("info", "failed to hash pubkey", "err", err)
	}
	_, _ = h.Write(buff)
	_, _ = h.Write(c.GroupHash)
	return h.Sum(nil)
}

// Equal indicates if two Chain Info objects are equivalent
func (c *Info) Equal(c2 *Info) bool {
	return c.GenesisTime == c2.GenesisTime &&
		c.Period == c2.Period &&
		c.PublicKey.Equal(c2.PublicKey) &&
		bytes.Equal(c.GroupHash, c2.GroupHash)
}
