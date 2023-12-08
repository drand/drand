package key

// Group is a list of Public keys providing helper methods to search and

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"sort"
	"time"

	"github.com/BurntSushi/toml"
	"golang.org/x/crypto/blake2b"

	common2 "github.com/drand/drand/common"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/protobuf/common"
	proto "github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share/dkg"
)

// TODO new256 returns an error so we make a wrapper around
var hashFunc = func() hash.Hash { h, _ := blake2b.New256(nil); return h }

// Group holds all information about a group of drand nodes.
type Group struct {
	// Threshold to setup during the DKG or resharing protocol.
	Threshold int
	// Period to use for the beacon randomness generation
	Period time.Duration
	// Scheme indicates a set of values the process will use to act in specific ways
	Scheme *crypto.Scheme
	// ID is the unique identifier for this group
	ID string
	// CatchupPeriod is a delay to insert while in a catchup mode
	// also can be thought of as the minimum period allowed between
	// beacon and subsequent partial generation
	CatchupPeriod time.Duration
	// List of nodes forming this group
	Nodes []*Node
	// Time at which the first round of the chain is mined
	GenesisTime int64
	// Seed of the genesis block. When doing a DKG from scratch, it will be
	// populated directly from the list of nodes and other parameters. WHen
	// doing a resharing, this seed is taken from the first group of the
	// network.
	GenesisSeed []byte
	// In case of a resharing, this is the time at which the network will
	// transition from the old network to the new network.
	TransitionTime int64
	// The distributed public key of this group. It is nil if the group has not
	// ran a DKG protocol yet.
	PublicKey *DistPublic
}

// Find returns the Node that is equal to the given identity (without the
// index). If the node is not found, Find returns nil.
func (g *Group) Find(pub *Identity) *Node {
	for _, pu := range g.Nodes {
		if pu.Identity.Equal(pub) {
			// we have to create a new object to avoid triggering the race detector with the DKG
			// store which also uses the `Node`s from the group file
			return &Node{
				Identity: &Identity{
					Key:       pu.Key,
					Addr:      pu.Addr,
					Signature: pu.Signature,
					Scheme:    g.Scheme,
					TLS:       pu.TLS,
				},
				Index: pu.Index,
			}
		}
	}
	return nil
}

// Node returns the node at the given index if it exists in the group. If it does
// not, Node() returns nil.
func (g *Group) Node(i Index) *Node {
	for _, n := range g.Nodes {
		if n.Index == i {
			return n
		}
	}
	return nil
}

// DKGNodes return the slice of nodes of this group that is consumable by the
// dkg library: only the public key and index are used.
func (g *Group) DKGNodes() []dkg.Node {
	dnodes := make([]dkg.Node, len(g.Nodes))
	for i, node := range g.Nodes {
		dnodes[i] = dkg.Node{
			Index:  node.Index,
			Public: node.Identity.Key,
		}
	}
	return dnodes
}

// Hash provides a compact hash of a group
func (g *Group) Hash() []byte {
	h := hashFunc()

	sort.Slice(g.Nodes, func(i, j int) bool {
		return g.Nodes[i].Index < g.Nodes[j].Index
	})

	// all nodes public keys and positions
	for _, n := range g.Nodes {
		_, _ = h.Write(n.Hash())
	}

	_ = binary.Write(h, binary.LittleEndian, uint32(g.Threshold))
	_ = binary.Write(h, binary.LittleEndian, uint64(g.GenesisTime))

	if g.TransitionTime != 0 {
		_ = binary.Write(h, binary.LittleEndian, g.TransitionTime)
	}

	if g.PublicKey != nil {
		_, _ = h.Write(g.PublicKey.Hash())
	}

	// To keep backward compatibility
	if !common2.IsDefaultBeaconID(g.ID) {
		_, _ = h.Write([]byte(g.ID))
	}

	return h.Sum(nil)
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
	_ = toml.NewEncoder(&b).Encode(g.TOML())
	return b.String()
}

// Equal indicates if two groups are equal
//
//nolint:gocyclo
func (g *Group) Equal(g2 *Group) bool {
	if g == nil {
		return g2 == nil
	}
	if g2 == nil {
		return false
	}
	if !common2.CompareBeaconIDs(g.ID, g2.ID) {
		return false
	}
	if g.Threshold != g2.Threshold {
		return false
	}
	if g.Period.String() != g2.Period.String() {
		return false
	}
	if g.Len() != g2.Len() {
		return false
	}
	if !bytes.Equal(g.GetGenesisSeed(), g2.GetGenesisSeed()) {
		return false
	}
	if g.TransitionTime != g2.TransitionTime {
		return false
	}
	if g.Scheme == nil {
		if g2.Scheme != nil {
			return false
		}
	} else {
		if g2.Scheme == nil || g.Scheme.Name != g2.Scheme.Name {
			return false
		}
	}
	for i := 0; i < g.Len(); i++ {
		if !g.Nodes[i].Equal(g2.Nodes[i]) {
			return false
		}
	}

	if g.PublicKey != nil {
		if g2.PublicKey != nil {
			// both keys aren't nil so we verify
			return g.PublicKey.Equal(g2.PublicKey)
		}
		// g is not nil g2 is nil
		return false
	} else if g2.PublicKey != nil {
		// g is nil g2 is not nil
		return false
	}

	return true
}

