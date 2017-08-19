package main

import (
	"fmt"
	"reflect"

	"github.com/dedis/protobuf"
	kyber "gopkg.in/dedis/kyber.v1"
	"gopkg.in/dedis/kyber.v1/share/pedersen/dkg"
)

type Drand struct {
	Hello *Public
	Dkg   *DKGPacket
	Tbls  *TBLS
}

type DKGPacket struct {
	Deal          *dkg.Deal
	Response      *dkg.Response
	Justification *dkg.Justification
}

type TBLS struct {
	Message []byte
}

// unmarshal reads the protobuf encoded buffer into a Drand struct
func unmarshal(g kyber.Group, buff []byte) (*Drand, error) {
	cons := make(protobuf.Constructors)
	var s kyber.Scalar
	var p kyber.Point
	cons[reflect.TypeOf(&s).Elem()] = func() interface{} { return g.Scalar() }
	cons[reflect.TypeOf(&p).Elem()] = func() interface{} { return g.Point() }
	fmt.Printf("#1 --> %v --> %v\n", cons, g)
	var drand = new(Drand)
	return drand, protobuf.DecodeWithConstructors(buff, drand, cons)
}
