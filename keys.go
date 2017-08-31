package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/dedis/drand/pbc"
	kyber "gopkg.in/dedis/kyber.v1"
	"gopkg.in/dedis/kyber.v1/share/pedersen/dkg"
	"gopkg.in/dedis/kyber.v1/util/random"
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

func (p *Private) Save(file string) error {
	hexKey := scalarToString(p.Key)
	fd, err := createSecureFile(file)
	if err != nil {
		return err
	}
	defer fd.Close()
	if err := toml.NewEncoder(fd).Encode(&PrivateTOML{hexKey}); err != nil {
		return err
	}
	return p.Public.Save(file)
}

func (p *Private) Load(file string) error {
	ptoml := &PrivateTOML{}
	if _, err := toml.DecodeFile(file, ptoml); err != nil {
		return err
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
	return p.Public.Load(publicFile(file))
}

func (p *Public) Equal(p2 *Public) bool {
	return p.Key.Equal(p2.Key) && p.Address == p2.Address
}

// Load reads the TOML description of the public key written in the given file.
func (p *Public) Load(file string) error {
	ptoml := &PublicTOML{}
	if _, err := toml.DecodeFile(file, ptoml); err != nil {
		return err
	}
	pub, err := ptoml.Public()
	(*p) = (*pub)
	return err
}

// Save saves a public key into the given file
func (p *Public) Save(prefix string) error {
	hex := pointToString(p.Key)
	fd, err := os.Create(publicFile(prefix))
	if err != nil {
		return err
	}
	defer fd.Close()
	return toml.NewEncoder(fd).Encode(&PublicTOML{p.Address, hex})
}

// Public returns the Public struct from the TOML representation.
func (p *PublicTOML) Public() (*Public, error) {
	buff, err := hex.DecodeString(p.Key)
	if err != nil {
		return nil, err
	}
	pub := &Public{}
	pub.Address = p.Address
	pub.Key = g2.Point()
	return pub, pub.Key.UnmarshalBinary(buff)
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
	List      []*IndexedPublic
	Threshold int
}

// IndexedPublic wraps a Public with its index relative to the group
type IndexedPublic struct {
	*Public
	Index int
}

// Contains returns true if the public key is contained in the list or not.
func (g *Group) Contains(pub *Public) bool {
	for _, pu := range g.List {
		if pu.Equal(pub) {
			return true
		}
	}
	return false
}

// Index returns the index of the given public key with a boolean indicating
// whether the public has been found or not.
func (g *Group) Index(pub *Public) (int, bool) {
	for _, pu := range g.List {
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
	return g.List[i].Public
}

// Points returns itself under the form of a list of kyber.Point
func (g *Group) Points() []kyber.Point {
	pts := make([]kyber.Point, g.Len())
	for _, pu := range g.List {
		pts[pu.Index] = pu.Key
	}
	return pts
}

// Len returns the number of participants in the group
func (g *Group) Len() int {
	return len(g.List)
}

// GroupTOML is the representation of a Group TOML compatible
type GroupTOML struct {
	List []*PublicTOML
	T    int
}

// Load decodes the group pointed by the filename given in arguments
func (g *Group) Load(file string) error {
	gt := &GroupTOML{}
	if _, err := toml.DecodeFile(file, gt); err != nil {
		return err
	}
	g.Threshold = gt.T
	list := make([]*Public, len(gt.List))
	var err error
	for i, ptoml := range gt.List {
		if list[i], err = ptoml.Public(); err != nil {
			return err
		}
	}
	g.List = toIndexedList(list)
	if g.Threshold == 0 {
		return errors.New("group file have threshold 0!")
	} else if g.Threshold > g.Len() {
		return errors.New("group file have threshold superior to number of participants!")
	}
	return nil
}

func (g *Group) Save(file string) error {
	gtoml := &GroupTOML{T: g.Threshold}
	gtoml.List = make([]*PublicTOML, g.Len())
	for i, p := range g.List {
		key := pointToString(p.Key)
		gtoml.List[i] = &PublicTOML{Key: key, Address: p.Address}
	}
	fd, err := os.Create(file)
	if err != nil {
		return err
	}
	defer fd.Close()
	return toml.NewEncoder(fd).Encode(gtoml)
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
		fmt.Printf("Public index %d -> %s -> %s\n", i, p.Address, p.Key.String()[:15])
	}
	return ilist
}

// DKSToml is the TOML representation of a dkg.DistKeyShare
type DKSToml struct {
	Commits []string
	Share   string
	Index   int
}

func SaveShare(d *dkg.DistKeyShare, file string) error {
	dtoml := &DKSToml{}
	dtoml.Commits = make([]string, len(d.Commits))
	for i, c := range d.Commits {
		dtoml.Commits[i] = pointToString(c)
	}
	dtoml.Share = scalarToString(d.Share.V)
	dtoml.Index = d.Share.I
	fd, err := createSecureFile(file)
	if err != nil {
		return err
	}
	defer fd.Close()
	return toml.NewEncoder(fd).Encode(dtoml)
}

func LoadShare(file string) (*dkg.DistKeyShare, error) {
	return nil, nil
}

func pointToString(p kyber.Point) string {
	buff, _ := p.MarshalBinary()
	return hex.EncodeToString(buff)
}

func scalarToString(s kyber.Scalar) string {
	buff, _ := s.MarshalBinary()
	return hex.EncodeToString(buff)
}
