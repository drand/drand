// +build experimental

/* This package implements the Deal cryptographic primitive, which is based
 * on poly/sharing.go
 *
 * Failures are frequent in large-scale systems. When reliability is paramount,
 * the system requires some guarentee that it will still be able to make
 * progress in the midst of failures. To do so, recovering critical information
 * from failed nodes is often needed. Herein is the importance of this package.
 * The deal package provides such reliability in the area of private keys.
 *
 * If a server wishes to have extra reliability for a private key it is using,
 * it can construct a Deal struct. A Deal will take the private key and
 * shard it into secret shares via poly/sharing.go logic. The server can
 * then give the shares to a group of other servers who can act as insurers
 * of the Deal. The insurers will keep the secret shares. If the original
 * server ever goes offline, another server could ask the insurers for their
 * secret shares and then combine them into the original secret key. Hence, this
 * server could continue in the place of the original and the sytem can continue
 * to make progress.
 *
 * This file provides structs for handling the cryptographic logic of this
 * process. Other files can use these primitives to build a more robust
 * system. In particular, there are 5 structs (3 public, 2 private):
 *
 *   1) Deal = respondible for sharding the secret, creating shares, and
 *                tracking which shares belong to which insurers
*
 *   2) State = responsible for keeping state about a given Deal such
 *              as shares recovered and messages that either certify the
 *              Deal or prove that it is malicious
 *
 *   3) signature = proves that an insurer has signed off on a Deal.
 *                  The signature could either be used to express
 *                  approval or disapproval
 *
 *   4) blameProof = provides proof that a given Deal share was malicously
 *                   constructed. A valid blameProof proves that the Deal
 *                   is untrustworthy and that the creator of the Deal is
 *                   malicious
 *
 *   5) Response = a union of signature and blameProof. This serves as a public
 *                 wrapper for the two structs.
 *
 * Further documentation for each of the different structs can be found below.
 * It is suggested to start with the Deal struct referring to the others
 * as necessary. Once a general knowledge of Deal is gained, the others
 * will make more sense.
 *
 * Code using this package will typically have the following flow (please see
 * "Key Terms" below for a definition of terms used):
 *
 * Step I: Take out the Deal
 *
 *   1) The Dealer constructs a new Deal and stores it within a State.
 *
 * Step II: Certify the Dealer
 *
 *   1) The Dealer sends the Deal to the insurers.
 *
 *   2) The insurers verify the Deal is well-formed and make sure that their
 *      secret shares are valid.
 *
 *     a) If a secret share is invalid, an insurer creates a blameProof and sends
 *        it back.
 *
 *     b) If the share is valid, an insurer creates a signature to send
 *        to the Dealer.
 *
 *   3) The Dealer receives the message from the insurer.
 *
 *     a) If it is a valid blameProof, the Dealer must start all over and
 *        construct a non-malicious Deal (or, the system can ban this malicious
 *        Dealer).
 *
 *     b) If the message is a signature, the Dealer can add the signature
 *        to its State.
 *
 *   4) Repeat steps 1-3 until the Dealer has collected enough
 *      signatures for the Deal to be certified.
 *
 * Step III: Distribute the Deal
 *
 *   1) Once the Deal is certified, the Dealer can then send the deal to
 *      clients.
 *
 *   2) Clients can then request the signatures from the insurers to make sure
 *      the Deal is indeed certified.
 *
 *     a) This prevents a malicious Dealer from simply leaving out valid
 *        blameProofs and only sending good signatures to the clients.
 *
 *   3) Once the client receives enough signatures, the client will then trust
 *      the Dealer to do work with the deald private key.
 *
 * Step IV: Perform work for Clients
 *
 * Step V: Reconstruct the Deal Secret (if the Dealer goes down)
 *
 *   1) If the Dealer is unresponsive for too long, a client can inform the
 *      insurers of the Deal.
 *
 *   2) The insurers can then check if the Dealer is indeed unresponsive.
 *
 *     a) If so, the insurer reveal its share and sends it to the client.
 *
 *   3) The client repeats steps 1-2 until enough shares are recovered to
 *      reconstruct the secret.
 *
 *   4) The client reconstructs the secret and takes over for the Dealer.
 *
 *
 *
 * Key Terms:
 *   Dealer = the server making a Deal
 *   client   = recipients of a Deal who are trusting the Dealer
 *   insurer  = servers who store secret shares of a deal. Such servers help
 *              during secret reconstruction.
 *
 *   Users of this code = programmers wishing to use this code in programs
*/
package poly

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"

	"github.com/dedis/kyber/abstract"
	"github.com/dedis/kyber/anon"
	"github.com/dedis/kyber/config"
	"github.com/dedis/kyber/proof"
	"github.com/dedis/kyber/random"
)

// Used mostly in marshalling code, this is the size of a uint32
var uint32Size int = binary.Size(uint32(0))

// This is the protocol name used by crypto/proof verifiers and provers.
var protocolName string = "Deal Protocol"

// These are messages used for signatures
var sigMsg []byte = []byte("Deal Signature")
var sigBlameMsg []byte = []byte("Deal Blame Signature")

// This error denotes that a share was maliciously constructed (fails
// the public polynomial check). Hence, the Dealer is malicious.
var maliciousShare = errors.New("Share is malicious. PubPoly.Check failed.")

/* Deal structs are mechanisms by which a server can deal other servers
 * that an abstract.Scalar will be availble even if the secret's owner goes
 * down. The secret to be deald will be sharded into shared secrets that can
 * be combined using Lagrange Interpolation to produce the original secret.
 * Other servers will act as insurers maintaining a share. If a client ever
 * needs the secret to be reconstructed, it can contact the insurers to regain
 * the shares and reconstruct the secret.
 *
 * The insurers and secrets arrays should remain synchronized. In other words,
 * insurers[i] and secrets[i] should both refer to the same server.
 *
 * Note to users of this code:
 *
 *   Here is a list of methods that should be called by each type of server:
 *
 * - Dealers
 *   * ConstructDeal
 *
 * - Insurers
 *   * ProduceResponse
 *   * State.RevealShare (public wrapper to Deal.RevealShare in State struct)
 *
 * - Clients
 *   * VerifyRevealedShare
 *
 * - All
 *   * UnmarshalInit
 *   * Id
 *   * DealerId
 *   * Insurers
 *   * State.DealCertified
 *   * State.SufficientSignatures
 */
type Deal struct {

	// The id of the deal used to differentiate it from others
	// The id is the short term public key of the private key being deald
	id abstract.Point

	// The cryptographic key suite used throughout the Deal.
	suite abstract.Suite

	// The minimum number of shares needed to reconstruct the secret
	t int

	// The minimum number of signatures approving the Deal that
	// are needed before the Deal is certified. t <= r <= n
	r int

	// The total number of shares
	n int

	// The long-term public key of the Dealer
	pubKey abstract.Point

	// The public polynomial that is used to verify the shared secrets
	pubPoly PubPoly

	// A list of servers who will act as insurers of the Deal. The list
	// contains the long-term public keys of the insurers
	insurers []abstract.Point

	// The list of shared secrets to be sent to the insurers. They are
	// encrypted with Diffie-Hellman shared secrets between the insurer
	// and the Dealer.
	secrets []abstract.Scalar
}