// GroupTOML is the representation of a Group TOML compatible
type GroupTOML struct {
	Threshold      int
	Period         string
	CatchupPeriod  string
	Nodes          []*NodeTOML
	GenesisTime    int64
	TransitionTime int64           `toml:",omitempty"`
	GenesisSeed    string          `toml:",omitempty"`
	PublicKey      *DistPublicTOML `toml:",omitempty"`
	SchemeID       string
	ID             string
}

//nolint:gocyclo
func (g *Group) FromTOML(i interface{}) error {
	if i == nil {
		return nil
	}
	gt, ok := i.(*GroupTOML)
	if !ok {
		return fmt.Errorf("grouptoml unknown")
	}
	g.Threshold = gt.Threshold

	// migration path from < v1.4, gt.SchemeID might not be contained in the group file, in which case it's the default
	sch, err := crypto.GetSchemeByID(gt.SchemeID)
	if err != nil {
		return fmt.Errorf("unable to instantiate group with crypto Scheme named %q", gt.SchemeID)
	}
	g.Scheme = sch
	g.Nodes = make([]*Node, len(gt.Nodes))
	for i, ptoml := range gt.Nodes {
		g.Nodes[i] = new(Node)
		if err := g.Nodes[i].FromTOML(ptoml); err != nil {
			return fmt.Errorf("group: unwrapping node[%d]: %w", i, err)
		}
	}

	if g.Threshold < dkg.MinimumT(len(gt.Nodes)) {
		return errors.New("group file has threshold 0")
	} else if g.Threshold > g.Len() {
		return errors.New("group file threshold greater than number of participants")
	}

	if gt.PublicKey != nil {
		// dist key only if dkg ran
		g.PublicKey = new(DistPublic)
		if err = g.PublicKey.FromTOML(sch, gt.PublicKey); err != nil {
			return fmt.Errorf("group: unwrapping distributed public key: %w", err)
		}
	}
	g.Period, err = time.ParseDuration(gt.Period)
	if err != nil {
		return err
	}
	if gt.CatchupPeriod == "" {
		g.CatchupPeriod = 0
	} else {
		g.CatchupPeriod, err = time.ParseDuration(gt.CatchupPeriod)
		if err != nil {
			return err
		}
	}
	g.GenesisTime = gt.GenesisTime
	if gt.TransitionTime != 0 {
		g.TransitionTime = gt.TransitionTime
	}
	if gt.GenesisSeed != "" {
		if g.GenesisSeed, err = hex.DecodeString(gt.GenesisSeed); err != nil {
			return fmt.Errorf("group: decoding genesis seed %w", err)
		}
	}

	// for backward compatibility we make sure to write "default" as beacon id if not set
	g.ID = common2.GetCanonicalBeaconID(gt.ID)

	return nil
}

// TOML returns a TOML-encodable version of the Group
func (g *Group) TOML() interface{} {
	gtoml := &GroupTOML{
		Threshold: g.Threshold,
	}
	gtoml.Nodes = make([]*NodeTOML, g.Len())
	for i, n := range g.Nodes {
		gtoml.Nodes[i] = n.TOML().(*NodeTOML)
	}

	if g.PublicKey != nil {
		gtoml.PublicKey = g.PublicKey.TOML().(*DistPublicTOML)
	}

	gtoml.ID = g.ID
	gtoml.SchemeID = g.Scheme.Name
	gtoml.Period = g.Period.String()
	gtoml.CatchupPeriod = g.CatchupPeriod.String()
	gtoml.GenesisTime = g.GenesisTime
	if g.TransitionTime != 0 {
		gtoml.TransitionTime = g.TransitionTime
	}
	gtoml.GenesisSeed = hex.EncodeToString(g.GetGenesisSeed())
	return gtoml
}

// GetGenesisSeed exposes the hash of the genesis seed for the group
func (g *Group) GetGenesisSeed() []byte {
	if g.GenesisSeed != nil {
		return g.GenesisSeed
	}

	g.GenesisSeed = g.Hash()
	return g.GenesisSeed
}

// TOMLValue returns an empty TOML-compatible value of the group
func (g *Group) TOMLValue() interface{} {
	return &GroupTOML{}
}

