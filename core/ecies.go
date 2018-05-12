package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"hash"
	"io"

	"github.com/dedis/drand/protobuf/crypto"
	"github.com/dedis/drand/protobuf/drand"
	"github.com/dedis/kyber"
	"github.com/dedis/kyber/util/random"
	"golang.org/x/crypto/hkdf"
)

// This file provides an implementation of the ECIES scheme.

var DefaultHash = sha256.New

// Encrypts performs a ephemereal-static  DH exchange, creates the shared key
// from it using a KDF scheme (hkdf from Go at the time of writing) and then
// computes the ciphertext using a AEAD scheme (AES-GCM from Go at the time of
// writing). This methods returns the ephemeral point of the DH exchange, the
// ciphertext and the associated nonce. It returns an error if something went
// wrong during the encryption.
func Encrypt(g kyber.Group, fn func() hash.Hash, public kyber.Point, msg []byte) (*drand.ECIESObject, error) {
	// generate an ephemeral key pair and performs the DH
	r := g.Scalar().Pick(random.New())
	R := g.Point().Mul(r, nil)
	eph := R

	ephProto, err := crypto.KyberToProtoPoint(eph)
	if err != nil {
		return nil, err
	}
	dh := g.Point().Mul(r, public)
	dhBuff, err := dh.MarshalBinary()
	if err != nil {
		return nil, err
	}
	reader := hkdf.New(fn, dhBuff, nil, nil)

	// derive key and encrypt with AES GCM
	byteLength := 32
	key := make([]byte, byteLength, byteLength)
	n, err := reader.Read(key)
	if err != nil {
		return nil, err
	} else if n != byteLength {
		return nil, errors.New("not enough bits from the shared secret")
	}

	// even though optional for this mode of ECIES, it _should_ not hurt if we
	// add it.
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ciphertext := aesgcm.Seal(nil, nonce, msg, nil)
	return &drand.ECIESObject{
		Ephemeral:  ephProto,
		Ciphertext: ciphertext,
		Nonce:      nonce,
	}, nil
}

// Decrypts does almost the same as Encrypt: the ephemereal static DH exchange,
// and the derivation of the symmetric key. It finally tries to decrypt the
// ciphertext and returns the plaintext if successful, an error otherwise.
func Decrypt(g kyber.Group, fn func() hash.Hash, priv kyber.Scalar, o *drand.ECIESObject) ([]byte, error) {
	eph, err := crypto.ProtoToKyberPoint(o.GetEphemeral())
	if err != nil {
		return nil, err
	}
	dh := g.Point().Mul(priv, eph)
	dhBuff, err := dh.MarshalBinary()
	if err != nil {
		return nil, err
	}

	reader := hkdf.New(fn, dhBuff, nil, nil)
	// derive key and encrypt with AES GCM
	byteLength := 32
	key := make([]byte, byteLength, byteLength)
	n, err := reader.Read(key)
	if err != nil {
		return nil, err
	} else if n != byteLength {
		return nil, errors.New("not enough bits from the shared secret")
	}

	// even though optional for this mode of ECIES, it _should_ not hurt if we
	// add it.
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aesgcm.Open(nil, o.Nonce, o.Ciphertext, nil)
}