/* Constructs a new Deal to guarentee a secret.
 *
 * Arguments
 *    secretPair   = the keypair of the secret to be deald
 *    longPair     = the long term keypair of the Dealer
 *    t            = minimum number of shares needed to reconstruct the secret.
 *    r            = minimum signatures needed to certify the Deal
 *    insurers     = a list of the long-term public keys of the insurers.
 *
 *
 * It is expected that:
 *
 *    t <= r <= len(insurers)
 *
 *    secretPair.Suite == longPair.Suite
 *
 * Returns
 *   A newly constructed Deal
 */
func (p *Deal) ConstructDeal(secretPair *config.KeyPair,
	longPair *config.KeyPair, t, r int, insurers []abstract.Point) *Deal {
	p.id = secretPair.Public
	p.t = t
	p.r = r
	p.n = len(insurers)
	p.suite = secretPair.Suite
	p.pubKey = longPair.Public
	p.insurers = make([]abstract.Point, p.n, p.n)
	copy(p.insurers, insurers)
	p.secrets = make([]abstract.Scalar, p.n, p.n)

	// Verify that t <= r <= n
	if !(p.t <= r && p.r <= p.n) {
		panic("Invalid t, r, and n. Expected t <= r <= n")
	}

	if longPair.Suite != secretPair.Suite {
		panic("Two different suites used.")
	}

	// Create the public polynomial and private shares. The number of shares
	// should be equal to the number of insurers.
	pripoly := new(PriPoly).Pick(p.suite, p.t,
		secretPair.Secret, random.Stream)
	prishares := new(PriShares).Split(pripoly, p.n)
	p.pubPoly = PubPoly{}
	p.pubPoly.Commit(pripoly, nil)

	// Populate the secrets array with the shares encrypted by a Diffie-
	// Hellman shared secret between the Dealer and appropriate insurer
	for i := 0; i < p.n; i++ {
		diffieBase := p.suite.Point().Mul(insurers[i], longPair.Secret)
		diffieSecret := p.diffieHellmanSecret(diffieBase)
		p.secrets[i] = p.suite.Scalar().Add(prishares.Share(i),
			diffieSecret)
	}

	return p
}

/* Initializes a Deal for unmarshalling
 *
 * Arguments
 *    t           = the minimum number of shares needed to reconstruct the scalar
 *    r           = the minimum number of positive Response's needed to cerifty the
 *                  deal
 *    n           = the total number of insurers.
 *    suite       = the suite used within the Deal
 *
 * Returns
 *   An initialized Deal ready to be unmarshalled
 */
func (p *Deal) UnmarshalInit(t, r, n int, suite abstract.Suite) *Deal {
	p.t = t
	p.r = r
	p.n = n
	p.suite = suite
	p.pubPoly = PubPoly{}
	p.pubPoly.Init(p.suite, p.t, nil)
	return p
}

/* An internal helper used during unmarshalling, verifies that the Deal was
 * constructed correctly.
 *
 * Return
 *   an error if the deal is malformed, nil otherwise.
 *
 * TODO Consider more ways to verify (such as making sure there are no duplicate
 *      keys in p.insurers or that the Dealer's long term public key is not in
 *      p.insurers).
 * NOT TODO : Dealer long term public key COULD and most of the time WILL
 *		be in p.insurers  (you can insure yourself, there's no problem about that)
 */
func (p *Deal) verifyDeal() error {
	// Verify t <= r <= n
	if p.t > p.n || p.t > p.r || p.r > p.n {
		return errors.New("Invalid t-of-n shares Deal. Expected: t <= r <= n")
	}
	// There should be a scalar and public key for each of the n insurers.
	if len(p.insurers) != p.n || len(p.secrets) != p.n {
		return errors.New("Insurers and scalars array should be of length deal.n")
	}
	return nil
}

func (p *Deal) PubPoly() *PubPoly {
	return &p.pubPoly
}

// Returns the id of the Deal
func (p *Deal) Id() string {
	return p.id.String()
}

// Returns the id of the Dealer (aka its long term public key)
func (p *Deal) DealerId() string {
	return p.pubKey.String()
}

// Returns a copy of the Dealer's long term public key
func (p *Deal) DealerKey() abstract.Point {
	return p.suite.Point().Add(p.suite.Point().Null(), p.pubKey)
}

// Returns the list of insurers of the deal.
// A copy of insurers is return to prevent tampering.
func (p *Deal) Insurers() []abstract.Point {
	result := make([]abstract.Point, p.n, p.n)
	copy(result, p.insurers)
	return result
}

/* Given a Diffie-Hellman shared public key, produces a scalar to encrypt
 * another scalar
 *
 * Arguments
 *    diffieBase  = the DH shared public key
 *
 * Return
 *   the DH secret
 */
func (p *Deal) diffieHellmanSecret(diffieBase abstract.Point) abstract.Scalar {
	buff, err := diffieBase.MarshalBinary()
	if err != nil {
		panic("Bad shared secret for Diffie-Hellman given.")
	}
	cipher := p.suite.Cipher(buff)
	return p.suite.Scalar().Pick(cipher)
}

/* An internal helper function used by ProduceResponse, verifies that a share
 * has been properly constructed.
 *
 * Arguments
 *    i         = the index of the share to verify
 *    gKeyPair  = the long term key pair of the insurer of share i
 *
 * Return
 *  an error if the share is malformed, nil otherwise.
 */
func (p *Deal) verifyShare(i int, gKeyPair *config.KeyPair) error {
	if i < 0 || i >= p.n {
		return errors.New("Invalid index. Expected 0 <= i < n")
	}
	msg := "The long-term public key the Deal recorded as the insurer" +
		"of this shares differs from what is expected"
	if !p.insurers[i].Equal(gKeyPair.Public) {
		return errors.New(msg)
	}
	diffieBase := p.suite.Point().Mul(p.pubKey, gKeyPair.Secret)
	diffieSecret := p.diffieHellmanSecret(diffieBase)
	share := p.suite.Scalar().Sub(p.secrets[i], diffieSecret)
	if !p.pubPoly.Check(i, share) {
		return maliciousShare
	}
	return nil
}

/* An internal helper function responsible for producing signatures
 *
 * Arguments
 *    i         = the index of the insurer's share
 *    gKeyPair  = the long term public/private keypair of the insurer.
 *    msg       = the message to sign
 *
 * Return
 *   A signature object with the signature.
 */
