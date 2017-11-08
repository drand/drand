package cipher

import (
	"crypto/cipher"
	"errors"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/util/bytes"
	"github.com/dedis/kyber/util/subtle"
)

type cipherAEAD struct {
	kyber.Cipher
}

// NewAEAD wraps a kyber.message Cipher to implement
// the Authenticated Encryption with Associated Data (AEAD) interface.
func NewAEAD(c kyber.Cipher) cipher.AEAD {
	return &cipherAEAD{c}
}

func (ca *cipherAEAD) NonceSize() int {
	return ca.KeySize()
}

func (ca *cipherAEAD) Overhead() int {
	return ca.KeySize()
}

func (ca *cipherAEAD) Seal(dst, nonce, plaintext, data []byte) []byte {

	// Fork off a temporary Cipher state indexed by the nonce
	ct := ca.Clone()
	ct.Message(nil, nil, nonce)

	// Encrypt the plaintext and update the temporary Cipher state
	dst, ciphertext := bytes.Grow(dst, len(plaintext))
	ct.Message(ciphertext, plaintext, ciphertext)

	// Compute and append the authenticator based on post-encryption state
	dst, auth := bytes.Grow(dst, ct.KeySize())
	ct.Message(auth, nil, nil)

	return dst
}

func (ca *cipherAEAD) Open(dst, nonce, ciphertext, data []byte) ([]byte, error) {

	// Fork off a temporary Cipher state indexed via the nonce
	ct := ca.Clone()
	ct.Message(nil, nil, nonce)

	// Compute the plaintext's length
	authl := ct.KeySize()
	plainl := len(ciphertext) - authl
	if plainl < 0 {
		return nil, errors.New("AEAD ciphertext too short")
	}
	auth := ciphertext[plainl:]
	ciphertext = ciphertext[:plainl]

	// Decrypt the plaintext and update the temporary Cipher state
	dst, plaintext := bytes.Grow(dst, plainl)
	ct.Message(plaintext, ciphertext, ciphertext)

	// Compute and check the authenticator based on post-encryption state
	ct.Message(auth, auth, nil)
	if subtle.ConstantTimeAllEq(auth, 0) == 0 {
		return nil, errors.New("AEAD authenticator check failed")
	}

	return dst, nil
}
