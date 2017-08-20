package main

import (
	"encoding/hex"
	"os"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/dedis/drand/pbc"
	kyber "gopkg.in/dedis/kyber.v1"
	"gopkg.in/dedis/kyber.v1/share/pedersen/dkg"
	"gopkg.in/dedis/kyber.v1/util/random"
)

var pairing = pbc.NewPairingFp382_1()
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
	buff, _ := p.Key.MarshalBinary()
	hexKey := hex.EncodeToString(buff)
	fd, err := os.Create(file)
	if err != nil {
		return err
	}
	if err := fd.Chmod(0644); err != nil {
		return err
	}
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

func (p *Public) Load(file string) error {
	ptoml := &PublicTOML{}
	if _, err := toml.DecodeFile(file, ptoml); err != nil {
		return err
	}
	buff, err := hex.DecodeString(ptoml.Key)
	if err != nil {
		return err
	}
	p.Address = ptoml.Address
	p.Key = g2.Point()
	return p.Key.UnmarshalBinary(buff)

}

func (p *Public) Save(prefix string) error {
	hex := p.Key.String()
	fd, err := os.Create(publicFile(prefix))
	if err != nil {
		return err
	}
	return toml.NewEncoder(fd).Encode(&PublicTOML{p.Address, hex})

}

func (p *Public) Equal(p2 *Public) bool {
	return p.Key.Equal(p2.Key) && p.Address == p2.Address
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
type Group []*IndexedPublic

// IndexedPublic wraps a Public with its index relative to the group
type IndexedPublic struct {
	*Public
	Index int
}

// IntoGroup returns an indexed list of publics sorted by the alphabetical
// hexadecimal representation of the individual public keys.
func IntoGroup(list []*Public) Group {
	sort.Sort(ByKey(list))
	il := make(Group, len(list))
	for i, p := range list {
		il[i] = &IndexedPublic{
			Public: p,
			Index:  i,
		}
	}
	return il
}

// Load decodes the group pointed by the filename given in arguments
func (g *Group) Load(file string) error {
	var list []*Public
	if _, err := toml.DecodeFile(file, list); err != nil {
		return err
	}
	gg := IntoGroup(list)
	g = &gg
	return nil
}

// Save stores the group into the given file name.
func (g *Group) Save(file string) error {
	fd, err := os.Create(file)
	if err != nil {
		return err
	}
	return toml.NewEncoder(fd).Encode(g)
}

// Contains returns true if the public key is contained in the list or not.
func (i *Group) Contains(pub *Public) bool {
	for _, pu := range *i {
		if pu.Equal(pub) {
			return true
		}
	}
	return false
}

// Index returns the index of the given public key with a boolean indicating
// whether the public has been found or not.
func (i *Group) Index(pub *Public) (int, bool) {
	for _, pu := range *i {
		if pu.Equal(pub) {
			return pu.Index, true
		}
	}
	return 0, false
}

// Points returns itself under the form of a list of kyber.Point
func (i *Group) Points() []kyber.Point {
	pts := make([]kyber.Point, len(*i))
	for _, pu := range *i {
		pts[pu.Index] = pu.Key
	}
	return pts
}

func SaveShare(d *dkg.DistKeyShare) error {

}

func LoadShare(file string) (*dkg.DistKeyShare, error) {

}