func (p *Deal) sign(i int, gKeyPair *config.KeyPair, msg []byte) *signature {
	set := anon.Set{gKeyPair.Public}
	sig := anon.Sign(gKeyPair.Suite, random.Stream, msg, set, nil, 0,
		gKeyPair.Secret)
	return new(signature).init(gKeyPair.Suite, sig)
}

/* An internal helper function, verifies a signature is from a given insurer.
 *
 * Arguments
 *    i   = the index of the insurer in the insurers list
 *    sig = the signature object containing the signature
 *    msg = the message that was signed
 *
 * Return
 *   an error if the signature is malformed, nil otherwise.
 */
func (p *Deal) verifySignature(i int, sig *signature, msg []byte) error {
	if i < 0 || i >= p.n {
		return errors.New("Invalid index. Expected 0 <= i < n")
	}
	if sig.signature == nil {
		return errors.New("Nil signature")
	}
	set := anon.Set{p.insurers[i]}
	_, err := anon.Verify(sig.suite, msg, set, nil, sig.signature)
	return err
}

/* Create a blameProof that the Dealer maliciously constructed a shared secret.
 * This should be called if verifyShare fails due to the public polynomial
 * check failing. If it failed for other reasons (such as a bad index) it is not
 * advised to call this function since the share might actually be valid.
 *
 * Arguments
 *    i         = the index of the malicious shared secret
 *    gKeyPair  = the long term key pair of the insurer of share i
 *
 * Return
 *   A blameProof that the Dealer is malicious or nil if an error occurs
 *   An error object denoting the status of the blameProof construction
 *
 * TODO: Consider whether it is worthwile to produce some form of blame if
 *       the Dealer gives an invalid index.
 */
func (p *Deal) blame(i int, gKeyPair *config.KeyPair) (*blameProof, error) {
	diffieKey := p.suite.Point().Mul(p.pubKey, gKeyPair.Secret)
	insurerSig := p.sign(i, gKeyPair, sigBlameMsg)

	choice := make(map[proof.Predicate]int)
	pred := proof.Rep("D", "x", "P")
	choice[pred] = 1
	rand := p.suite.Cipher(abstract.RandomKey)
	sval := map[string]abstract.Scalar{"x": gKeyPair.Secret}
	pval := map[string]abstract.Point{"D": diffieKey, "P": p.pubKey}
	prover := pred.Prover(p.suite, sval, pval, choice)
	proof, err := proof.HashProve(p.suite, protocolName, rand, prover)
	if err != nil {
		return nil, err
	}
	return new(blameProof).init(p.suite, diffieKey, proof, insurerSig), nil
}

/* Verifies that a blameProof proves a share to be maliciously constructed.
 *
 * Arguments
 *    i     = the index of the share subject to blame
 *    proof = blameProof that alleges the Dealer to have constructed a bad share.
 *
 * Return
 *   an error if the blame is unjustified or nil if the blame is justified.
 */
func (p *Deal) verifyBlame(i int, bproof *blameProof) error {
	// Basic sanity checks
	if i < 0 || i >= p.n {
		return errors.New("Invalid index. Expected 0 <= i < n")
	}
	if err := p.verifySignature(i, &bproof.signature, sigBlameMsg); err != nil {
		return err
	}

	// Verify the Diffie-Hellman shared secret was constructed properly
	pval := map[string]abstract.Point{"D": bproof.diffieKey, "P": p.pubKey}
	pred := proof.Rep("D", "x", "P")
	verifier := pred.Verifier(p.suite, pval)
	err := proof.HashVerify(p.suite, protocolName, verifier,
		bproof.proof)
	if err != nil {
		return err
	}

	// Verify the share is bad.
	diffieSecret := p.diffieHellmanSecret(bproof.diffieKey)
	share := p.suite.Scalar().Sub(p.secrets[i], diffieSecret)
	if p.pubPoly.Check(i, share) {
		return errors.New("Unjustified blame. The share checks out okay.")
	}
	return nil
}

/* For insurers, produces a response to a Deal. If the insurer's share is
 * valid, the function returns a Response expressing the insurer's approval.
 * Otherwise, a Response with a blameProof blaming the Dealer is made.
 *
 * Arguments
 *    i        = the index of the insurer in the insurers list
 *    gkeypair = the long term public/private keypair of the insurer.
 *
 * Return
 *   the Response, or nil if there is an error.
 *   an error, nil otherwise.
 */
func (p *Deal) ProduceResponse(i int, gKeyPair *config.KeyPair) (*Response, error) {
	if err := p.verifyShare(i, gKeyPair); err != nil {
		// verifyShare may also fail because the index is invalid or
		// the insurer key is not the one expected. Do not produce a
		// blameProof in these cases, simply ignore the Deal till
		// the Dealer sends the valid index for this insurer.
		if err != maliciousShare {
			return nil, err
		}

		blameProof, err := p.blame(i, gKeyPair)
		if err != nil {
			return nil, err
		}
		return new(Response).constructBlameProofResponse(blameProof), nil
	}

	sig := p.sign(i, gKeyPair, sigMsg)
	return new(Response).constructSignatureResponse(sig), nil
}

/* An internal function, reveals the secret share that the insurer has been
 * protecting. The public version is State.RevealShare.
 *
 * Arguments
 *    i        = the index of the insurer
 *    gkeyPair = the long-term keypair of the insurer
 *
 * Return
 *   the revealed private share
 */
func (p *Deal) RevealShare(i int, gKeyPair *config.KeyPair) abstract.Scalar {
	diffieBase := p.suite.Point().Mul(p.pubKey, gKeyPair.Secret)
	diffieSecret := p.diffieHellmanSecret(diffieBase)
	share := p.suite.Scalar().Sub(p.secrets[i], diffieSecret)
	return share
}

/* Verify that a revealed share is properly formed. This should be called by
 *in clients or others who request an insurer to reveal its shared secret.
 *
 * Arguments
 *    i     = the index of the share
 *    share = the share to validate.
 *
 * Return
 *   Whether the secret is valid
 */
func (p *Deal) VerifyRevealedShare(i int, share abstract.Scalar) error {
	if i < 0 || i >= p.n {
		return errors.New("Invalid index. Expected 0 <= i < n")
	}
	if !p.pubPoly.Check(i, share) {
		return errors.New("The share failed the public polynomial check.")
	}
	return nil
}

/* Tests whether two Deal structs are equal
 *
 * Arguments
 *    p2 = a pointer to the struct to test for equality
 *
 * Returns
 *   true if equal, false otherwise
 */
