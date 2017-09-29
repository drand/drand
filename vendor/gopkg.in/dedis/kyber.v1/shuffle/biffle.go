package shuffle

import (
	kyber "gopkg.in/dedis/kyber.v1"
	"gopkg.in/dedis/kyber.v1/proof"
	"gopkg.in/dedis/kyber.v1/util/random"
)

func bifflePred() proof.Predicate {

	// Branch 0 of either/or proof (for bit=0)
	rep000 := proof.Rep("Xbar0-X0", "beta0", "G")
	rep001 := proof.Rep("Ybar0-Y0", "beta0", "H")
	rep010 := proof.Rep("Xbar1-X1", "beta1", "G")
	rep011 := proof.Rep("Ybar1-Y1", "beta1", "H")

	// Branch 1 of either/or proof (for bit=1)
	rep100 := proof.Rep("Xbar0-X1", "beta1", "G")
	rep101 := proof.Rep("Ybar0-Y1", "beta1", "H")
	rep110 := proof.Rep("Xbar1-X0", "beta0", "G")
	rep111 := proof.Rep("Ybar1-Y0", "beta0", "H")

	and0 := proof.And(rep000, rep001, rep010, rep011)
	and1 := proof.And(rep100, rep101, rep110, rep111)

	or := proof.Or(and0, and1)
	return or
}

func bifflePoints(suite Suite, G, H kyber.Point,
	X, Y, Xbar, Ybar [2]kyber.Point) map[string]kyber.Point {

	return map[string]kyber.Point{
		"G":        G,
		"H":        H,
		"Xbar0-X0": suite.Point().Sub(Xbar[0], X[0]),
		"Ybar0-Y0": suite.Point().Sub(Ybar[0], Y[0]),
		"Xbar1-X1": suite.Point().Sub(Xbar[1], X[1]),
		"Ybar1-Y1": suite.Point().Sub(Ybar[1], Y[1]),
		"Xbar0-X1": suite.Point().Sub(Xbar[0], X[1]),
		"Ybar0-Y1": suite.Point().Sub(Ybar[0], Y[1]),
		"Xbar1-X0": suite.Point().Sub(Xbar[1], X[0]),
		"Ybar1-Y0": suite.Point().Sub(Ybar[1], Y[0])}
}

// Biffle is a binary shuffle ("biffle") for 2 ciphertexts based on general ZKPs.
func Biffle(suite Suite, G, H kyber.Point,
	X, Y [2]kyber.Point, rand kyber.Cipher) (
	Xbar, Ybar [2]kyber.Point, prover proof.Prover) {

	// Pick the single-bit permutation.
	bit := int(random.Byte(rand) & 1)

	// Pick a fresh ElGamal blinding factor for each pair
	var beta [2]kyber.Scalar
	for i := 0; i < 2; i++ {
		beta[i] = suite.Scalar().Pick(rand)
	}

	// Create the output pair vectors
	for i := 0; i < 2; i++ {
		piI := i ^ bit
		Xbar[i] = suite.Point().Mul(beta[piI], G)
		Xbar[i].Add(Xbar[i], X[piI])
		Ybar[i] = suite.Point().Mul(beta[piI], H)
		Ybar[i].Add(Ybar[i], Y[piI])
	}

	or := bifflePred()
	secrets := map[string]kyber.Scalar{
		"beta0": beta[0],
		"beta1": beta[1]}
	points := bifflePoints(suite, G, H, X, Y, Xbar, Ybar)
	choice := map[proof.Predicate]int{or: bit}
	prover = or.Prover(suite, secrets, points, choice)
	return
}

// BiffleVerifier returns a verifier of the biffle
func BiffleVerifier(suite Suite, G, H kyber.Point,
	X, Y, Xbar, Ybar [2]kyber.Point) (
	verifier proof.Verifier) {

	or := bifflePred()
	points := bifflePoints(suite, G, H, X, Y, Xbar, Ybar)
	return or.Verifier(suite, points)
}
