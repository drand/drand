// +build experimental
// +build sodium

// This package implements the BLAKE2b cryptographic hash function,
// described at:
//
//	https://blake2.net
//
package blake2

// #include "blake2.h"
import "C"

import (
	"unsafe"
)

const Blake2bBlockBytes = 128
const Blake2bOutBytes = 64
const Blake2bKeyBytes = 64
const Blake2bSaltBytes = 16
const Blake2bPersonalBytes = 16

type Blake2bParam struct {
	digestLength uint8                      // 1
	keyLength    uint8                      // 2
	fanout       uint8                      // 3
	depth        uint8                      // 4
	leafLength   uint32                     // 8
	nodeOffset   uint64                     // 16
	nodeDepth    uint8                      // 17
	innerLength  uint8                      // 18
	_            [14]byte                   // 32
	salt         [Blake2bSaltBytes]byte     // 48
	personal     [Blake2bPersonalBytes]byte // 64
}

type Blake2bState struct {
	s C.blake2b_state

	// Initialization parameters, saved for Hash.Reset()
	key   []byte
	param *Blake2bParam
}

func NewBlake2b(key []byte, param *Blake2bParam) *Blake2bState {
	s := new(Blake2bState)
	s.Init(key, param)
	return s
}

// Initialize BLAKE2b state, with optional key and/or parameters.
func (s *Blake2bState) Init(key []byte, param *Blake2bParam) {
	s.key = key
	s.param = param
	if param == nil {
		C.blake2b_init(&s.s, Blake2bOutBytes)
	} else {
		C.blake2b_init_param(&s.s,
			(*C.blake2b_param)(unsafe.Pointer(param)))
	}

	// If key was provided, use it as the first data block to hash.
	if len(key) > 0 {
		if len(key) > Blake2bKeyBytes {
			panic("key too long for BLAKE2b")
		}
		var block [Blake2bBlockBytes]byte
		copy(block[:], key)
		s.Write(block[:])
		for i := range block {
			block[i] = 0 // erase key from temporary block
		}
	}
}

// Reset to initial state.
// If Init() was previously called with a key and/or parameter block,
// they are reused to reproduce an identical initial state.
func (s *Blake2bState) Reset() {
	s.Init(s.key, s.param)
}

// Write bytes to update hash state.
func (s *Blake2bState) Write(p []byte) (n int, err error) {
	l := len(p)
	C.blake2b_update(&s.s, (*C.uint8_t)(unsafe.Pointer(&p[0])),
		C.uint64_t(l))
	return l, nil
}

func (s *Blake2bState) Size() int {
	return Blake2bOutBytes
}

func (s *Blake2bState) BlockSize() int {
	return Blake2bBlockBytes
}

// Return the hash of the stream up to this point, without changing the state.
func (s *Blake2bState) Sum(b []byte) []byte {

	// Make a copy of the hash state, since blake2b_final() mutates it
	// but the Hash interface specifies that Sum() does not affect state.
	st := s.s

	var d [Blake2bOutBytes]byte
	if C.blake2b_final(&st, (*C.uint8_t)(unsafe.Pointer(&d[0])),
		C.uint8_t(Blake2bOutBytes)) != 0 {
		panic("blake2b_final failed")
	}
	return append(b, d[:]...)
}

// Implementation of the cipher.Stream interface
//func (h *Blake2bState) XORKeyStream(dst,src []byte) {
//}
