// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sha3

// This file provides functions for creating instances of the SHA-3
// and SHAKE hash functions, as well as utility functions for hashing
// bytes.

import (
	"hash"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/cipher"
)

var sha3opts = []interface{}{cipher.Padding(0x06)}

// NewCipher224 creates a Cipher implementing the SHA3-224 algorithm,
// which provides 224-bit security against preimage attacks
// and 112-bit security against collisions.
func NewCipher224(key []byte, options ...interface{}) kyber.Cipher {
	return cipher.FromSponge(newKeccak448(), key,
		append(sha3opts, options...)...)
}

// NewCipher256 creates a Cipher implementing the SHA3-256 algorithm,
// which provides 256-bit security against preimage attacks
// and 128-bit security against collisions.
func NewCipher256(key []byte, options ...interface{}) kyber.Cipher {
	return cipher.FromSponge(newKeccak512(), key,
		append(sha3opts, options...)...)
}

// NewCipher384 creates a Cipher implementing the SHA3-384 algorithm,
// which provides 384-bit security against preimage attacks
// and 192-bit security against collisions.
func NewCipher384(key []byte, options ...interface{}) kyber.Cipher {
	return cipher.FromSponge(newKeccak768(), key,
		append(sha3opts, options...)...)
}

// NewCipher512 creates a Cipher implementing the SHA3-512 algorithm,
// which provides 512-bit security against preimage attacks
// and 256-bit security against collisions.
func NewCipher512(key []byte, options ...interface{}) kyber.Cipher {
	return cipher.FromSponge(newKeccak1024(), key,
		append(sha3opts, options...)...)
}

// New224 creates a new SHA3-224 hash.
// Its generic security strength is 224 bits against preimage attacks,
// and 112 bits against collision attacks.
func New224() hash.Hash {
	return cipher.NewHash(NewCipher224, 224/8)
}

// New256 creates a new SHA3-256 hash.
// Its generic security strength is 256 bits against preimage attacks,
// and 128 bits against collision attacks.
func New256() hash.Hash {
	return cipher.NewHash(NewCipher256, 256/8)
}

// New384 creates a new SHA3-384 hash.
// Its generic security strength is 384 bits against preimage attacks,
// and 192 bits against collision attacks.
func New384() hash.Hash {
	return cipher.NewHash(NewCipher384, 384/8)
}

// New512 creates a new SHA3-512 hash.
// Its generic security strength is 512 bits against preimage attacks,
// and 256 bits against collision attacks.
func New512() hash.Hash {
	return cipher.NewHash(NewCipher512, 512/8)
}

// Sum224 returns the SHA3-224 digest of the data.
func Sum224(data []byte) (digest [28]byte) {
	h := New224()
	h.Write(data)
	h.Sum(digest[:0])
	return
}

// Sum256 returns the SHA3-256 digest of the data.
func Sum256(data []byte) (digest [32]byte) {
	h := New256()
	h.Write(data)
	h.Sum(digest[:0])
	return
}

// Sum384 returns the SHA3-384 digest of the data.
func Sum384(data []byte) (digest [48]byte) {
	h := New384()
	h.Write(data)
	h.Sum(digest[:0])
	return
}

// Sum512 returns the SHA3-512 digest of the data.
func Sum512(data []byte) (digest [64]byte) {
	h := New512()
	h.Write(data)
	h.Sum(digest[:0])
	return
}
