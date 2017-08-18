package main

import kyber "gopkg.in/dedis/kyber.v1"

type Drand struct {
	Dkg  *DKG
	Tbls *TBLS
}

type DKG struct {
	Public kyber.Point
}
