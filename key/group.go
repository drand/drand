// Group is a list of Public keys providing helper methods to search and
package key

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/dchest/blake2b"
	kyber "github.com/dedis/kyber"
	vss "github.com/dedis/kyber/share/vss/pedersen"
)

// Group holds all information about a group of drand nodes.
type Group struct {
	// List of identities forming this group
	Nodes []*Identity
	// The distributed public key of this group. It is nil if the group has not
	// ran a DKG protocol yet.
	PublicKey *DistPublic
	// Threshold to setup during the DKG or resharing protocol.
	Threshold int
	// Period to use for the beacon randomness generation
	Period time.Duration
}

func (g *Group) Identities() []*Identity {
	return g.Nodes
}

// Contains returns true if the public key is contained in the list or not.
func (g *Group) Contains(pub *Identity) bool {
	for _, pu := range g.Nodes {
		if pu.Equal(pub) {
			return true
		}
	}
	return false
}

// Index returns the index of the given public key with a boolean indicating
// whether the public has been found or not.
func (g *Group) Index(pub *Identity) (int, bool) {
	for i, pu := range g.Nodes {
		if pu.Equal(pub) {
			return i, true
		}
	}
	return -1, false
}

// Public returns the public associated to that index
// or panic otherwise. XXX Change that to return error
func (g *Group) Public(i int) *Identity {
	if i >= g.Len() {
		panic("out of bounds access for Group")
	}
	return g.Nodes[i]
}

// Hash returns an unique short representation of this group.
// NOTE: It currently does NOT take into account the distributed public key when
// set for simplicity (we want old nodes and new nodes to easily refer to the
// same group for example). This may cause trouble in the future and may require
// more thoughts.
func (g *Group) Hash() (string, error) {
	h := blake2b.New256()

	// all nodes public keys and positions
	for i, n := range g.Nodes {
		binary.Write(h, binary.LittleEndian, uint32(i))
		b, err := n.Key.MarshalBinary()
		if err != nil {
			return "", err
		}
		h.Write(b)
	}
	binary.Write(h, binary.LittleEndian, uint32(g.Threshold))
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Points returns itself under the form of a list of kyber.Point
func (g *Group) Points() []kyber.Point {
	pts := make([]kyber.Point, g.Len())
	for i, pu := range g.Nodes {
		pts[i] = pu.Key
	}
	return pts
}

// Len returns the number of participants in the group
func (g *Group) Len() int {
	return len(g.Nodes)
}

func (g *Group) String() string {
	var b bytes.Buffer
	toml.NewEncoder(&b).Encode(g.TOML())
	return b.String()
}

// GroupTOML is the representation of a Group TOML compatible
type GroupTOML struct {
	Nodes     []*PublicTOML
	PublicKey *DistPublicTOML
	Threshold int
	Period    string
}

// FromTOML decodes the group from the toml struct
func (g *Group) FromTOML(i interface{}) (err error) {
	gt, ok := i.(*GroupTOML)
	if !ok {
		return fmt.Errorf("grouptoml unknown")
	}
	g.Threshold = gt.Threshold
	g.Nodes = make([]*Identity, len(gt.Nodes))
	for i, ptoml := range gt.Nodes {
		g.Nodes[i] = new(Identity)
		if err := g.Nodes[i].FromTOML(ptoml); err != nil {
			return err
		}
	}

	if g.Threshold < vss.MinimumT(len(gt.Nodes)) {
		return errors.New("group file have threshold 0")
	} else if g.Threshold > g.Len() {
		return errors.New("group file threshold greater than number of participants")
	}

	if gt.PublicKey != nil {
		// dist key only if dkg ran
		g.PublicKey = &DistPublic{}
		if err = g.PublicKey.FromTOML(gt.PublicKey); err != nil {
			return err
		}
	}
	g.Period, err = time.ParseDuration(gt.Period)
	return err
}

// TOML returns a TOML-encodable version of the Group
func (g *Group) TOML() interface{} {
	gtoml := &GroupTOML{Threshold: g.Threshold}
	gtoml.Nodes = make([]*PublicTOML, g.Len())
	for i, p := range g.Nodes {
		gtoml.Nodes[i] = p.TOML().(*PublicTOML)
	}

	if g.PublicKey != nil {
		gtoml.PublicKey = g.PublicKey.TOML().(*DistPublicTOML)
	}
	gtoml.Period = g.Period.String()
	return gtoml
}

// TOMLValue returns an empty TOML-compatible value of the group
func (g *Group) TOMLValue() interface{} {
	return &GroupTOML{}
}

// MergeGroup returns a NEW group with both list of identities combined,
// the maximum between the default threshold and the group's threshold,
// and with the same period as the group.
func (g *Group) MergeGroup(list []*Identity) *Group {
	thr := DefaultThreshold(len(list) + g.Len())
	if thr < g.Threshold {
		thr = g.Threshold
	}
	return &Group{
		Nodes:     append(g.Identities(), list...),
		Threshold: thr,
		Period:    g.Period,
	}
}

// NewGroup returns a list of identities as a Group.
func NewGroup(list []*Identity, threshold int) *Group {
	return &Group{
		Nodes:     list,
		Threshold: threshold,
	}
}

// LoadGroup returns a group associated with a given public key
func LoadGroup(list []*Identity, public *DistPublic, threshold int) *Group {
	return &Group{
		Nodes:     list,
		Threshold: threshold,
		PublicKey: public,
	}
}
