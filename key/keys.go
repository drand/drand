package key

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	kyber "github.com/dedis/kyber"
	"github.com/dedis/kyber/pairing/bn256"
	"github.com/dedis/kyber/share"
	"github.com/dedis/kyber/share/dkg/pedersen"
	"github.com/dedis/kyber/util/random"
)

var Pairing = bn256.NewSuite()
var G1 = Pairing.G1()
var G2 = Pairing.G2()

// Private is a wrapper around a random scalar  and the corresponding public
// key in G2
type Private struct {
	Key    kyber.Scalar
	Public *Identity
}

// Identity holds the corresponding public key of a Private. It also includes a
// valid internet facing ipv4 address where to this reach the node holding the
// public / private key pair.
type Identity struct {
	Key  kyber.Point
	Addr string
}

// Address implements the net.Peer interface
func (i *Identity) Address() string {
	return i.Addr
}

// NewKeyPair returns a freshly created private / public key pair. The group is
// decided by the group variable by default. Currently, drand only supports
// bn256.
func NewKeyPair(address string) *Private {
	key := G2.Scalar().Pick(random.New())
	pubKey := G2.Point().Mul(key, nil)
	pub := &Identity{
		Key:  pubKey,
		Addr: address,
	}
	return &Private{
		Key:    key,
		Public: pub,
	}
}

// PrivateTOML is the TOML-able version of a private key
type PrivateTOML struct {
	Key string
}

// PublicTOML is the TOML-able version of a public key
type PublicTOML struct {
	Address string
	Key     string
}

// TOML returns a struct that can be marshalled using a TOML-encoding library
func (p *Private) TOML() interface{} {
	hexKey := scalarToString(p.Key)
	return &PrivateTOML{hexKey}
}

// FromTOML constructs the private key from an unmarshalled structure from TOML
func (p *Private) FromTOML(i interface{}) error {
	ptoml, ok := i.(*PrivateTOML)
	if !ok {
		return errors.New("private can't decode toml from non PrivateTOML struct")
	}

	buff, err := hex.DecodeString(ptoml.Key)
	if err != nil {
		return err
	}
	p.Key = G2.Scalar()
	if err := p.Key.UnmarshalBinary(buff); err != nil {
		return err
	}
	p.Public = new(Identity)
	return nil
}

// TOMLValue returns an empty TOML-compatible interface value
func (p *Private) TOMLValue() interface{} {
	return &PrivateTOML{}
}

// Equal returns true if the cryptographic public key of p equals p2's
func (p *Identity) Equal(p2 *Identity) bool {
	return p.Key.Equal(p2.Key)
}

// FromTOML loads reads the TOML description of the public key
func (p *Identity) FromTOML(i interface{}) error {
	ptoml, ok := i.(*PublicTOML)
	if !ok {
		return errors.New("Public can't decode from non PublicTOML struct")
	}
	buff, err := hex.DecodeString(ptoml.Key)
	if err != nil {
		return err
	}
	p.Addr = ptoml.Address
	p.Key = G2.Point()
	return p.Key.UnmarshalBinary(buff)
}

// TOML returns a empty TOML-compatible version of the public key
func (p *Identity) TOML() interface{} {
	hex := pointToString(p.Key)
	return &PublicTOML{
		Address: p.Addr,
		Key:     hex,
	}
}

// TOMLValue returns a TOML-compatible interface value
func (p *Identity) TOMLValue() interface{} {
	return &PublicTOML{}
}

// ByKey is simply an interface to sort lexig
type ByKey []*Identity

func (b ByKey) Len() int {
	return len(b)
}

func (b ByKey) Swap(i, j int) {
	(b)[i], (b)[j] = (b)[j], (b)[i]
}

func (b ByKey) Less(i, j int) bool {
	is, _ := (b)[i].Key.MarshalBinary()
	js, _ := (b)[j].Key.MarshalBinary()
	return bytes.Compare(is, js) < 0
}

// Group is a list of IndexedPublic providing helper methods to search and
// get public keys from a list.
type Group struct {
	Nodes     []*IndexedPublic
	Threshold int
}

// IndexedPublic wraps a Public with its index relative to the group
type IndexedPublic struct {
	*Identity
	Index int
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
	for _, pu := range g.Nodes {
		if pu.Equal(pub) {
			return pu.Index, true
		}
	}
	return 0, false
}