// LoadGroup returns a group that contains all information with respect
// to a QUALified set of nodes that ran successfully a setup or reshare phase.
// The threshold is automatically guessed from the length of the distributed
// key.
// Note: only used in tests
func LoadGroup(list []*Node, genesis int64, public *DistPublic, period time.Duration,
	transition int64, sch *crypto.Scheme, beaconID string) *Group {
	return &Group{
		Nodes:          list,
		Threshold:      len(public.Coefficients),
		PublicKey:      public,
		Period:         period,
		CatchupPeriod:  period / 2,
		GenesisTime:    genesis,
		TransitionTime: transition,
		Scheme:         sch,
		ID:             beaconID,
	}
}

// MinimumT calculates the threshold needed for the group to produce sufficient shares to decode
func MinimumT(n int) int {
	return (n >> 1) + 1
}

// GroupFromProto converts a protobuf group into a local Group object
func GroupFromProto(g *proto.GroupPacket, targetScheme *crypto.Scheme) (*Group, error) {
	sch, err := crypto.SchemeFromName(g.GetSchemeID())
	if err != nil {
		return nil, fmt.Errorf("invalid Scheme name in GroupPacket: %s", g.GetSchemeID())
	}
	if targetScheme != nil && targetScheme.Name != sch.Name {
		return nil, fmt.Errorf("mismatch in Scheme name in GroupPacket: %s != %s", targetScheme.Name, sch.Name)
	}

	var nodes = make([]*Node, 0, len(g.GetNodes()))
	for _, pbNode := range g.GetNodes() {
		kid, err := NodeFromProto(pbNode, sch)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, kid)
	}

	n := len(nodes)
	thr := int(g.GetThreshold())

	if thr < MinimumT(n) {
		return nil, fmt.Errorf("invalid threshold: %d vs %d (minimum)", thr, MinimumT(n))
	}

	genesisTime := int64(g.GetGenesisTime())
	if genesisTime == 0 {
		return nil, fmt.Errorf("genesis time zero")
	}

	period := time.Duration(g.GetPeriod()) * time.Second
	if period == time.Duration(0) {
		return nil, fmt.Errorf("period time is zero")
	}

	catchupPeriod := time.Duration(g.GetCatchupPeriod()) * time.Second
	beaconID := g.GetMetadata().GetBeaconID()

	var dist = new(DistPublic)
	for _, coeff := range g.DistKey {
		c := sch.KeyGroup.Point()
		if err := c.UnmarshalBinary(coeff); err != nil {
			return nil, fmt.Errorf("invalid distributed key coefficients:%w", err)
		}
		dist.Coefficients = append(dist.Coefficients, c)
	}

	group := &Group{
		Threshold:      thr,
		Period:         period,
		CatchupPeriod:  catchupPeriod,
		Nodes:          nodes,
		GenesisTime:    genesisTime,
		TransitionTime: int64(g.GetTransitionTime()),
		Scheme:         sch,
		ID:             beaconID,
	}

	if g.GetGenesisSeed() != nil {
		group.GenesisSeed = g.GetGenesisSeed()
	}

	if len(dist.Coefficients) > 0 {
		if len(dist.Coefficients) != group.Threshold {
			return nil, fmt.Errorf("public coefficient length %d is not equal to threshold %d", len(dist.Coefficients), group.Threshold)
		}
		group.PublicKey = dist
	}

	return group, nil
}

// ToProto encodes a local group object into its wire format
func (g *Group) ToProto(version common2.Version) *proto.GroupPacket {
	var out = new(proto.GroupPacket)
	var ids = make([]*proto.Node, len(g.Nodes))

	for i, id := range g.Nodes {
		key, _ := id.Key.MarshalBinary()
		ids[i] = &proto.Node{
			Public: &proto.Identity{
				Address:   id.Address(),
				Key:       key,
				Signature: id.Signature,
				Tls:       id.TLS,
			},
			Index: id.Index,
		}
	}

	out.Nodes = ids
	out.Period = uint32(g.Period.Seconds())
	out.CatchupPeriod = uint32(g.CatchupPeriod.Seconds())
	out.Threshold = uint32(g.Threshold)
	out.GenesisTime = uint64(g.GenesisTime)
	out.TransitionTime = uint64(g.TransitionTime)
	out.GenesisSeed = g.GetGenesisSeed()
	out.SchemeID = g.Scheme.Name

	out.Metadata = common.NewMetadata(version.ToProto())
	out.Metadata.BeaconID = common2.GetCanonicalBeaconID(g.ID)

	if g.PublicKey != nil {
		var coeffs = make([][]byte, len(g.PublicKey.Coefficients))
		for i, c := range g.PublicKey.Coefficients {
			buff, _ := c.MarshalBinary()
			coeffs[i] = buff
		}
		out.DistKey = coeffs
	}
	return out
}

// UnsignedIdentities return true if all identities in the group are signed
// correctly or not. This method is here because of backward compatibility where
// identities were not self-signed before.
func (g *Group) UnsignedIdentities() []*Node {
	var unsigned []*Node
	for _, n := range g.Nodes {
		if n.Identity.ValidSignature() != nil {
			unsigned = append(unsigned, n)
		}
	}
	return unsigned
}
