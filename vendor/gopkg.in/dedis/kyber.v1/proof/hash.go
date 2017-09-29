package proof

import (
	"bytes"

	"gopkg.in/dedis/kyber.v1"
)

// Hash-based noninteractive Sigma-protocol prover context
type hashProver struct {
	suite   Suite
	proof   bytes.Buffer
	msg     bytes.Buffer
	pubrand kyber.Cipher
	prirand kyber.Cipher
}

func newHashProver(suite Suite, protoName string,
	rand kyber.Cipher) *hashProver {
	var sc hashProver
	sc.suite = suite
	sc.pubrand = suite.Cipher([]byte(protoName))
	sc.prirand = rand
	return &sc
}

func (c *hashProver) Put(message interface{}) error {
	return c.suite.Write(&c.msg, message)
}

func (c *hashProver) consumeMsg() {
	if c.msg.Len() > 0 {

		// Stir the message into the public randomness pool
		buf := c.msg.Bytes()
		c.pubrand.Message(nil, nil, buf)

		// Append the current message data to the proof
		c.proof.Write(buf)
		c.msg.Reset()
	}
}

// Get public randomness that depends on every bit in the proof so far.
func (c *hashProver) PubRand(data ...interface{}) error {
	c.consumeMsg()
	return c.suite.Read(c.pubrand, data...)
}

// Get private randomness
func (c *hashProver) PriRand(data ...interface{}) {
	if err := c.suite.Read(c.prirand, data...); err != nil {
		panic("error reading random stream: " + err.Error())
	}
}

// Obtain the encoded proof once the Sigma protocol is complete.
func (c *hashProver) Proof() []byte {
	c.consumeMsg()
	return c.proof.Bytes()
}

// Noninteractive Sigma-protocol verifier context
type hashVerifier struct {
	suite   Suite
	proof   bytes.Buffer // Buffer with which to read the proof
	prbuf   []byte       // Byte-slice underlying proof buffer
	pubrand kyber.Cipher
}

func newHashVerifier(suite Suite, protoName string,
	proof []byte) *hashVerifier {
	var c hashVerifier
	if _, err := c.proof.Write(proof); err != nil {
		panic("Buffer.Write failed")
	}
	c.suite = suite
	c.prbuf = c.proof.Bytes()
	c.pubrand = suite.Cipher([]byte(protoName))
	return &c
}

func (c *hashVerifier) consumeMsg() {
	l := len(c.prbuf) - c.proof.Len() // How many bytes read?
	if l > 0 {
		// Stir consumed bytes into the public randomness pool
		buf := c.prbuf[:l]
		c.pubrand.Message(nil, nil, buf)

		c.prbuf = c.proof.Bytes() // Reset to remaining bytes
	}
}

// Read structured data from the proof
func (c *hashVerifier) Get(message interface{}) error {
	return c.suite.Read(&c.proof, message)
}

// Get public randomness that depends on every bit in the proof so far.
func (c *hashVerifier) PubRand(data ...interface{}) error {
	c.consumeMsg() // Stir in newly-read data
	return c.suite.Read(c.pubrand, data...)
}

// HashProve runs a given Sigma-protocol prover with a ProverContext
// that produces a non-interactive proof via the Fiat-Shamir heuristic.
// Returns a byte-slice containing the noninteractive proof on success,
// or an error in the case of failure.
//
// The optional protocolName is fed into the hash function used in the proof,
// so that a proof generated for a particular protocolName
// will verify successfully only if the verifier uses the same protocolName.
//
// The caller must provide a source of random entropy for the proof;
// this can be random.Stream to use fresh random bits,
// or a pseudorandom stream based on a secret seed
// to create deterministically reproducible proofs.
//
func HashProve(suite Suite, protocolName string,
	random kyber.Cipher, prover Prover) ([]byte, error) {
	ctx := newHashProver(suite, protocolName, random)
	if e := (func(ProverContext) error)(prover)(ctx); e != nil {
		return nil, e
	}
	return ctx.Proof(), nil
}

// HashVerify computes a hash-based noninteractive proof generated with HashProve.
// The suite and protocolName must be the same as those given to HashProve.
// Returns nil if the proof checks out, or an error on any failure.
func HashVerify(suite Suite, protocolName string,
	verifier Verifier, proof []byte) error {
	ctx := newHashVerifier(suite, protocolName, proof)
	return (func(VerifierContext) error)(verifier)(ctx)
}
