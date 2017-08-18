package main

import kyber "gopkg.in/dedis/kyber.v1"
import "strings"

type Private struct {
	Key    kyber.Point
	Public *Public
}

type Public struct {
	Key     kyber.Point
	Address string
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
