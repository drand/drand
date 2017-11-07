package test

import (
	"bytes"
	"crypto/cipher"
	"crypto/rand"
	"hash"
	"testing"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/util/subtle"
)

// HashBench performs a benchmark on a hash function
func HashBench(b *testing.B, hash func() hash.Hash) {
	b.SetBytes(1024 * 1024)
	data := make([]byte, 1024)
	for i := 0; i < b.N; i++ {
		h := hash()
		for j := 0; j < 1024; j++ {
			_, _ = h.Write(data)
		}
		h.Sum(nil)
	}
}

// StreamCipherBench performs a benchmark on a stream cipher.
func StreamCipherBench(b *testing.B, keylen int,
	cipher func([]byte) cipher.Stream) {
	key := make([]byte, keylen)
	b.SetBytes(1024 * 1024)
	data := make([]byte, 1024)
	for i := 0; i < b.N; i++ {
		c := cipher(key)
		for j := 0; j < 1024; j++ {
			c.XORKeyStream(data, data)
		}
	}
}

// BlockCipherBench performs a benchmark on a block cipher operating in counter mode.
func BlockCipherBench(b *testing.B, keylen int,
	bcipher func([]byte) cipher.Block) {
	StreamCipherBench(b, keylen, func(key []byte) cipher.Stream {
		bc := bcipher(key)
		iv := make([]byte, bc.BlockSize())
		return cipher.NewCTR(bc, iv)
	})
}

// BitDiff compares the bits between two arrays returning the fraction
// of differences. If the two arrays are not of the same length
// no comparison is made and a -1 is returned.
func BitDiff(a, b []byte) float64 {
	if len(a) != len(b) {
		return -1
	}

	count := 0
	for i := 0; i < len(a); i++ {
		for j := 0; j < 8; j++ {
			count += int(((a[i] ^ b[i]) >> uint(j)) & 1)
		}
	}

	return float64(count) / float64(len(a)*8)
}

// CipherHelloWorldHelper test if a Cipher can encrypt and decrypt.
func CipherHelloWorldHelper(t *testing.T,
	newCipher func([]byte, ...interface{}) kyber.Cipher,
	n int, bitdiff float64) {
	text := []byte("Hello, World")
	cryptsize := len(text)

	bc := newCipher(nil)
	keysize := bc.KeySize()

	nkeys := make([][]byte, n)
	ncrypts := make([][]byte, n)

	for i := range nkeys {
		nkeys[i] = make([]byte, keysize)
		_, _ = rand.Read(nkeys[i])
		bc = newCipher(nkeys[i])
		ncrypts[i] = make([]byte, cryptsize)
		bc.Message(ncrypts[i], text, ncrypts[i])

		bc = newCipher(nkeys[i])
		decrypted := make([]byte, len(text))
		bc.Message(decrypted, ncrypts[i], ncrypts[i])
		if !bytes.Equal(text, decrypted) {
			t.Log("Encryption / Decryption failed", i)
			t.FailNow()
		}
	}

	for i := range ncrypts {
		for j := i + 1; j < len(ncrypts); j++ {
			if bytes.Equal(ncrypts[i], ncrypts[j]) {
				t.Log("Different keys result in same encryption")
				t.FailNow()
			}

			res := BitDiff(ncrypts[i], ncrypts[j])
			if res < bitdiff {
				t.Log("Encryptions not sufficiently different:", res)
				t.FailNow()
			}
		}
	}
}

