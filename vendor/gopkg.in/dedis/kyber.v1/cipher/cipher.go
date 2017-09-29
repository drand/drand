package cipher

import (
	"crypto/cipher"
)

// Stream is just an alias for cipher.Stream
type Stream cipher.Stream

// Block is just an alias for cipher.Block
type Block cipher.Block

// NoKey is given to a Cipher constructor to create an unkeyed
// Cipher.
var NoKey = []byte{}

// RandomKey is given to a Cipher constructor to create a
// randomly seeded Cipher.
var RandomKey []byte
