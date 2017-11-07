// +build experimental

package openssl

// #include <openssl/aes.h>
// #cgo CFLAGS: -Wno-deprecated
// #cgo LDFLAGS: -lcrypto -ldl
import "C"

import (
	"crypto/cipher"
)

const blocksize = 16

type aes struct {
	key C.AES_KEY // expanded AES key
}

// Create a new AES block cipher.
// The key must be 16, 24, or 32 bytes long.
func NewAES(key []byte) cipher.Block {
	a := &aes{}
	if C.AES_set_encrypt_key((*C.uchar)(&key[0]), C.int(len(key)*8), &a.key) != 0 {
		panic("C.AES_set_encrypt_key failed")
	}
	return a
}

func (a *aes) BlockSize() int {
	return blocksize
}

func (a *aes) Encrypt(dst, src []byte) {
	C.AES_encrypt((*C.uchar)(&src[0]), (*C.uchar)(&dst[0]), &a.key)
}

func (a *aes) Decrypt(dst, src []byte) {
	C.AES_decrypt((*C.uchar)(&src[0]), (*C.uchar)(&dst[0]), &a.key)
}

// XXX probably obsolete; looks like cipher.NewCTR() is actually faster.
type aesctr struct {
	key      C.AES_KEY       // expanded AES key
	ctr, out [blocksize]byte // input counter and output buffer
	idx      int             // bytes of current block already used
}

// Create a new stream cipher based on AES in counter mode.
// The key must be 16, 24, or 32 bytes long.
func newAESCTR(key []byte) cipher.Stream {
	a := &aesctr{}
	if C.AES_set_encrypt_key((*C.uchar)(&key[0]), C.int(len(key)*8), &a.key) != 0 {
		panic("C.AES_set_encrypt_key failed")
	}
	// counter automatically starts at 0
	a.idx = blocksize // need a fresh block first time
	return a
}

func (a *aesctr) XORKeyStream(dst, src []byte) {
	for i := range src {
		if a.idx == blocksize {
			// generate a block by encrypting the current counter
			C.AES_encrypt((*C.uchar)(&a.ctr[0]), (*C.uchar)(&a.out[0]), &a.key)

			// increment the counter
			for j := blocksize - 1; ; j-- {
				a.ctr[j]++
				if a.ctr[j] != 0 {
					break
				}
			}

			a.idx = 0
		}

		dst[i] = src[i] ^ a.out[a.idx]
		a.idx++
	}
}
