package cipher

import (
	"crypto/cipher"
	"crypto/hmac"
	"hash"

	"gopkg.in/dedis/kyber.v1"
	"gopkg.in/dedis/kyber.v1/util/ints"
	"gopkg.in/dedis/kyber.v1/util/random"
)

type streamCipher struct {

	// Configuration state
	newStream                 func(key []byte) cipher.Stream
	newHash                   func() hash.Hash
	blockLen, keyLen, hashLen int

	// Per-message cipher state
	k []byte        // master secret state from last message, 0 if unkeyed
	h hash.Hash     // hash or hmac for absorbing input
	s cipher.Stream // stream cipher for encrypting, nil if none
}

const bufLen = 1024

var zeroBytes = make([]byte, bufLen)

// FromStream constructs a general message Cipher
// from a Stream cipher and a cryptographic Hash.
func FromStream(newStream func(key []byte) cipher.Stream,
	newHash func() hash.Hash, blockLen, keyLen, hashLen int,
	key []byte, options ...interface{}) kyber.Cipher {

	sc := streamCipher{}
	sc.newStream = newStream
	sc.newHash = newHash
	sc.blockLen = blockLen
	sc.keyLen = keyLen
	sc.hashLen = hashLen
	sc.h = sc.newHash()

	if key == nil {
		key = random.Bytes(hashLen, random.Stream)
	}
	if len(key) > 0 {
		sc.Message(nil, nil, key)
	}

	if len(options) > 0 {
		panic("no FromStream options supported yet")
	}

	return kyber.Cipher{CipherState: &sc}
}

func (sc *streamCipher) Partial(dst, src, key []byte) {

	n := ints.Max(len(dst), len(src), len(key)) // bytes to process

	// create our Stream cipher if needed
	if sc.s == nil {
		if sc.k == nil {
			sc.k = make([]byte, sc.hashLen)
		}
		sc.s = sc.newStream(sc.k[:sc.keyLen])
	}

	// squeeze cryptographic output
	ndst := ints.Min(n, len(dst))    // # bytes to write to dst
	nsrc := ints.Min(ndst, len(src)) // # src bytes available
	sc.s.XORKeyStream(dst[:nsrc], src[:nsrc])
	if n > nsrc {
		buf := make([]byte, n-nsrc)
		sc.s.XORKeyStream(buf, buf)
		copy(dst[nsrc:], buf)
	}

	// absorb cryptographic input (which may overlap with dst)
	if key != nil {
		nkey := ints.Min(n, len(key)) // # key bytes available
		_, _ = sc.h.Write(key[:nkey])
		if n > nkey {
			buf := make([]byte, n-nkey)
			_, _ = sc.h.Write(buf)
		}
	}
}

func (sc *streamCipher) Message(dst, src, key []byte) {
	sc.Partial(dst, src, key)

	sc.k = sc.h.Sum(sc.k[:0])         // update state with absorbed data
	sc.h = hmac.New(sc.newHash, sc.k) // ready for next msg
	sc.s = nil                        // create a fresh stream cipher
}

func (sc *streamCipher) KeySize() int {
	return sc.keyLen
}

func (sc *streamCipher) HashSize() int {
	return sc.hashLen
}

func (sc *streamCipher) BlockSize() int {
	return sc.blockLen
}

func (sc *streamCipher) Clone() kyber.CipherState {
	if sc.s != nil {
		panic("cannot clone cipher state mid-message")
	}

	nsc := *sc
	if sc.k != nil { // keyed state
		nsc.k = make([]byte, sc.hashLen)
		copy(nsc.k, sc.k)
		nsc.h = hmac.New(nsc.newHash, nsc.k)
	} else { // unkeyed state
		nsc.h = nsc.newHash()
	}

	return &nsc
}