// AuthenticateAndEncrypt tests a Cipher if:
// 1) Encryption / decryption works
// 2) Encryption / decryption with different key don't work
// 3) Changing a bit in the ciphertext or mac results in failed mac check
// 4) Different keys produce sufficiently random output
func AuthenticateAndEncrypt(t *testing.T,
	newCipher func([]byte, ...interface{}) kyber.Cipher,
	n int, bitdiff float64, text []byte) {
	cryptsize := len(text)
	decrypted := make([]byte, len(text))

	bc := newCipher(nil)
	keysize := bc.KeySize()
	hashsize := bc.HashSize()
	mac := make([]byte, hashsize)

	ncrypts := make([][]byte, n)
	nkeys := make([][]byte, n)
	nmacs := make([][]byte, n)

	// Encrypt / decrypt / mac test
	for i := range nkeys {
		nkeys[i] = make([]byte, keysize)
		_, _ = rand.Read(nkeys[i])
		bc = newCipher(nkeys[i])
		ncrypts[i] = make([]byte, cryptsize)
		bc.Message(ncrypts[i], text, ncrypts[i])
		nmacs[i] = make([]byte, hashsize)
		bc.Message(nmacs[i], nil, nil)

		bc = newCipher(nkeys[i])
		bc.Message(decrypted, ncrypts[i], ncrypts[i])
		if !bytes.Equal(text, decrypted) {
			t.Log("Encryption / Decryption failed", i)
			t.FailNow()
		}

		bc.Message(mac, nmacs[i], nil)
		if subtle.ConstantTimeAllEq(mac, 0) != 1 {
			t.Log("MAC Check failed")
			t.FailNow()
		}
	}

	// Different keys test
	for i := range ncrypts {
		for j := range ncrypts {
			if i == j {
				continue
			}
			bc = newCipher(nkeys[i])
			bc.Message(decrypted, ncrypts[j], ncrypts[j])
			bc.Message(mac, nmacs[j], nil)
			if subtle.ConstantTimeAllEq(mac, 0) == 1 {
				t.Log("MAC Check passed")
				t.FailNow()
			}
		}
	}

	// Not enough randomness in 1 byte to pass this consistently
	if len(ncrypts[0]) < 8 {
		return
	}

	// Bit difference test
	for i := range ncrypts {
		for j := i + 1; j < len(ncrypts); j++ {
			res := BitDiff(ncrypts[i], ncrypts[j])
			if res < bitdiff {
				t.Log("Encryptions not sufficiently different", res)
				t.FailNow()
			}
		}
	}

	deltacopy := make([]byte, cryptsize)

	// Bits in either testmsg or testmac should be flipped
	// then the resulting MAC check should fail
	deltatest := func(index int, testmsg []byte, testmac []byte) {
		bc = newCipher(nkeys[index])
		bc.Message(decrypted, testmsg, testmsg)
		bc.Message(mac, testmac, nil)
		if subtle.ConstantTimeAllEq(mac, 0) == 1 {
			t.Log("MAC Check passed")
			t.FailNow()
		}
	}

	for i := range ncrypts {
		copy(ncrypts[i], deltacopy)

		deltacopy[0] ^= 255
		deltatest(i, deltacopy, nmacs[i])
		deltacopy[0] = ncrypts[i][0]

		deltacopy[len(deltacopy)/2-1] ^= 255
		deltatest(i, deltacopy, nmacs[i])
		deltacopy[len(deltacopy)/2-1] = ncrypts[i][len(deltacopy)/2-1]

		deltacopy[len(deltacopy)-1] ^= 255
		deltatest(i, deltacopy, nmacs[i])
		deltacopy[len(deltacopy)-1] = ncrypts[i][len(deltacopy)-1]

		deltamac := make([]byte, hashsize)
		copy(nmacs[i], deltamac)
		deltamac[0] ^= 255
		deltatest(i, ncrypts[i], deltamac)
	}
}

// CipherAuthenticatedEncryptionHelper iterates through various sized messages and verify
// that encryption and authentication work
func CipherAuthenticatedEncryptionHelper(t *testing.T,
	newCipher func([]byte, ...interface{}) kyber.Cipher,
	n int, bitdiff float64) {
	//	AuthenticateAndEncrypt(t, newCipher, n, bitdiff, []byte{})
	AuthenticateAndEncrypt(t, newCipher, n, bitdiff, []byte{'a'})
	AuthenticateAndEncrypt(t, newCipher, n, bitdiff, []byte("Hello, World"))

	kb := make([]byte, 2^10)
	for i := 0; i < len(kb); i++ {
		kb[i] = byte(i & 256)
	}
	AuthenticateAndEncrypt(t, newCipher, n, bitdiff, kb)

	mb := make([]byte, 2^20)
	for i := 0; i < len(mb); i++ {
		mb[i] = byte(i & 256)
	}
	AuthenticateAndEncrypt(t, newCipher, n, bitdiff, mb)
}

// CipherTest test a Cipher functionalities.
func CipherTest(t *testing.T,
	newCipher func([]byte, ...interface{}) kyber.Cipher) {
	n := 5
	bitdiff := .30
	CipherHelloWorldHelper(t, newCipher, n, bitdiff)
	CipherAuthenticatedEncryptionHelper(t, newCipher, n, bitdiff)
}
