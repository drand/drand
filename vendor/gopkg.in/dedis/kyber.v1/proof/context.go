package proof

// Prover represents the prover role in an arbitrary Sigma-protocol.
// A prover is simply a higher-order function that takes a ProverContext,
// runs the protocol while making calls to the ProverContext methods as needed,
// and returns nil on success or an error once the protocol run concludes.
// The resulting proof is embodied in the interactions with the ProverContext,
// but HashProve() may be used to encode the proof into a non-interactive proof
// using a hash function via the Fiat-Shamir heuristic.
type Prover func(ctx ProverContext) error

// Verifier represents the verifier role in an arbitrary Sigma-protocol.
// A verifier is a higher-order function that takes a VerifierContext,
// runs the protocol while making calls to VerifierContext methods as needed,
// and returns nil on success or an error once the protocol run concludes.
type Verifier func(ctx VerifierContext) error

// ProverContext represents the kyber.environment
// required by the prover in a Sigma protocol.
//
// In a basic 3-step Sigma protocol such as a standard digital signature,
// the prover first calls Put() one or more times
// to send commitment information to the verifier,
// then calls PubRand() to obtain a public random challenge from the verifier,
// and finally makes further calls to Put() to respond to the challenge.
//
// The prover may also call PriRand() at any time
// to obtain any private randomness needed in the proof.
// The prover should obtain secret randomness only from this source,
// so that the prover may be run deterministically if desired.
//
// More sophisticated Sigma protocols requiring more than 3 steps,
// such as the Neff shuffle, may also use this interface;
// in this case the prover simply calls PubRand() multiple times.
//
type ProverContext interface {
	Put(message interface{}) error        // Send message to verifier
	PubRand(message ...interface{}) error // Get public randomness
	PriRand(message ...interface{})       // Get private randomness
}

// VerifierContext represents the kyber.environment
// required by the verifier in a Sigma protocol.
//
// The verifier calls Get() to obtain the prover's message data,
// interspersed with calls to PubRand() to obtain challenge data.
// Note that the challenge itself comes from the VerifierContext,
// not from the verifier itself as in the traditional Sigma-protocol model.
// By separating challenge production from proof verification logic,
// we obtain the flexibility to use a single Verifier function
// in both non-interactive proofs (e.g., via HashProve)
// and in interactive proofs (e.g., via DeniableProver).
type VerifierContext interface {
	Get(message interface{}) error        // Receive message from prover
	PubRand(message ...interface{}) error // Get public randomness
}
