// +build experimental

package openssl

// #include <openssl/sha.h>
// #cgo CFLAGS: -Wno-deprecated
// #cgo LDFLAGS: -lcrypto
import "C"

import (
	"hash"
	"unsafe"
)

// SHA1 hash function
type sha1 struct {
	ctx C.SHA_CTX
}

func (h *sha1) Reset() {
	if C.SHA1_Init(&h.ctx) != 1 {
		panic("SHA1_Init failed") // hash funcs shouldn't fail
	}
}

func (h *sha1) Write(p []byte) (n int, err error) {
	l := len(p)
	if C.SHA1_Update(&h.ctx, unsafe.Pointer(&p[0]), C.size_t(l)) == 0 {
		panic("SHA1_Update failed")
	}
	return l, nil
}

func (h *sha1) Size() int {
	return C.SHA_DIGEST_LENGTH
}

func (h *sha1) BlockSize() int {
	return C.SHA_CBLOCK
}

func (h *sha1) Sum(b []byte) []byte {
	c := h.ctx
	d := make([]byte, C.SHA_DIGEST_LENGTH)
	if C.SHA1_Final((*C.uchar)(&d[0]), &c) == 0 {
		panic("SHA1_Final failed")
	}
	return append(b, d...)
}

// Create a SHA-1 hash.
func NewSHA1() hash.Hash {
	s := new(sha1)
	s.Reset()
	return s
}

// SHA224 hash function
type sha224 struct {
	ctx C.SHA256_CTX
}

func (h *sha224) Reset() {
	if C.SHA224_Init(&h.ctx) != 1 {
		panic("SHA224_Init failed") // hash funcs shouldn't fail
	}
}

func (h *sha224) Write(p []byte) (n int, err error) {
	l := len(p)
	if C.SHA224_Update(&h.ctx, unsafe.Pointer(&p[0]), C.size_t(l)) == 0 {
		panic("SHA224_Update failed")
	}
	return l, nil
}

func (h *sha224) Size() int {
	return C.SHA224_DIGEST_LENGTH
}

func (h *sha224) BlockSize() int {
	return C.SHA256_CBLOCK
}

func (h *sha224) Sum(b []byte) []byte {
	c := h.ctx
	d := make([]byte, C.SHA224_DIGEST_LENGTH)
	if C.SHA224_Final((*C.uchar)(&d[0]), &c) == 0 {
		panic("SHA224_Final failed")
	}
	return append(b, d...)
}

// Create a SHA-224 hash.
func NewSHA224() hash.Hash {
	s := new(sha224)
	s.Reset()
	return s
}

// SHA256 hash function
type sha256 struct {
	ctx C.SHA256_CTX
}

func (h *sha256) Reset() {
	if C.SHA256_Init(&h.ctx) != 1 {
		panic("SHA256_Init failed") // hash funcs shouldn't fail
	}
}

func (h *sha256) Write(p []byte) (n int, err error) {
	l := len(p)
	if l > 0 && C.SHA256_Update(&h.ctx, unsafe.Pointer(&p[0]), C.size_t(l)) == 0 {
		panic("SHA256_Update failed")
	}
	return l, nil
}

func (h *sha256) Size() int {
	return C.SHA256_DIGEST_LENGTH
}

func (h *sha256) BlockSize() int {
	return C.SHA256_CBLOCK
}

func (h *sha256) Sum(b []byte) []byte {
	c := h.ctx
	d := make([]byte, C.SHA256_DIGEST_LENGTH)
	if C.SHA256_Final((*C.uchar)(unsafe.Pointer(&d[0])), &c) == 0 {
		panic("SHA256_Final failed")
	}
	return append(b, d...)
}

// Create a SHA-256 hash.
func NewSHA256() hash.Hash {
	s := new(sha256)
	s.Reset()
	return s
}

// SHA384 hash function
type sha384 struct {
	ctx C.SHA512_CTX
}

func (h *sha384) Reset() {
	if C.SHA384_Init(&h.ctx) != 1 {
		panic("SHA384_Init failed") // hash funcs shouldn't fail
	}
}

func (h *sha384) Write(p []byte) (n int, err error) {
	l := len(p)
	if C.SHA384_Update(&h.ctx, unsafe.Pointer(&p[0]), C.size_t(l)) == 0 {
		panic("SHA384_Update failed")
	}
	return l, nil
}

func (h *sha384) Size() int {
	return C.SHA384_DIGEST_LENGTH
}

func (h *sha384) BlockSize() int {
	return C.SHA512_CBLOCK
}

func (h *sha384) Sum(b []byte) []byte {
	c := h.ctx
	d := make([]byte, C.SHA384_DIGEST_LENGTH)
	if C.SHA384_Final((*C.uchar)(unsafe.Pointer(&d[0])), &c) == 0 {
		panic("SHA384_Final failed")
	}
	return append(b, d...)
}

// Create a SHA-384 hash.
func NewSHA384() hash.Hash {
	s := new(sha384)
	s.Reset()
	return s
}

// SHA512 hash function
type sha512 struct {
	ctx C.SHA512_CTX
}

func (h *sha512) Reset() {
	if C.SHA512_Init(&h.ctx) != 1 {
		panic("SHA512_Init failed") // hash funcs shouldn't fail
	}
}

func (h *sha512) Write(p []byte) (n int, err error) {
	l := len(p)
	if C.SHA512_Update(&h.ctx, unsafe.Pointer(&p[0]), C.size_t(l)) == 0 {
		panic("SHA512_Update failed")
	}
	return l, nil
}

func (h *sha512) Size() int {
	return C.SHA512_DIGEST_LENGTH
}

func (h *sha512) BlockSize() int {
	return C.SHA512_CBLOCK
}

func (h *sha512) Sum(b []byte) []byte {
	c := h.ctx
	d := make([]byte, C.SHA512_DIGEST_LENGTH)
	if C.SHA512_Final((*C.uchar)(unsafe.Pointer(&d[0])), &c) == 0 {
		panic("SHA512_Final failed")
	}
	return append(b, d...)
}

// Create a SHA-512 hash.
func NewSHA512() hash.Hash {
	s := new(sha512)
	s.Reset()
	return s
}