func (p *Deal) Equal(p2 *Deal) bool {
	if p.n != p2.n {
		return false
	}
	if p.suite != nil && p2.suite != nil {
		if p.suite.String() != p2.suite.String() {
			fmt.Printf("Comparise with the suites failed\n")
			return false
		}
	} else {
		return false
	}

	for i := 0; i < p.n; i++ {
		if !p.secrets[i].Equal(p2.secrets[i]) ||
			!p.insurers[i].Equal(p2.insurers[i]) {
			return false
		}
	}
	return p.id.Equal(p2.id) && p.t == p2.t && p.r == p2.r &&
		p.pubKey.Equal(p2.pubKey) && p.pubPoly.Equal(&p2.pubPoly)
}

/* Returns the number of bytes used by this struct when marshalled
 *
 * Returns
 *   The marshal size
 *
 * Note
 *   This function can be used after UnmarshalInit
 */
func (p *Deal) MarshalSize() int {
	return 2*p.suite.PointLen() + p.pubPoly.MarshalSize() +
		p.n*p.suite.PointLen() + p.n*p.suite.ScalarLen()
}

/* Marshals a Deal struct into a byte array
 *
 * Returns
 *   A buffer of the marshalled struct
 *   The error status of the marshalling (nil if no error)
 *
 * Note
 *   The buffer is formatted as follows:
 *
 *      ||id||pubKey||pubPoly||==insurers_array==||==secrets==||
 *
 *   Remember: n == len(insurers) == len(secrets)
 */
func (p *Deal) MarshalBinary() ([]byte, error) {
	buf := make([]byte, p.MarshalSize())

	pointLen := p.suite.PointLen()
	polyLen := p.pubPoly.MarshalSize()
	secretLen := p.suite.ScalarLen()

	// Encode id, pubKey, and pubPoly
	idBuf, err := p.id.MarshalBinary()
	if err != nil {
		return nil, err
	}
	copy(buf, idBuf)

	pointBuf, err := p.pubKey.MarshalBinary()
	if err != nil {
		return nil, err
	}
	copy(buf[pointLen:], pointBuf)

	polyBuf, err := p.pubPoly.MarshalBinary()
	if err != nil {
		return nil, err
	}
	copy(buf[2*pointLen:], polyBuf)

	// Encode the insurers and secrets array (Based on poly/sharing.go code)
	bufPos := 2*pointLen + polyLen
	for i := range p.insurers {
		pb, err := p.insurers[i].MarshalBinary()
		if err != nil {
			return nil, err
		}
		copy(buf[bufPos+i*pointLen:], pb)
	}
	bufPos += p.n * pointLen

	for i := range p.secrets {
		pb, err := p.secrets[i].MarshalBinary()
		if err != nil {
			return nil, err
		}
		copy(buf[bufPos+i*secretLen:], pb)
	}
	return buf, nil
}

/* Unmarshals a Deal from a byte buffer
 *
 * Arguments
 *    buf = the buffer containing the Deal
 *
 * Returns
 *   The error status of the unmarshalling (nil if no error)
 */
func (p *Deal) UnmarshalBinary(buf []byte) error {
	pointLen := p.suite.PointLen()
	secretLen := p.suite.ScalarLen()

	bufPos := 0

	// Decode id, pubKey, and pubPoly
	p.id = p.suite.Point()
	if err := p.id.UnmarshalBinary(buf[bufPos : bufPos+pointLen]); err != nil {
		return err
	}
	bufPos += pointLen

	p.pubKey = p.suite.Point()
	if err := p.pubKey.UnmarshalBinary(buf[bufPos : bufPos+pointLen]); err != nil {
		return err
	}
	bufPos += pointLen

	polyLen := p.pubPoly.MarshalSize()
	if err := p.pubPoly.UnmarshalBinary(buf[bufPos : bufPos+polyLen]); err != nil {
		return err
	}
	bufPos += polyLen

	// Decode the insurers and secrets array (Based on poly/sharing.go code)
	p.insurers = make([]abstract.Point, p.n, p.n)
	for i := 0; i < p.n; i++ {
		start := bufPos + i*pointLen
		end := start + pointLen
		p.insurers[i] = p.suite.Point()
		if err := p.insurers[i].UnmarshalBinary(buf[start:end]); err != nil {
			return err
		}
	}
	bufPos += p.n * pointLen
	p.secrets = make([]abstract.Scalar, p.n, p.n)
	for i := 0; i < p.n; i++ {
		start := bufPos + i*secretLen
		end := start + secretLen
		p.secrets[i] = p.suite.Scalar()
		if err := p.secrets[i].UnmarshalBinary(buf[start:end]); err != nil {
			return err
		}
	}
	// Make sure the Deal is valid.
	return p.verifyDeal()
}

/* Marshals a Deal struct using an io.Writer
 *
 * Arguments
 *    w = the writer to use for marshalling
 *
 * Returns
 *   The number of bytes written
 *   The error status of the write (nil if no errors)
 */
func (p *Deal) MarshalTo(w io.Writer) (int, error) {
	buf, err := p.MarshalBinary()
	if err != nil {
		return 0, err
	}
	return w.Write(buf)
}

/* Unmarshals a Deal struct using an io.Reader
 *
 * Arguments
 *    r = the reader to use for unmarshalling
 *
 * Returns
 *   The number of bytes read
 *   The error status of the read (nil if no errors)
 */
func (p *Deal) UnmarshalFrom(r io.Reader) (int, error) {
	buf := make([]byte, p.MarshalSize())
	n, err := io.ReadFull(r, buf)
	if err != nil {
		return n, err
	}
	return n, p.UnmarshalBinary(buf)
}

/* Returns a string representation of the Deal for easy debugging
 *
 * Returns
 *   The Deal's string representation
 */
func (p *Deal) String() string {
	s := "{Deal:\n"
	s += "Suite => " + p.suite.String() + ",\n"
	s += "t => " + strconv.Itoa(p.t) + ",\n"
	s += "r => " + strconv.Itoa(p.r) + ",\n"
	s += "n => " + strconv.Itoa(p.n) + ",\n"
	s += "Public Key => " + p.pubKey.String() + ",\n"
	s += "Public Polynomial => " + p.pubPoly.String() + ",\n"
	insurers := ""
	secrets := ""
	for i := 0; i < p.n; i++ {
		insurers += p.insurers[i].String() + ",\n"
		secrets += p.secrets[i].String() + ",\n"
	}
	s += "Insurers =>\n[" + insurers + "],\n"
	s += "Secrets =>\n[" + secrets + "]\n"
	s += "}\n"
	return s
}

/* The State struct is responsible for maintaining state about Deal
 * structs. It consists of three main pieces:
 *
 *    1. The deal itself, which should be treated like an immutable object
 *    2. The shared secrets the server has recovered so far
 *    3. A list of responses from insurers cerifying or blaming the deal
 *
 * Each server should have one State per Deal
 *
 * Note to users of this code:
 *
 *    To add a share to PriShares, do:
 *
 *       p.PriShares.SetShare(index, share)
 *
 *    To reconstruct the secret, do:
 *
 *       p.PriShares.Secret()
 *
 *    Be warned that Secret will panic unless there are enough shares to
 *    reconstruct the secret. (See poly/sharing.go for more info)
 *
 * TODO Consider if it is worth adding a String function
 */
