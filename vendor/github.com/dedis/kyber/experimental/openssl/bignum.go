// +build experimental

package openssl

// #include <openssl/bn.h>
// #cgo CFLAGS: -Wno-deprecated
// #cgo LDFLAGS: -lcrypto
//
// static inline int bn_sign(BIGNUM *n) {
//	return BN_is_zero(n) ? 0 : BN_is_negative(n) ? -1 : 1;
// }
//
import "C"

import (
	"crypto/cipher"
	"math/big"
	"runtime"
	"unsafe"
)

// Simple wrapper around OpenSSL's bignum routines
type bignum struct {
	bn *_Ctype_struct_bignum_st
}

func newBigNum() *bignum {
	n := new(bignum)
	n.Init()
	return n
}

func (n *bignum) Init() {
	if n.bn == nil {
		n.bn = C.BN_new()
		runtime.SetFinalizer(n, freeBigNum)
	}
}

func (n *bignum) BitLen() int {
	return int(C.BN_num_bits(n.bn))
}

// Convert this bignum to its big-endian representation
// in a byte slice at least buflen bytes long, padding as needed with zeros.
// If buflen == 0, the resulting buffer is as short as possible.
func (n *bignum) Bytes(buflen int) []byte {
	l := (n.BitLen() + 7) / 8 // byte length of the actual bignum
	if buflen < l {
		buflen = l
	}
	buf := make([]byte, buflen)
	z := buflen - l // leading zero bytes we need to prepend
	if C.BN_bn2bin(n.bn, (*_Ctype_unsignedchar)(unsafe.Pointer(&buf[z]))) != C.int(l) {
		panic("BN_bn2bin returned wrong length")
	}
	return buf
}

func (n *bignum) SetBytes(buf []byte) *bignum {
	if C.BN_bin2bn((*_Ctype_unsignedchar)(unsafe.Pointer(&buf[0])), C.int(len(buf)), n.bn) != n.bn {
		panic("BN_bin2bn failed: " + getErrString())
	}
	return n
}

// Convert an OpenSSL bignum to a native Go BitInt
func (n *bignum) BigInt() *big.Int {
	return new(big.Int).SetBytes(n.Bytes(0))
}

// Set bignum's value from a native Go BigInt
func (n *bignum) SetBigInt(i *big.Int) *bignum {
	n.SetBytes(i.Bytes())
	return n
}

func (n *bignum) Cmp(m *bignum) int {
	return int(C.BN_cmp(n.bn, m.bn))
}

func (n *bignum) Sign(m *bignum) int {
	return int(C.bn_sign(n.bn))
}

// Set bignum n to a uniform random BigInt with a given maximum BitLen.
// If 'exact' is true, choose a BigInt with _exactly_ that BitLen, not less
func (n *bignum) RandBits(bitlen uint, exact bool, rand cipher.Stream) *bignum {
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
	n.SetBytes(b)
	return n
}

// Set bignum n to a uniform random BigInt less than a given modulus mod
func (n *bignum) RandMod(mod *bignum, rand cipher.Stream) *bignum {
	bitlen := uint(mod.BitLen())
	for {
		i := n.RandBits(bitlen, false, rand)
		if i.Cmp(mod) < 0 {
			return n
		}
	}
}

func freeBigNum(n *bignum) {
	//println("freeBigNum",n)
	C.BN_free(n.bn)
	n.bn = nil
}
