package shuffle

import (
	"crypto/cipher"
	"errors"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/proof"
)

// XX the Zs in front of some field names are a kludge to make them
// accessible via the reflection API,
// which refuses to touch unexported fields in a struct.

// P (Prover) step 0: public inputs to the simple k-shuffle.
type ssa0 struct {
	X []kyber.Point
	Y []kyber.Point
}

// V (Verifier) step 1: random challenge t
type ssa1 struct {
	Zt kyber.Scalar
}

// P step 2: Theta vectors
type ssa2 struct {
	Theta []kyber.Point
}

// V step 3: random challenge c
type ssa3 struct {
	Zc kyber.Scalar
}

// P step 4: alpha vector
type ssa4 struct {
	Zalpha []kyber.Scalar
}

// SimpleShuffle is the "Simple k-shuffle" defined in section 3 of
// Neff, "Verifiable Mixing (Shuffling) of ElGamal Pairs", 2004.
type SimpleShuffle struct {
	grp kyber.Group
	p0  ssa0
	v1  ssa1
	p2  ssa2
	v3  ssa3
	p4  ssa4
}

// Simple helper to compute G^{ab-cd} for Theta vector computation.
func thenc(grp kyber.Group, G kyber.Point,
	a, b, c, d kyber.Scalar) kyber.Point {

	var ab, cd kyber.Scalar
	if a != nil {
		ab = grp.Scalar().Mul(a, b)
	} else {
		ab = grp.Scalar().Zero()
	}
	if c != nil {
		if d != nil {
			cd = grp.Scalar().Mul(c, d)
		} else {
			cd = c
		}
	} else {
		cd = grp.Scalar().Zero()
	}
	return grp.Point().Mul(ab.Sub(ab, cd), G)
}

// Init initializes the simple shuffle with the given group and the k parameter
// from the paper.
func (ss *SimpleShuffle) Init(grp kyber.Group, k int) *SimpleShuffle {
	ss.grp = grp
	ss.p0.X = make([]kyber.Point, k)
	ss.p0.Y = make([]kyber.Point, k)
	ss.p2.Theta = make([]kyber.Point, 2*k)
	ss.p4.Zalpha = make([]kyber.Scalar, 2*k-1)
	return ss
}

// Prove the  "Simple k-shuffle" defined in section 3 of
// Neff, "Verifiable Mixing (Shuffling) of ElGamal Pairs", 2004.
// The Scalar vector y must be a permutation of Scalar vector x
// but with all elements multiplied by common Scalar gamma.
func (ss *SimpleShuffle) Prove(G kyber.Point, gamma kyber.Scalar,
	x, y []kyber.Scalar, rand cipher.Stream,
	ctx proof.ProverContext) error {

	grp := ss.grp

	k := len(x)
	if k <= 1 {
		panic("can't shuffle length 1 vector")
	}
	if k != len(y) {
		panic("mismatched vector lengths")
	}

	//	// Dump input vectors to show their correspondences
	//	for i := 0; i < k; i++ {
	//		println("x",grp.Scalar().Mul(gamma,x[i]).String())
	//	}
	//	for i := 0; i < k; i++ {
	//		println("y",y[i].String())
	//	}

	// Step 0: inputs
	for i := 0; i < k; i++ { // (4)
		ss.p0.X[i] = grp.Point().Mul(x[i], G)
		ss.p0.Y[i] = grp.Point().Mul(y[i], G)
	}
	if err := ctx.Put(ss.p0); err != nil {
		return err
	}

	// V step 1
	if err := ctx.PubRand(&ss.v1); err != nil {
		return err
	}
	t := ss.v1.Zt

	// P step 2
	gammaT := grp.Scalar().Mul(gamma, t)
	xhat := make([]kyber.Scalar, k)
	yhat := make([]kyber.Scalar, k)
	for i := 0; i < k; i++ { // (5) and (6) xhat,yhat vectors
		xhat[i] = grp.Scalar().Sub(x[i], t)
		yhat[i] = grp.Scalar().Sub(y[i], gammaT)
	}
	thlen := 2*k - 1 // (7) theta and Theta vectors
	theta := make([]kyber.Scalar, thlen)
	ctx.PriRand(theta)
	Theta := make([]kyber.Point, thlen+1)
	Theta[0] = thenc(grp, G, nil, nil, theta[0], yhat[0])
	for i := 1; i < k; i++ {
		Theta[i] = thenc(grp, G, theta[i-1], xhat[i],
			theta[i], yhat[i])
	}
	for i := k; i < thlen; i++ {
		Theta[i] = thenc(grp, G, theta[i-1], gamma,
			theta[i], nil)
	}
	Theta[thlen] = thenc(grp, G, theta[thlen-1], gamma, nil, nil)
	ss.p2.Theta = Theta
	if err := ctx.Put(ss.p2); err != nil {
		return err
	}

	// V step 3
	if err := ctx.PubRand(&ss.v3); err != nil {
		return err
	}
	c := ss.v3.Zc

	// P step 4
	alpha := make([]kyber.Scalar, thlen)
	runprod := grp.Scalar().Set(c)
	for i := 0; i < k; i++ { // (8)
		runprod.Mul(runprod, xhat[i])
		runprod.Div(runprod, yhat[i])
		alpha[i] = grp.Scalar().Add(theta[i], runprod)
	}
	gammainv := grp.Scalar().Inv(gamma)
	rungamma := grp.Scalar().Set(c)
	for i := 1; i < k; i++ {
		rungamma.Mul(rungamma, gammainv)
		alpha[thlen-i] = grp.Scalar().Add(theta[thlen-i], rungamma)
	}
	ss.p4.Zalpha = alpha
	return ctx.Put(ss.p4)
}

