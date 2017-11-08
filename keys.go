package main

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/dedis/drand/pbc"
	kyber "github.com/dedis/kyber"
	"github.com/dedis/kyber/share"
	"github.com/dedis/kyber/share/pedersen/dkg"
	"github.com/dedis/kyber/util/random"
)

var pairing = pbc.NewPairingFp254BNb()
var g1 = pairing.G1()
var g2 = pairing.G2()

// Private is a wrapper around a random scalar  and the corresponding public
// key in G2
type Private struct {
	Key    kyber.Scalar
	Public *Public
}

// Public holds the corresponding public key of a Private. It also includes a
// valid internet facing ipv4 address where to this reach the node holding the
// public / private key pair.
type Public struct {
	Key     kyber.Point
	Address string
}

// NewKeyPair returns a freshly created private / public key pair.
func NewKeyPair(address string) *Private {
	key := g2.Scalar().Pick(random.Stream)
	pubKey := g2.Point().Mul(key, nil)
	pub := &Public{
		Key:     pubKey,
		Address: address,
	}
	return &Private{
		Key:    key,
		Public: pub,
	}
}

type PrivateTOML struct {
	Key string
}

type PublicTOML struct {
	Address string
	Key     string
}

func (p *Private) TOML() interface{} {
	hexKey := scalarToString(p.Key)
	return &PrivateTOML{hexKey}
}

func (p *Private) FromTOML(i interface{}) error {
	ptoml, ok := i.(*PrivateTOML)
	if !ok {
		return errors.New("private can't decode toml from non PrivateTOML struct")
	}

	buff, err := hex.DecodeString(ptoml.Key)
	if err != nil {
		return err
	}
	p.Key = g2.Scalar()
	if err := p.Key.UnmarshalBinary(buff); err != nil {
		return err
	}
	p.Public = new(Public)
	return nil
}

func (p *Private) TOMLValue() interface{} {
	return &PrivateTOML{}
}

func (p *Public) Equal(p2 *Public) bool {
	return p.Key.Equal(p2.Key) && p.Address == p2.Address
}

// loads reads the TOML description of the public key
func (p *Public) FromTOML(i interface{}) error {
	ptoml, ok := i.(*PublicTOML)
	if !ok {
		return errors.New("Public can't decode from non PublicTOML struct")
	}
	buff, err := hex.DecodeString(ptoml.Key)
	if err != nil {
		return err
	}
	p.Address = ptoml.Address
	p.Key = g2.Point()
	return p.Key.UnmarshalBinary(buff)
}

// Save saves a public key into the given file
func (p *Public) TOML() interface{} {
	hex := pointToString(p.Key)
	return &PublicTOML{p.Address, hex}
}

func (p *Public) TOMLValue() interface{} {
	return &PublicTOML{}
}

type ByKey []*Public

func (b ByKey) Len() int {
	return len(b)
}

func (b ByKey) Swap(i, j int) {
	(b)[i], (b)[j] = (b)[j], (b)[i]
}

func (b ByKey) Less(i, j int) bool {
	is := (b)[i].Key.String()
	js := (b)[j].Key.String()
	return strings.Compare(is, js) < 0
}

// Group is a list of IndexedPublic providing helper methods to search and
// get public keys from a list.
type Group struct {
	Nodes     []*IndexedPublic
	Threshold int
}

// IndexedPublic wraps a Public with its index relative to the group
type IndexedPublic struct {
	*Public
	Index int
}

// Contains returns true if the public key is contained in the list or not.
func (g *Group) Contains(pub *Public) bool {
	for _, pu := range g.Nodes {
		if pu.Equal(pub) {
			return true
		}
	}
	return false
}

// Index returns the index of the given public key with a boolean indicating
// whether the public has been found or not.
func (g *Group) Index(pub *Public) (int, bool) {
	for _, pu := range g.Nodes {
		if pu.Equal(pub) {
			return pu.Index, true
		}
	}
	return 0, false
}

func (g *Group) Public(i int) *Public {
	if i >= g.Len() {
		panic("out of bounds access for Group")
	}
	return g.Nodes[i].Public
}

// Points returns itself under the form of a list of kyber.Point
func (g *Group) Points() []kyber.Point {
	pts := make([]kyber.Point, g.Len())
	for _, pu := range g.Nodes {
		pts[pu.Index] = pu.Key
	}
	return pts
}