type State struct {

	// The actual deal
	Deal Deal

	// Primarily used by clients, contains shares the client has currently
	// obtained from insurers. This is what will be used to reconstruct the
	// deald secret.
	PriShares PriShares

	// A list of responses (either approving signatures or blameProofs)
	// that have been received so far.
	responses []*Response
}

/* Initializes a new State
 *
 * Arguments
 *    deal = the deal to keep track of
 *
 * Returns
 *   An initialized State
 */
func (ps *State) Init(deal Deal) *State {
	ps.Deal = deal

	// Initialize a new PriShares based on information from the deal.
	ps.PriShares = PriShares{}
	ps.PriShares.Empty(deal.suite, deal.t, deal.n)
	// There will be at most n responses, one per insurer
	ps.responses = make([]*Response, deal.n, deal.n)
	return ps
}

/* Adds a response from an insurer to the State. Checks to see whether the response
 * is valid and then adds it.
 *
 * Arguments
 *    i        = the index in the signature array this signature belongs
 *    response = the response to add
 *
 * Returns
 *   nil if the deal was added succesfully, an error otherwise.
 */
func (ps *State) AddResponse(i int, response *Response) error {
	if ps.responses[i] != nil {
		return errors.New("Response already added.")
	}

	var err error
	switch response.rtype {
	case signatureResponse:
		err = ps.Deal.verifySignature(i, response.signature, sigMsg)

	case blameProofResponse:
		err = ps.Deal.verifyBlame(i, response.blameProof)

	default:
		err = errors.New("Invalid response.")
	}
	if err != nil {
		return err
	}
	ps.responses[i] = response
	return nil
}

/* A public wrapper for Deal.RevealShare, ensures that a share is only
 * revealed for a Deal that has received a sufficient number of signatures.
 * An insurer should call this function on behalf of a client after verifying
 * that the Dealer is non-responsive.
 *
 * Arguments
 *    i        = the index of the insurer
 *    gkeyPair = the long-term keypair of the insurer
 *
 * Return
 *   (share, error)
 *      share = the revealed private share, or nil if the deal share is corrupted
 *      error = nil if successful, error if the deal share is corrupted
 *
 *   This error checking insures that a good insurer who has produced a valid blameproof does
 *   not reveal an incorrect share.
 *
 * Postcondition
 *   panics if an insufficient number of signatures have been received
 *
 *
 * Note
 *   The reason that SufficientSignatures is used instead of DealCertified is
 *   to prevent the following senario:
 *
 *      1) A malicious server creates a deal and selects as an insurer another
 *         malicious peer. The malicious peer is given an invalid share.
 *
 *      2) The other insurers certify the deal and the malicious insurer does
 *         not respond.
 *
 *      3) The malicious server enters the system and gives its deal to clients.
 *
 *      4) The malicious insurer then sends out the valid blameProof.
 *
 *      5) Now, the good insurers are unable to reveal the secret and reconstruct
 *         the deal.
 *
 *      6) The malicious server leaves the system. The insurance policy is now
 *         useless.
 *
 *   To prevent this, blameproofs are not taken into consideration. As a result,
 *   any server that produces an invalid share risks having its secret revealed
 *   at any moment after the deal has garnered enough signatures to be
 *   considered certified otherwise. This is further incentive to create valid deals.
 */
func (ps *State) RevealShare(i int, gKeyPair *config.KeyPair) (abstract.Scalar, error) {
	if ps.SufficientSignatures() != nil {
		panic("RevealShare should only be called with deals with enough signatures.")
	}
	share := ps.Deal.RevealShare(i, gKeyPair)
	if !ps.Deal.pubPoly.Check(i, share) {
		return nil, errors.New("This share is corrupted.")
	}
	return share, nil
}

/* Checks whether the Deal object has received enough signatures to be
 * considered certified.
 *
 * Arguments
 *   blameProofFail = whether to fail if a valid blame proof is encountered.
 *
 * Return
 *   an error denoting whether or not the Deal is certified.
 *     nil       == certified
 *     error     == not_yet_certified
 *
 * Note to users of this code
 *   An error here is not necessarily a cause for alarm, particularly if the
 *   Deal just needs more signatures. However, it could be a red flag if
 *   the error was caused by a valid blameProof. A single valid blameProof will
 *   permanently make a Deal uncertified.
 *
 * Technical Notes: The function goes through the list of responses and checks
 *                  for signatures that are properly signed. If at least r of
 *                  these are signed and r is greater than t (the minimum number
 *                  of shares needed to reconstruct the secret), the deal is
 *                  considered certified. If any valid blameProofs are found, an
 *                  error is immediately produced if blameProofFail is true.
 *                  Otherwise, it ignores blameProofs.
 *
 *                  AddResponse handles deal validation. Hence, it is assumed
 *                  any deals included within the response array are valid.
 */
func (ps *State) dealCertified(blameProofFail bool) error {
	if err := ps.Deal.verifyDeal(); err != nil {
		return err
	}
	validSigs := 0
	for i := 0; i < ps.Deal.n; i++ {
		if ps.responses[i] == nil {
			continue
		}

		if ps.responses[i].rtype == signatureResponse {
			validSigs += 1
		}

		if blameProofFail && ps.responses[i].rtype == blameProofResponse {
			return errors.New("A valid blameProof proves this Deal to be uncertified.")
		}
	}
	if validSigs < ps.Deal.r {
		return errors.New(fmt.Sprintf("Not enough signatures yet to be certified %d vs %d", validSigs, ps.Deal.r))
	}
	return nil
}

/* This public function checks whether the Deal is certified. Three things
 * must hold for this to be the case:
 *
 *   1) The deal must be syntatically valid.
 *   2) It must have >= r valid signatures
 *   3) It must not have any valid blameProofs
 *
 *
 * Use this function when determining whether a deal is safe to be accepted.
 *
 * Please see dealCertified for more details.
 */
func (ps *State) DealCertified() error {
	return ps.dealCertified(true)
}

/* This public function checks whether the State has received enough signatures
 * for a deal to be considered certified. It ignores any valid blame proofs.
 *
 * Use this function when determining whether it is safe to reveal a share.
 *
 * Please see dealCertified for more details.
 */
func (ps *State) SufficientSignatures() error {
	return ps.dealCertified(false)
}

/* The signature struct is used by insurers to express their approval
 * or disapproval of a given deal. After receiving a deal and verifying
 * that their shares are good, insurers can produce a signature to send back
 * to the Dealer. Alternatively, the insurers can produce a blameProof (see
 * below) and use the signature to certify that they authored the blame.
 *
 * In order for a Deal to be considered certified, a Dealer will need to
 * collect a certain amount of signatures from its insurers (please see the
 * Deal struct below for more details).
 *
 * Besides unmarshalling, users of this code do not need to worry about creating
 * a signature directly. Deal structs know how to generate signatures via
 * Deal.ProduceResponse
 */