// Simple helper to verify Theta elements,
// by checking whether A^a*B^-b = T.
// P,Q,s are simply "scratch" kyber.Point/Scalars reused for efficiency.
func thver(A, B, T, P, Q kyber.Point, a, b, s kyber.Scalar) bool {
	P.Mul(a, A)
	Q.Mul(s.Neg(b), B)
	P.Add(P, Q)
	return P.Equal(T)
}

// Verify for Neff simple k-shuffle proofs.
func (ss *SimpleShuffle) Verify(G, Gamma kyber.Point,
	ctx proof.VerifierContext) error {

	grp := ss.grp

	// extract proof transcript
	X := ss.p0.X
	Y := ss.p0.Y
	Theta := ss.p2.Theta
	alpha := ss.p4.Zalpha

	// Validate all vector lengths
	k := len(Y)
	thlen := 2*k - 1
	if k <= 1 || len(Y) != k || len(Theta) != thlen+1 ||
		len(alpha) != thlen {
		return errors.New("malformed SimpleShuffleProof")
	}

	// check verifiable challenges (usually by reproducing a hash)
	if err := ctx.Get(ss.p0); err != nil {
		return err
	}
	if err := ctx.PubRand(&ss.v1); err != nil { // fills in v1
		return err
	}
	t := ss.v1.Zt
	if err := ctx.Get(ss.p2); err != nil {
		return err
	}
	if err := ctx.PubRand(&ss.v3); err != nil { // fills in v3
		return err
	}
	c := ss.v3.Zc
	if err := ctx.Get(ss.p4); err != nil {
		return err
	}

	// Verifier step 5
	negt := grp.Scalar().Neg(t)
	U := grp.Point().Mul(negt, G)
	W := grp.Point().Mul(negt, Gamma)
	Xhat := make([]kyber.Point, k)
	Yhat := make([]kyber.Point, k)
	for i := 0; i < k; i++ {
		Xhat[i] = grp.Point().Add(X[i], U)
		Yhat[i] = grp.Point().Add(Y[i], W)
	}
	P := grp.Point() // scratch variables
	Q := grp.Point()
	s := grp.Scalar()
	good := true
	good = good && thver(Xhat[0], Yhat[0], Theta[0], P, Q, c, alpha[0], s)
	for i := 1; i < k; i++ {
		good = good && thver(Xhat[i], Yhat[i], Theta[i], P, Q,
			alpha[i-1], alpha[i], s)
	}
	for i := k; i < thlen; i++ {
		good = good && thver(Gamma, G, Theta[i], P, Q,
			alpha[i-1], alpha[i], s)
	}
	good = good && thver(Gamma, G, Theta[thlen], P, Q,
		alpha[thlen-1], c, s)
	if !good {
		return errors.New("incorrect SimpleShuffleProof")
	}

	return nil
}
