package main

import (
	"reflect"

	"github.com/dedis/protobuf"
	kyber "gopkg.in/dedis/kyber.v1"
)

type Drand struct {
	Hello *Public
	Dkg   *DKG
	Tbls  *TBLS
}

type DKG struct {
	Public kyber.Point
}

type TBLS struct {
	Message []byte
}

// constructors returns a protobuf.Constructor instantiated with the right group
func constructors(g kyber.Group) protobuf.Constructors {
	cons := make(protobuf.Constructors)
	var s kyber.Scalar
	var p kyber.Point
	cons[reflect.TypeOf(&s).Elem()] = func() interface{} { return g.Scalar() }
	cons[reflect.TypeOf(&p).Elem()] = func() interface{} { return g.Point() }
	return cons
}

// unmarshal reads the protobuf encoded buffer into a Drand struct
func unmarshal(g kyber.Group, buff []byte) (*Drand, error) {
	cons := constructors(g)
	var drand Drand
	return &drand, protobuf.DecodeWithConstructors(buff, &drand, cons)
}
