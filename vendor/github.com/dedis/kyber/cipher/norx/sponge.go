// Package norx implements the experimental NORX cipher.
// For details on the NORX cipher see https://norx.io
// This package is very experimental and NOT for use in prodution systems.
//
// This is a fork of the NORX implementation in Go by Philipp Jovanovic,
// from http://github.com/daeinar/norx-go
package norx

import (
	"encoding/binary"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/cipher"
)

func (s *state_t) Rate() int { return BYTES_RATE }

func (s *state_t) Capacity() int {
	return (WORDS_STATE - WORDS_RATE) * BYTES_WORD
}

func (s state_t) Clone() cipher.Sponge {
	return &s
}

func (s *state_t) Transform(dst, src []byte) {

	a := s.s[:]
	for len(src) > 0 {
		a[0] ^= binary.LittleEndian.Uint64(src)
		src = src[8:]
		a = a[1:]
	}

	permute(s)

	a = s.s[:]
	for len(dst) > 0 {
		binary.LittleEndian.PutUint64(dst, a[0])
		a = a[1:]
		dst = dst[8:]
	}
}

func newSponge() cipher.Sponge {
	var zeros [32]uint8
	s := &state_t{}
	setup(s, zeros[:], zeros[:]) // XXX initialize via options
	return s
}

// NewCipher creates a Cipher implementing the 64-4-1 mode of NORX.
func NewCipher(key []byte, options ...interface{}) kyber.Cipher {
	return cipher.FromSponge(newSponge(), key, options...)
}
