// Package group holds a reference to all kyber.Group and to all cipher suites
// defined. It provides a quick access to one specific suite using the
//
//  Suite("ed25519")
//
// method. Currently, only the "ed25519" suite is available by default. To have
// access to the "curve25519" and all nist/ suites, one needs to build the
// kyber library with the tag "vartime", such as:
//
//   go build -tags vartime
//
// Note that all suite and groups references are case insensitive.
package group

import (
	"strings"

	"gopkg.in/dedis/kyber.v1/group/edwards25519"
)

var suites = map[string]interface{}{}

func init() {
	ed25519 := edwards25519.NewAES128SHA256Ed25519()
	suites[strings.ToLower(ed25519.String())] = ed25519
}

// Suite return
func Suite(name string) interface{} {
	s, ok := suites[strings.ToLower(name)]
	if !ok {
		panic("group has no suite named " + name)
	}
	return s
}