// Len returns the number of participants in the group
func (g *Group) Len() int {
	return len(g.Nodes)
}

// GroupTOML is the representation of a Group TOML compatible
type GroupTOML struct {
	Nodes []*PublicTOML
	T     int
}

// Load decodes the group from the toml struct
func (g *Group) FromTOML(i interface{}) error {
	gt, ok := i.(*GroupTOML)
	if !ok {
		return fmt.Errorf("grouptoml unknown")
	}
	g.Threshold = gt.T
	list := make([]*Public, len(gt.Nodes))
	for i, ptoml := range gt.Nodes {
		list[i] = new(Public)
		if err := list[i].FromTOML(ptoml); err != nil {
			return err
		}
	}
	g.Nodes = toIndexedList(list)
	if g.Threshold == 0 {
		return errors.New("group file have threshold 0!")
	} else if g.Threshold > g.Len() {
		return errors.New("group file have threshold superior to number of participants!")
	}
	return nil
}

func (g *Group) TOML() interface{} {
	gtoml := &GroupTOML{T: g.Threshold}
	gtoml.Nodes = make([]*PublicTOML, g.Len())
	for i, p := range g.Nodes {
		key := pointToString(p.Key)
		gtoml.Nodes[i] = &PublicTOML{Key: key, Address: p.Address}
	}
	return gtoml
}

func (g *Group) TOMLValue() interface{} {
	return &GroupTOML{}
}

// returns an indexed list from a list of public keys. Functionality needed in
// tests where one does not necessary load a group from a file.
func toIndexedList(list []*Public) []*IndexedPublic {
	sort.Sort(ByKey(list))
	ilist := make([]*IndexedPublic, len(list))
	for i, p := range list {
		ilist[i] = &IndexedPublic{
			Public: p,
			Index:  i,
		}
	}
	return ilist
}

// Share represents the private information that a node holds after a successful
// DKG. This information MUST stay private !
type Share dkg.DistKeyShare

func (s *Share) Public() *DistPublic {
	return &DistPublic{s.Commits[0]}
}

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

func (s *Share) FromTOML(i interface{}) error {
	t, ok := i.(*ShareTOML)
	if !ok {
		return errors.New("invalid struct received for share")
	}
	s.Commits = make([]kyber.Point, len(t.Commits))
	for i, c := range t.Commits {
		p, err := stringToPoint(g2, c)
		if err != nil {
			return fmt.Errorf("share.Commit[%d] corruputed: %s", i, err)
		}
		s.Commits[i] = p
	}
	sshare, err := stringToScalar(g2, t.Share)
	if err != nil {
		return fmt.Errorf("share.Share corrupted: %s", err)
	}
	s.Share = &share.PriShare{V: sshare, I: t.Index}
	return nil
}

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

type DistPublicTOML struct {
	Key string
}

func (d *DistPublic) TOML() interface{} {
	str := pointToString(d.Key)
	return &DistPublicTOML{str}
}

func (d *DistPublic) FromTOML(i interface{}) error {
	dtoml, ok := i.(*DistPublicTOML)
	if !ok {
		return errors.New("wrong interface: expected DistPublicTOML")
	}
	var err error
	d.Key, err = stringToPoint(g2, dtoml.Key)
	return err
}

func (d *DistPublic) TOMLValue() interface{} {
	return &DistPublicTOML{}
}

// BeaconSignature is the final reconstructed BLS signature that is saved in the
// filesystem.
type BeaconSignature struct {
	Request   *BeaconRequest
	Signature string
}

func NewBeaconSignature(req *BeaconRequest, signature []byte) *BeaconSignature {
	b64sig := base64.StdEncoding.EncodeToString(signature)
	return &BeaconSignature{
		Request:   req,
		Signature: b64sig,
	}
}

// Save stores the beacon signature into the given filename overwriting any
// previous files if existing.
func (b *BeaconSignature) TOML() interface{} {
	return b
}

func (b *BeaconSignature) FromTOML(i interface{}) error {
	bb, ok := i.(*BeaconSignature)
	if !ok {
		return errors.New("beacon signature can't decode: wrong type")
	}
	*b = *bb
	return nil
}

func (b *BeaconSignature) TOMLValue() interface{} {
	return &BeaconSignature{}
}

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
