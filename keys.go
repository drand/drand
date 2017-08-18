package main

import (
	"strings"

	"github.com/dedis/drand/pbc"
	kyber "gopkg.in/dedis/kyber.v1"
	"gopkg.in/dedis/kyber.v1/util/random"
)

var pairing = pbc.NewPairingFp382_1()

// Private key is a wrapper around a random scalar  and the corresponding public
// key in G2
type Private struct {
	Key    kyber.Point
	Public *Public
}

type Public struct {
	Key     kyber.Point
	Address string
}

func NewKeyPair(address string) *Private {
	g := pairing.G2()
	key := g.Scalar().Pick(random.Stream)
}

func (p *Public) Equal(p2 *Public) bool {
	return p.Key.Equal(p2.Key) && p.Address == p2.Address
}

type Publics []*Public

func (p *Publics) Len() int {
	return len(*p)
}

func (p *Publics) Swap(i, j int) {
	(*p)[i], (*p)[j] = (*p)[j], (*p)[i]
}

func (p *Publics) Less(i, j int) bool {
	is := (*p)[i].Key.String()
	js := (*p)[j].Key.String()
	return strings.Compare(is, js) < 0
}

func (p *Publics) Contains(pub *Public) bool {
	for _, pu := range *p {
		if pu.Equal(pub) {
			return true
		}
	}
	return false
}