type signature struct {

	// The suite used for signing
	suite abstract.Suite

	// The signature proving that the insurer either approves or disapproves
	// of a Deal struct
	signature []byte
}

/* An internal function, initializes a new signature
 *
 * Arguments
 *    suite = the signing suite
 *    sig   = the signature of approval
 *
 * Returns
 *   An initialized signature
 */
func (p *signature) init(suite abstract.Suite, sig []byte) *signature {
	p.suite = suite
	p.signature = sig
	return p
}

/* For users of this code, initializes a signature for unmarshalling
 *
 * Arguments
 *    suite = the signing suite
 *
 * Returns
 *   An initialized signature ready to unmarshal a buffer
 */
func (p *signature) UnmarshalInit(suite abstract.Suite) *signature {
	p.suite = suite
	return p
}

/* Tests whether two signature structs are equal
 *
 * Arguments
 *    p2 = a pointer to the struct to test for equality
 *
 * Returns
 *   true if equal, false otherwise
 */
func (p *signature) Equal(p2 *signature) bool {
	return p.suite == p2.suite && reflect.DeepEqual(p, p2)
}

/* Returns the number of bytes used by this struct when marshalled
 *
 * Returns
 *   The marshal size
 *
 * Note
 *   The function is only useful for a signature struct that has already
 *   been unmarshalled. Since signatures can be of variable length, the marshal
 *   size is not known before unmarshalling. Do not call before unmarshalling.
 */
func (p *signature) MarshalSize() int {
	return uint32Size + len(p.signature)
}

/* Marshals a signature struct into a byte array
 *
 * Returns
 *   A buffer of the marshalled struct
 *   The error status of the marshalling (nil if no error)
 *
 * Note
 *   The buffer is formatted as follows:
 *
 *      ||Signature_Length||==Signature_Array===||
 */
func (p *signature) MarshalBinary() ([]byte, error) {
	buf := make([]byte, p.MarshalSize())
	binary.LittleEndian.PutUint32(buf, uint32(len(p.signature)))
	copy(buf[uint32Size:], p.signature)
	return buf, nil
}

/* Unmarshals a signature from a byte buffer
 *
 * Arguments
 *    buf = the buffer containing the signature
 *
 * Returns
 *   The error status of the unmarshalling (nil if no error)
 */
func (p *signature) UnmarshalBinary(buf []byte) error {
	if len(buf) < uint32Size {
		return errors.New("Buffer size too small")
	}

	sigLen := int(binary.LittleEndian.Uint32(buf))
	if len(buf) < uint32Size+sigLen {
		return errors.New("Buffer size too small")
	}

	p.signature = buf[uint32Size : uint32Size+sigLen]
	return nil
}

/* Marshals a signature struct using an io.Writer
 *
 * Arguments
 *    w = the writer to use for marshalling
 *
 * Returns
 *   The number of bytes written
 *   The error status of the write (nil if no errors)
 */
func (p *signature) MarshalTo(w io.Writer) (int, error) {
	buf, err := p.MarshalBinary()
	if err != nil {
		return 0, err
	}
	return w.Write(buf)
}

/* Unmarshal a signature struct using an io.Reader
 *
 * Arguments
 *    r = the reader to use for unmarshalling
 *
 * Returns
 *   The number of bytes read
 *   The error status of the read (nil if no errors)
 */
func (p *signature) UnmarshalFrom(r io.Reader) (int, error) {
	// Retrieve the signature length from the reader
	buf := make([]byte, uint32Size)
	n, err := io.ReadFull(r, buf)
	if err != nil {
		return n, err
	}

	sigLen := int(binary.LittleEndian.Uint32(buf))

	// Calculate the length of the entire message and create the new buffer.
	finalBuf := make([]byte, uint32Size+sigLen)

	// Copy the old buffer into the new
	copy(finalBuf, buf)

	// Read the rest and unmarshal.
	m, err := io.ReadFull(r, finalBuf[n:])
	if err != nil {
		return n + m, err
	}
	return n + m, p.UnmarshalBinary(finalBuf)
}

/* Returns a string representation of the signature for easy debugging
 *
 * Returns
 *   The signature's string representation
 */
func (p *signature) String() string {
	s := "{signature:\n"
	s += "Suite => " + p.suite.String() + ",\n"
	s += "Signature => " + hex.EncodeToString(p.signature) + "\n"
	s += "}\n"
	return s
}

/* The blameProof struct provides an accountability measure. If a Dealer
 * decides to construct a faulty share, insurers can construct a blameProof
 * to show that the Dealer is malicious.
 *
 * The insurer provides the Diffie-Hellman shared secret with the Dealer so
 * that others can decode the share in question. A zero knowledge blameProof is
 * provided to prove that the shared secret was constructed properly. Lastly, a
 * signature is attached to prove that the insurer endorses the blame.
 * When other servers receive the blameProof, they can then verify whether the
 * Dealer is malicious or the insurer is falsely accusing the Dealer.
 *
 * To quickly summarize the blame procedure, the following must hold for the
 * blame to succeed:
 *
 *   1. The signature must be valid
 *
 *   2. The Diffie-Hellman key must be verified to be correct
 *
 *   3. The insurer's share when decrypted must fail the PubPoly.Check of
 *   the Deal struct
 *
 * If all hold, the Dealer is proven malicious. Otherwise, the insurer is
 * slanderous.
 *
 * Beyond unmarshalling, users of this code need not worry about constructing a
 * blameProof struct themselves. The Deal struct knows how to create a
 * blameProof via the Deal.ProduceResponse method.
 */
type blameProof struct {

	// The suite used throughout the blameProof
	suite abstract.Suite

	// The Diffie-Hellman shared secret between the insurer and Dealer
	diffieKey abstract.Point

	// A HashProve proof that the insurer properly constructed the Diffie-
	// Hellman shared secret
	proof []byte

	// The signature denoting that the insurer approves of the blame
	signature signature
}

/* An internal function, initializes a new blameProof struct
 *
 * Arguments
 *    suite = the suite used for the Diffie-Hellman key, blameProof, and signature
 *    key   = the shared Diffie-Hellman key
 *    dkp   = the blameProof validating the Diffie-Hellman key
 *    sig   = the insurer's signature
 *
 * Returns
 *   An initialized blameProof
 */
func (bp *blameProof) init(suite abstract.Suite, key abstract.Point,
	dkp []byte, sig *signature) *blameProof {
	bp.suite = suite
	bp.diffieKey = key
	bp.proof = dkp
	bp.signature = *sig
	return bp
}

