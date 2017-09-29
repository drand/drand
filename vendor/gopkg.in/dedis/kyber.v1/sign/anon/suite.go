package anon

import (
	"gopkg.in/dedis/kyber.v1"
)

// Suite represents the set of functionalities needed by the package anon.
type Suite interface {
	kyber.Group
	kyber.CipherFactory
	kyber.HashFactory
	kyber.Encoding
}
