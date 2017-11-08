// Package random provides facilities for generating
// random or pseudorandom cryptographic objects.
package random

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"math/big"
)

// Bits chooses a uniform random BigInt with a given maximum BitLen.
// If 'exact' is true, choose a BigInt with _exactly_ that BitLen, not less
func Bits(bitlen uint, exact bool, rand cipher.Stream) []byte {
	b := make([]byte, (bitlen+7)/8)
	rand.XORKeyStream(b, b)
	highbits := bitlen & 7
	if highbits != 0 {
		b[0] &= ^(0xff << highbits)
	}
	if exact {
		if highbits != 0 {
			b[0] |= 1 << (highbits - 1)
		} else {
			b[0] |= 0x80
		}
	}
	return b
}

// Bool chooses a uniform random boolean
func Bool(rand cipher.Stream) bool {
	b := Bits(8, false, rand)
	return b[0]&1 != 0
}

// Byte chooses a uniform random byte
func Byte(rand cipher.Stream) byte {
	b := Bits(8, false, rand)
	return b[0]
}

// Uint8 chooses a uniform random uint8
func Uint8(rand cipher.Stream) uint8 {
	b := Bits(8, false, rand)
	return uint8(b[0])
}

// Uint16 chooses a uniform random uint16
func Uint16(rand cipher.Stream) uint16 {
	b := Bits(16, false, rand)
	return binary.BigEndian.Uint16(b)
}

// Uint32 chooses a uniform random uint32
func Uint32(rand cipher.Stream) uint32 {
	b := Bits(32, false, rand)
	return binary.BigEndian.Uint32(b)
}

// Uint64 chooses a uniform random uint64
func Uint64(rand cipher.Stream) uint64 {
	b := Bits(64, false, rand)
	return binary.BigEndian.Uint64(b)
}

// Int choose a uniform random big.Int less than a given modulus
func Int(mod *big.Int, rand cipher.Stream) *big.Int {
	bitlen := uint(mod.BitLen())
	i := new(big.Int)
	for {
		i.SetBytes(Bits(bitlen, false, rand))
		if i.Sign() > 0 && i.Cmp(mod) < 0 {
			return i
		}
	}
}

// Bytes chooses a random n-byte slice
func Bytes(n int, rand cipher.Stream) []byte {
	b := make([]byte, n)
	rand.XORKeyStream(b, b)
	return b
}

// NonZeroBytes calls Bytes as long as it gets a slice full of '0's.
// This is needed when using suite.Cipher(cipher.NoKey)
// because the first 6 iterations returns 0000...000 as
// bytes for edwards & ed25519 cipher.
// Issue reported in https://github.com/dedis/kyber/issues/70
func NonZeroBytes(n int, rand cipher.Stream) []byte {
	var randoms []byte
	for {
		randoms = Bytes(n, rand)
		for _, b := range randoms {
			if b != 0x00 {
				return randoms
			}
		}
	}
}

type randstream struct {
}

func (r *randstream) XORKeyStream(dst, src []byte) {
	l := len(dst)
	if len(src) != l {
		panic("XORKeyStream: mismatched buffer lengths")
	}

	buf := make([]byte, l)
	n, err := rand.Read(buf)
	if err != nil {
		panic(err)
	}
	if n < len(buf) {
		panic("short read on infinite random stream!?")
	}

	for i := 0; i < l; i++ {
		dst[i] = src[i] ^ buf[i]
	}
}

// Stream is the standard virtual "stream cipher" that just generates
// fresh cryptographically strong random bits.
var Stream cipher.Stream = new(randstream)
