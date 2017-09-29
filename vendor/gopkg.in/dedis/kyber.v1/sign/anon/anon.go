// Package anon implements cryptographic primitives for anonymous communication.
package anon

import (
	"gopkg.in/dedis/kyber.v1"
)

// Set represents an explicit anonymity set
// as a list of public keys.
type Set []kyber.Point

// PriKey represents a member of an anonymity set
type PriKey struct {
	Set               // Public key-set
	Mine int          // Index of the public key I own
	Pri  kyber.Scalar // Private key for that public key
}
