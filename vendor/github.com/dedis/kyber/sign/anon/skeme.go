package anon

import (
	"crypto/cipher"
	"crypto/hmac"
	"crypto/subtle"
	"errors"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/util/random"
)

// SKEME is a pairwise anonymous key agreement for point-to-point interactions.
// We use the encryption-based SKEME authenticated key exchange protocol,
// with the anonymity-set encryption, to authenticate a Diffie-Hellman secret.
// The result is a private two-party communication channel,
// where each party knows that the other is a member of a specific set,
// but does not know the other's price identity unless his set is of size one.
// Once we have performed this key agreement, we can use more efficient
// pairwise cryptographic primitives such as GCM authenticators,
// which are not directly usable in multiparty contexts.
//
type SKEME struct {
	suite    Suite
	hide     bool
	lpri     PriKey       // local private key
	rpub     Set          // remote public key
	lx       kyber.Scalar // local Diffie-Hellman private key
	lX, rX   kyber.Point  // local,remote Diffie-Hellman pubkeys
	lXb, rXb []byte       // local,remote DH pubkeys byte-encoded

	ms         kyber.Cipher  // master symmetric shared stream
	ls, rs     cipher.Stream // local->remote,remote->local streams
	lmac, rmac []byte        // local,remote key-confirmation MACs

	lm, rm   []byte // local,remote message strings
	lml, rml int    // local,remote message lengths
}

// Init the SKEME
func (sk *SKEME) Init(suite Suite, rand cipher.Stream,
	lpri PriKey, rpub Set, hide bool) {
	sk.suite = suite
	sk.hide = hide
	sk.lpri, sk.rpub = lpri, rpub

	// Create our Diffie-Hellman keypair
	sk.lx = suite.Scalar().Pick(rand)
	sk.lX = suite.Point().Mul(sk.lx, nil)
	sk.lXb, _ = sk.lX.MarshalBinary()

	// Encrypt and send the DH key to the receiver.
	// This is a deviation from SKEME, to protect message metadata
	// and further harden messages against tampering or active MITM DoS.
	sk.lm = Encrypt(suite, rand, sk.lXb, rpub, hide)
}

// ToSend returns the current message that should be sent (retransmitting if needed)
func (sk *SKEME) ToSend() []byte {
	return sk.lm
}

// Recv decrypts the message. It returns false if the SKEME expects more data,
// an error if any checks or decryption is invalid, and true otherwise.
func (sk *SKEME) Recv(rm []byte) (bool, error) {

	M, err := Decrypt(sk.suite, rm, sk.lpri.Set, sk.lpri.Mine, sk.lpri.Pri,
		sk.hide)
	if err != nil {
		return false, err
	}

	// Decode the remote DH public key
	ptlen := sk.suite.PointLen()
	if len(M) < ptlen {
		return false, errors.New("SKEME message too short for DH key")
	}
	if sk.rX == nil {
		rXb := M[:ptlen]
		rX := sk.suite.Point()
		if err := rX.UnmarshalBinary(M[:ptlen]); err != nil {
			return false, err
		}
		sk.rX = rX // remote DH public key
		sk.rXb = rXb

		// Compute the shared secret and the key-confirmation MACs
		DH := sk.suite.Point().Mul(sk.lx, rX)
		seed, _ := DH.MarshalBinary()
		sk.ms = sk.suite.Cipher(seed)
		mkey := random.Bytes(sk.ms.KeySize(), sk.ms)
		sk.ls, sk.lmac = sk.mkmac(mkey, sk.lXb, sk.rXb)
		sk.rs, sk.rmac = sk.mkmac(mkey, sk.rXb, sk.lXb)

		// Transmit our key-confirmation MAC with the next message
		sk.lm = append(sk.lm, sk.lmac...)
	}

	// Decode and check the remote key-confirmation MAC if present
	maclo := ptlen
	machi := maclo + sk.ms.KeySize()
	if len(M) < machi {
		return false, nil // not an error, just not done yet
	}
	if subtle.ConstantTimeCompare(M[maclo:machi], sk.rmac) == 0 {
		return false, errors.New("SKEME remote MAC check failed")
	}

	// Shared key confirmed, good to go!
	// (Although remote might still need our key confirmation.)
	return true, nil
}

func (sk *SKEME) mkmac(masterkey, Xb1, Xb2 []byte) (cipher.Stream, []byte) {
	keylen := sk.ms.KeySize()
	hmac := hmac.New(sk.suite.Hash, masterkey)
	_, _ = hmac.Write(Xb1)
	_, _ = hmac.Write(Xb2)
	key := hmac.Sum(nil)[:keylen]

	stream := sk.suite.Cipher(key)
	mac := random.Bytes(keylen, stream)
	return stream, mac
}