/* Initializes a blameProof struct for unmarshalling
 *
 * Arguments
 *    s = the suite used for the Diffie-Hellman key, blameProof, and signature
 *
 * Returns
 *   An initialized blameProof ready to be unmarshalled
 */
func (bp *blameProof) UnmarshalInit(suite abstract.Suite) *blameProof {
	bp.suite = suite
	return bp
}

/* Tests whether two blameProof structs are equal
 *
 * Arguments
 *    bp2 = a pointer to the struct to test for equality
 *
 * Returns
 *   true if equal, false otherwise
 */
func (bp *blameProof) Equal(bp2 *blameProof) bool {
	return bp.suite == bp2.suite &&
		bp.diffieKey.Equal(bp2.diffieKey) &&
		reflect.DeepEqual(bp.proof, bp2.proof) &&
		bp.signature.Equal(&bp2.signature)
}

/* Returns the number of bytes used by this struct when marshalled
 *
 * Returns
 *   The marshal size
 *
 * Note
 *   Since signature structs and the Diffie-Hellman blameProof can be of
 *   variable length, this function is only useful for a blameProof that is
 *   already unmarshalled. Do not call before unmarshalling.
 */
func (bp *blameProof) MarshalSize() int {
	return 2*uint32Size + bp.suite.PointLen() + len(bp.proof) +
		bp.signature.MarshalSize()
}

/* Marshals a blameProof struct into a byte array
 *
 * Returns
 *   A buffer of the marshalled struct
 *   The error status of the marshalling (nil if no error)
 *
 * Note
 *   The buffer is formatted as follows:
 *
 *   ||Diffie_Key_blameProof_Length||signature_Length||Diffie_Key||
 *      Diffie_Key_blameProof||signature||
 */
func (bp *blameProof) MarshalBinary() ([]byte, error) {
	pointLen := bp.suite.PointLen()
	proofLen := len(bp.proof)
	buf := make([]byte, bp.MarshalSize())

	binary.LittleEndian.PutUint32(buf, uint32(proofLen))
	binary.LittleEndian.PutUint32(buf[uint32Size:],
		uint32(bp.signature.MarshalSize()))

	pointBuf, err := bp.diffieKey.MarshalBinary()
	if err != nil {
		return nil, err
	}
	copy(buf[2*uint32Size:], pointBuf)
	copy(buf[2*uint32Size+pointLen:], bp.proof)

	sigBuf, err := bp.signature.MarshalBinary()
	if err != nil {
		return nil, err
	}
	copy(buf[2*uint32Size+pointLen+proofLen:], sigBuf)
	return buf, nil
}

/* Unmarshals a blameProof from a byte buffer
 *
 * Arguments
 *    buf = the buffer containing the blameProof
 *
 * Returns
 *   The error status of the unmarshalling (nil if no error)
 */
func (bp *blameProof) UnmarshalBinary(buf []byte) error {
	// Verify the buffer is large enough for the diffie proof length
	// (uint32), the signature length (uint32), and the
	// Diffie-Hellman shared secret (abstract.Point)
	pointLen := bp.suite.PointLen()
	if len(buf) < 2*uint32Size+pointLen {
		return errors.New("Buffer size too small")
	}
	proofLen := int(binary.LittleEndian.Uint32(buf))
	sigLen := int(binary.LittleEndian.Uint32(buf[uint32Size:]))

	bufPos := 2 * uint32Size
	bp.diffieKey = bp.suite.Point()
	if err := bp.diffieKey.UnmarshalBinary(buf[bufPos : bufPos+pointLen]); err != nil {
		return err
	}
	bufPos += pointLen

	if len(buf) < 2*uint32Size+pointLen+proofLen+sigLen {
		return errors.New("Buffer size too small")
	}
	bp.proof = make([]byte, proofLen, proofLen)
	copy(bp.proof, buf[bufPos:bufPos+proofLen])
	bufPos += proofLen

	bp.signature = signature{}
	bp.signature.UnmarshalInit(bp.suite)
	if err := bp.signature.UnmarshalBinary(buf[bufPos : bufPos+sigLen]); err != nil {
		return err
	}
	return nil
}

/* Marshals a blameProof struct using an io.Writer
 *
 * Arguments
 *    w = the writer to use for marshalling
 *
 * Returns
 *   The number of bytes written
 *   The error status of the write (nil if no errors)
 */
func (bp *blameProof) MarshalTo(w io.Writer) (int, error) {
	buf, err := bp.MarshalBinary()
	if err != nil {
		return 0, err
	}
	return w.Write(buf)
}

/* Unmarshals a blameProof struct using an io.Reader
 *
 * Arguments
 *    r = the reader to use for unmarshalling
 *
 * Returns
 *   The number of bytes read
 *   The error status of the read (nil if no errors)
 */
func (bp *blameProof) UnmarshalFrom(r io.Reader) (int, error) {
	// Retrieve the proof length and signature length from the reader
	buf := make([]byte, 2*uint32Size)
	n, err := io.ReadFull(r, buf)
	if err != nil {
		return n, err
	}
	pointLen := bp.suite.PointLen()
	proofLen := int(binary.LittleEndian.Uint32(buf))
	sigLen := int(binary.LittleEndian.Uint32(buf[uint32Size:]))

	// Calculate the final buffer, copy the old data to it, and fill it
	// for unmarshalling
	finalLen := 2*uint32Size + pointLen + proofLen + sigLen
	finalBuf := make([]byte, finalLen)
	copy(finalBuf, buf)
	m, err := io.ReadFull(r, finalBuf[n:])
	if err != nil {
		return n + m, err
	}
	return n + m, bp.UnmarshalBinary(finalBuf)
}

/* Returns a string representation of the blameProof for easy debugging
 *
 * Returns
 *   The blameProof's string representation
 */
func (bp *blameProof) String() string {
	proofHex := hex.EncodeToString(bp.proof)
	s := "{blameProof:\n"
	s += "Suite => " + bp.suite.String() + ",\n"
	s += "Diffie-Hellman Shared Secret => " + bp.diffieKey.String() + ",\n"
	s += "Diffie-Hellman blameProof => " + proofHex + ",\n"
	s += "signature => " + bp.signature.String() + "\n"
	s += "}\n"
	return s
}

// Used within a Response struct, it denotes the type of message to be sent.
type responseType int

const (
	// errorResponse means an invalid responseType. Named this to
	// distinguish it form the type error.
	errorResponse responseType = iota

	// Denotes that the Response contains a signature
	signatureResponse

	// Denotes that the Response contains a blameProof
	blameProofResponse
)

/* The Response struct is a union of the signature and blameProof types.
 * It is the public-facing message that insurers send in response to a Deal.
 * It hides the details of signature's and blameProofs so that users of
 * this code will not have to worry about them.
 *
 * Please see the signature and blameProof structs for more details.
 */
