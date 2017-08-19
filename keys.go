package main

import (
	"sort"
	"strings"

	"github.com/dedis/drand/pbc"
	kyber "gopkg.in/dedis/kyber.v1"
	"gopkg.in/dedis/kyber.v1/util/random"
)

var pairing = pbc.NewPairingFp382_1()

// Private key is a wrapper around a random scalar  and the corresponding public
// key in G2
type Private struct {
	Key    kyber.Scalar
	Public *Public
}

type Public struct {
	Key     kyber.Point
	Address string
}

// NewKeyPair returns a freshly created private / public key pair.
func NewKeyPair(address string) *Private {
	g := pairing.G2()
	key := g.Scalar().Pick(random.Stream)
	pubKey := g.Point().Mul(key, nil)
	pub := &Public{
		Key:     pubKey,
		Address: address,
	}
	return &Private{
		Key:    key,
		Public: pub,
	}
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

// IndexedList returns an indexed list of publics sorted by the alphabetical
// hexadecimal representation of the individual public keys.
func Sort(list []*Public) IndexedList {
	sort.Sort(ByKey(list))
	il := make(IndexedList, len(list))
	for i, p := range list {
		il[i] = &IndexedPublic{
			Public: p,
			Index:  i,
		}
	}
	return il
}

type IndexedPublic struct {
	*Public
	Index int
}

// IndexedList is a list of IndexedPublic providing helper methods to search and
// get public keys from a list.
type IndexedList []*IndexedPublic

// Contains returns true if the public key is contained in the list or not.
func (i *IndexedList) Contains(pub *Public) bool {
	for _, pu := range *i {
		if pu.Equal(pub) {
			return true
		}
	}
	return false
}

// Index returns the index of the given public key with a boolean indicating
// whether the public has been found or not.
func (i *IndexedList) Index(pub *Public) (int, bool) {
	for _, pu := range *i {
		if pu.Equal(pub) {
			return pu.Index, true
		}
	}
	return 0, false
}

// Points returns itself under the form of a list of kyber.Point
func (i *IndexedList) Points() []kyber.Point {
	pts := make([]kyber.Point, len(*i))
	for _, pu := range *i {
		pts[pu.Index] = pu.Key
	}
	return pts
}