// Public returns the public associated to that index
// or panic otherwise. XXX Change that to return error
func (g *Group) Public(i int) *Identity {
	if i >= g.Len() {
		panic("out of bounds access for Group")
	}
	return g.Nodes[i].Identity
}

// Points returns itself under the form of a list of kyber.Point
func (g *Group) Points() []kyber.Point {
	pts := make([]kyber.Point, g.Len())
	for _, pu := range g.Nodes {
		pts[pu.Index] = pu.Key
	}
	return pts
}

func (g *Group) Identities() []*Identity {
	ids := make([]*Identity, g.Len(), g.Len())
	for i := range g.Nodes {
		ids[i] = g.Nodes[i].Identity
	}
	return ids
}

// Len returns the number of participants in the group
func (g *Group) Len() int {
	return len(g.Nodes)
}

func (g *Group) Filter(indexes []int) *Group {
	var filtered []*IndexedPublic
	for idx := range indexes {
		filtered = append(filtered, &IndexedPublic{Identity: g.Public(idx), Index: idx})
	}
	return &Group{
		Threshold: g.Threshold,
		Nodes:     filtered,
	}
}

// GroupTOML is the representation of a Group TOML compatible
type GroupTOML struct {
	Nodes []*PublicTOML
	T     int
}

// FromTOML decodes the group from the toml struct
func (g *Group) FromTOML(i interface{}) error {
	gt, ok := i.(*GroupTOML)
	if !ok {
		return fmt.Errorf("grouptoml unknown")
	}
	g.Threshold = gt.T
	list := make([]*Identity, len(gt.Nodes))
	for i, ptoml := range gt.Nodes {
		list[i] = new(Identity)
		if err := list[i].FromTOML(ptoml); err != nil {
			return err
		}
	}
	g.Nodes = toIndexedList(list)
	if g.Threshold == 0 {
		return errors.New("group file have threshold 0")
	} else if g.Threshold > g.Len() {
		return errors.New("group file have threshold superior to number of participants")
	}
	return nil
}

// TOML returns a TOML-encodable version of the Group
func (g *Group) TOML() interface{} {
	gtoml := &GroupTOML{T: g.Threshold}
	gtoml.Nodes = make([]*PublicTOML, g.Len())
	for i, p := range g.Nodes {
		key := pointToString(p.Key)
		gtoml.Nodes[i] = &PublicTOML{Key: key, Address: p.Addr}
	}
	return gtoml
}

// TOMLValue returns an empty TOML-compatible value of the group
func (g *Group) TOMLValue() interface{} {
	return &GroupTOML{}
}

// NewGroup returns a list of identities as a Group. The threshold is set to a
// the default returned by DefaultThreshod.
func NewGroup(list []*Identity, threshold int) *Group {
	return &Group{
		Nodes:     toIndexedList(list),
		Threshold: threshold,
	}
}

// returns an indexed list from a list of public keys. Functionality needed in
// tests where one does not necessary load a group from a file.
func toIndexedList(list []*Identity) []*IndexedPublic {
	sort.Sort(ByKey(list))
	ilist := make([]*IndexedPublic, len(list))
	for i, p := range list {
		ilist[i] = &IndexedPublic{
			Identity: p,
			Index:    i,
		}
	}
	return ilist
}

// Share represents the private information that a node holds after a successful
// DKG. This information MUST stay private !
type Share dkg.DistKeyShare

// Public returns the distributed public key associated with the distributed key
// share
func (s *Share) Public() *DistPublic {
	return &DistPublic{s.Commits[0]}
}

// TOML returns a TOML-compatible version of this share
func (s *Share) TOML() interface{} {
	dtoml := &ShareTOML{}
	dtoml.Commits = make([]string, len(s.Commits))
	for i, c := range s.Commits {
		dtoml.Commits[i] = pointToString(c)
	}
	dtoml.Share = scalarToString(s.Share.V)
	dtoml.Index = s.Share.I
	return dtoml
}

