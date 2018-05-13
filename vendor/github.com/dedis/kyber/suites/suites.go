// Package suites allows callers to look up Kyber suites by name.
//
// Currently, only the "ed25519" suite is available by default. To
// have access to "curve25519" and the NIST suites (i.e. "P256"),
// one needs to call the "go" tool with the tag "vartime", such as:
//
//   go build -tags vartime
//   go install -tags vartime
//   go test -tags vartime
package suites

import (
	"errors"
	"strings"

	"github.com/dedis/kyber"
)

// Suite is the sum of all suites mix-ins in Kyber.
type Suite interface {
	kyber.Encoding
	kyber.Group
	kyber.HashFactory
	kyber.XOFFactory
	kyber.Random
}

var suites = map[string]Suite{}

// register is called by suites to make themselves known to Kyber.
//
func register(s Suite) {
	suites[strings.ToLower(s.String())] = s
}

// ErrUnknownSuite indicates that the suite was not one of the
// registered suites.
var ErrUnknownSuite = errors.New("unknown suite")

// Find looks up a suite by name.
func Find(name string) (Suite, error) {
	if s, ok := suites[strings.ToLower(name)]; ok {
		return s, nil
	}
	return nil, ErrUnknownSuite
}

// MustFind looks up a suite by name and panics if it is not found.
func MustFind(name string) Suite {
	s, err := Find(name)
	if err != nil {
		panic("Suite " + name + " not found.")
	}
	return s
}