type Response struct {

	// The type of response
	rtype responseType

	// For unmarshalling purposes, the suite of the signature or blameProof
	suite abstract.Suite

	// A signature proving that the insurer approves of a Deal
	signature *signature

	// blameProof showing that the Deal has been badly constructed.
	blameProof *blameProof
}

/* An internal function, constructs a new Response with a signature
 *
 * Arguments
 *    sig = the signature to send.
 *
 * Returns
 *   An initialized Response
 */
func (r *Response) constructSignatureResponse(sig *signature) *Response {
	r.rtype = signatureResponse
	r.signature = sig
	return r
}

/* An internal function, constructs a new Response with a blameProof
 *
 * Arguments
 *    sig = the blameProof to send.
 *
 * Returns
 *   An initialized Response
 */
func (r *Response) constructBlameProofResponse(blameProof *blameProof) *Response {
	r.rtype = blameProofResponse
	r.blameProof = blameProof
	return r
}

/* Initializes a Response struct for unmarshalling
 *
 * Arguments
 *    suite = the suite used for the signature or blameProof
 *
 * Returns
 *   An initialized Response ready to be unmarshalled
 */
func (r *Response) UnmarshalInit(suite abstract.Suite) *Response {
	r.suite = suite
	return r
}

/* Tests whether two Response structs are equal
 *
 * Arguments
 *    r2 = a pointer to the struct to test for equality
 *
 * Returns
 *   true if equal, false otherwise
 */
func (r *Response) Equal(r2 *Response) bool {
	if r.rtype == errorResponse {
		panic("Response not initialized")
	}

	if r.rtype != r2.rtype {
		return false
	}
	if r.rtype == signatureResponse {
		return r.signature.Equal(r2.signature)
	}
	// r.rtype == blameProofSignature
	return r.blameProof.Equal(r2.blameProof)
}

/* Returns the number of bytes used by this struct when marshalled
 *
 * Returns
 *   The marshal size
 *
 * Note
 *   Since signature structs and blameProof structs can be of
 *   variable length, this function is only useful for a Response that is
 *   already unmarshalled. Do not call before unmarshalling.
 */
func (r *Response) MarshalSize() int {
	if r.rtype == errorResponse {
		panic("Response not initialized")
	}

	if r.rtype == signatureResponse {
		return 2*uint32Size + r.signature.MarshalSize()
	}
	//r.rtype == blameProofSignature
	return 2*uint32Size + r.blameProof.MarshalSize()
}

/* Marshals a Response struct into a byte array
 *
 * Returns
 *   A buffer of the marshalled struct
 *   The error status of the marshalling (nil if no error)
 *
 * Note
 *   The buffer is formatted as follows:
 *
 *   ||signature_Or_blameProof_Length||Type||signature_or_blameProof||
 */
func (r *Response) MarshalBinary() ([]byte, error) {
	if r.rtype == errorResponse {
		panic("Response not initialized")
	}

	var msgLen int
	buf := make([]byte, r.MarshalSize())

	if r.rtype == signatureResponse {
		msgLen = r.signature.MarshalSize()
	} else { //r.rtype == blameProofResponse
		msgLen = r.blameProof.MarshalSize()
	}

	binary.LittleEndian.PutUint32(buf, uint32(msgLen))
	binary.LittleEndian.PutUint32(buf[uint32Size:], uint32(r.rtype))

	var msgBuf []byte
	var err error

	if r.rtype == signatureResponse {
		msgBuf, err = r.signature.MarshalBinary()
	} else { //r.rtype == blameProofResponse
		msgBuf, err = r.blameProof.MarshalBinary()
	}

	if err != nil {
		return nil, err
	}
	copy(buf[2*uint32Size:], msgBuf)
	return buf, nil
}

/* Unmarshals a Response from a byte buffer
 *
 * Arguments
 *    buf = the buffer containing the blameProof
 *
 * Returns
 *   The error status of the unmarshalling (nil if no error)
 */
func (r *Response) UnmarshalBinary(buf []byte) error {
	// Verify the buffer is large enough for the length of the
	// signature/blameProof as well as the type of message.
	if len(buf) < 2*uint32Size {
		return errors.New("Buffer size too small")
	}
	msgLen := int(binary.LittleEndian.Uint32(buf))
	r.rtype = responseType(binary.LittleEndian.Uint32(buf[uint32Size:]))

	if len(buf) < 2*uint32Size+msgLen {
		return errors.New("Buffer size too small")
	}

	var err error
	bufPos := 2 * uint32Size

	if r.rtype == errorResponse {
		return errors.New("Uninitialized reponse sent")
	}

	if r.rtype == signatureResponse {
		r.signature = new(signature).UnmarshalInit(r.suite)
		err = r.signature.UnmarshalBinary(buf[bufPos : bufPos+msgLen])
	} else { // r.rtype == blameProofResponse
		r.blameProof = new(blameProof).UnmarshalInit(r.suite)
		err = r.blameProof.UnmarshalBinary(buf[bufPos : bufPos+msgLen])
	}

	if err != nil {
		return err
	}
	return nil
}

/* Marshals a Response struct using an io.Writer
 *
 * Arguments
 *    w = the writer to use for marshalling
 *
 * Returns
 *   The number of bytes written
 *   The error status of the write (nil if no errors)
 */
func (r *Response) MarshalTo(w io.Writer) (int, error) {
	buf, err := r.MarshalBinary()
	if err != nil {
		return 0, err
	}
	return w.Write(buf)
}

/* Unmarshals a Response struct using an io.Reader
 *
 * Arguments
 *    r = the reader to use for unmarshalling
 *
 * Returns
 *   The number of bytes read
 *   The error status of the read (nil if no errors)
 */
func (rp *Response) UnmarshalFrom(r io.Reader) (int, error) {
	// Retrieve the length of the signature/blameProof
	buf := make([]byte, uint32Size)
	n, err := io.ReadFull(r, buf)
	if err != nil {
		return n, err
	}
	msgLen := int(binary.LittleEndian.Uint32(buf))

	// Calculate the final buffer, copy the old data to it, and fill it
	// for unmarshalling
	finalLen := 2*uint32Size + msgLen
	finalBuf := make([]byte, finalLen)
	copy(finalBuf, buf)
	m, err := io.ReadFull(r, finalBuf[n:])
	if err != nil {
		return n + m, err
	}
	return n + m, rp.UnmarshalBinary(finalBuf)
}

/* Returns a string representation of the Response for easy debugging
 *
 * Returns
 *   The Response's string representation
 */
func (r Response) String() string {
	s := "{Response:\n"
	s += "ResponseType => " + strconv.Itoa(int(r.rtype)) + ",\n"

	if r.rtype == signatureResponse {
		s += "signature => " + r.signature.String() + ",\n"
	}
	if r.rtype == blameProofResponse {
		s += "blameProof => " + r.blameProof.String() + ",\n"
	}
	s += "}\n"
	return s
}