// FromTOML initializes the share from the given TOML-compatible share interface
func (s *Share) FromTOML(i interface{}) error {
	t, ok := i.(*ShareTOML)
	if !ok {
		return errors.New("invalid struct received for share")
	}
	s.Commits = make([]kyber.Point, len(t.Commits))
	for i, c := range t.Commits {
		p, err := stringToPoint(G2, c)
		if err != nil {
			return fmt.Errorf("share.Commit[%d] corruputed: %s", i, err)
		}
		s.Commits[i] = p
	}
	sshare, err := stringToScalar(G2, t.Share)
	if err != nil {
		return fmt.Errorf("share.Share corrupted: %s", err)
	}
	s.Share = &share.PriShare{V: sshare, I: t.Index}
	return nil
}

// TOMLValue returns an empty TOML compatible interface of that Share
func (s *Share) TOMLValue() interface{} {
	return &ShareTOML{}
}

// ShareTOML is the TOML representation of a dkg.DistKeyShare
type ShareTOML struct {
	Commits []string
	Share   string
	Index   int
}

// DistPublic represents the distributed public key generated during a DKG. This
// is the information that can be safely exported to end users verifying a
// drand signature.
// The public key belongs in the same group as the individual public key,i.e. G2
type DistPublic struct {
	Key kyber.Point
}

// DistPublicTOML is a TOML compatible value of a DistPublic
type DistPublicTOML struct {
	Key string
}

// TOML returns a TOML-compatible version of d
func (d *DistPublic) TOML() interface{} {
	str := pointToString(d.Key)
	return &DistPublicTOML{str}
}

// FromTOML initializes d from the TOML-compatible version of a DistPublic
func (d *DistPublic) FromTOML(i interface{}) error {
	dtoml, ok := i.(*DistPublicTOML)
	if !ok {
		return errors.New("wrong interface: expected DistPublicTOML")
	}
	var err error
	d.Key, err = stringToPoint(G2, dtoml.Key)
	return err
}

// TOMLValue returns an empty TOML-compatible dist public interface
func (d *DistPublic) TOMLValue() interface{} {
	return &DistPublicTOML{}
}

// BeaconSignature is the final reconstructed BLS signature that is saved in the
// filesystem.
type BeaconSignature struct {
	Timestamp   int64
	PreviousSig string
	Signature   string
}

// NewBeaconSignature initializes a beacon signature from
// - a timestamp
// - a previous sig. Can be nil if there is no previous signature
// - a signature of the timestamp and the previous sig
func NewBeaconSignature(timestamp int64, previousSig, signature []byte) *BeaconSignature {
	hexSig := hex.EncodeToString(signature)
	hexPrev := hex.EncodeToString(previousSig)
	return &BeaconSignature{
		Timestamp:   timestamp,
		PreviousSig: hexPrev,
		Signature:   hexSig,
	}
}

// TOML returns a TOML-compatible version of this beacon signature
func (b *BeaconSignature) TOML() interface{} {
	return b
}

// FromTOML initializes b from a TOML-compatible version of a beacon signature
func (b *BeaconSignature) FromTOML(i interface{}) error {
	bb, ok := i.(*BeaconSignature)
	if !ok {
		return errors.New("beacon signature can't decode: wrong type")
	}
	*b = *bb
	return nil
}

// TOMLValue returns an empty TOML-compatible version of a beacon signature
func (b *BeaconSignature) TOMLValue() interface{} {
	return &BeaconSignature{}
}

// RawSig returns the signature
func (b *BeaconSignature) RawSig() []byte {
	s, err := base64.StdEncoding.DecodeString(b.Signature)
	if err != nil {
		panic("beacon signature have invalid base64 encoded ! File corrupted ? Attack ? God ? Pesto ?")
	}
	return s
}

func pointToString(p kyber.Point) string {
	buff, _ := p.MarshalBinary()
	return hex.EncodeToString(buff)
}

func scalarToString(s kyber.Scalar) string {
	buff, _ := s.MarshalBinary()
	return hex.EncodeToString(buff)
}

func stringToPoint(g kyber.Group, s string) (kyber.Point, error) {
	buff, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	p := g.Point()
	return p, p.UnmarshalBinary(buff)
}

func stringToScalar(g kyber.Group, s string) (kyber.Scalar, error) {
	buff, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	sc := g.Scalar()
	return sc, sc.UnmarshalBinary(buff)
}

func DefaultThreshold(n int) int {
	return (n*2)/3 + 1
}
