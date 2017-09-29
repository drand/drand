package aes

/*	Do we want an experimental AES-CTR/GCM-based alternative
	to the generic HMAC composition?
	That's what this code wanted to be.

import (
	"hash"
	"bytes"
	"crypto/aes"
	"crypto/hmac"
	"crypto/sha256"
	"gopkg.in/dedis/kyber.v1
	"gopkg.in/dedis/kyber.v1/cipher"
	"gopkg.in/dedis/kyber.v1/cipher/generic"
)


// Secret state comprising an AES/SHA-based kyber.stateful cipher.
type state128 struct {
	suite128		// inherit methods from AES128 suite
	h [32]byte		// SHA256 hash-based cipher state
}

// AES128-CTR stream cipher keyed with the current kyber.cipher state
func (s *state128) stream() cipher.Stream {
	aes, err := aes.NewCipher(s.h[:16])
	if err != nil {
		panic("can't instantiate AES: " + err.Error())
	}
	iv := make([]byte,16)
	return cipher.NewCTR(aes,iv)
}

func (s *state128) Clone() State {
	return &state128{s.h}
}

func (s *state128) Crypt(dst,src,mix interface{}) (int,error) {
}

func (s *state128) Absorb(obj ...interface{}) {
	mac := hmac.New(sha256.New, st.h[:])
	generic.HashAbsorb(mac, obj)
	copy(st.h[:], mac.Sum(nil))
}

func (s *state128) Encrypt(w io.Writer, obj ...interface{}) error {

	// Serialize the source object(s) if necessary.
	var srcb,dstb []byte
	if len(obj) == 1 {
		if b,ok := src.([]byte); ok {
			srcb = b
			dstb = make([]byte, len(b))
		}
	}
	if srcb == nil {	// We need to serialize the src object(s)
		buf := bytes.Buffer{}
		if _,err := generic.Write(buf, obj...); err != nil {
			return err
		}
		srcb = buf.Bytes()
		dstb = srcb		// can encrypt in-place
	}

	// Encrypt the buffer.
	st := stream()
	st.XORKeyStream(dstb, srcb)

	// Absorb the ciphertext (standard "encrypt-then-MAC" practice)
	mac := hmac.New(sha256.New, st.h[:])
	mac.Write(dstb)
	copy(s.h[:], mac.Sum(nil))

	// Write encrypted stream to the output.
	if _,err := w.Write(buf); err != nil {
		return err
	}

	return nil
}

func (s *state128) Decrypt(r io.Reader, obj ...interface{}) error {

	// Read encrypted stream.

	// Decrypt the buffer.

}

func (s *state128) Random() cipher.Random {
	r := &generic.RandomStream{s.stream()}
	s.Absorb()	// Update our state
	return r
}

func (s *state128) Hash(dst []byte, mix ...interface{}) []byte {
}

*/
