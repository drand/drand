// Package proof implements generic support for Sigma-protocols
// and discrete logarithm proofs in the Camenisch/Stadler framework.
// For the cryptographic foundations of this framework see
// "Proof Systems for General Statements about Discrete Logarithms" at
// ftp://ftp.inf.ethz.ch/pub/crypto/publications/CamSta97b.pdf.
package proof

import (
	"errors"

	"github.com/dedis/kyber"
)

// Suite defines the functionalities needed for this package to operate
// correctly. It provides a general abstraction to easily change the underlying
// implementations.
type Suite interface {
	kyber.Group
	kyber.HashFactory
	kyber.CipherFactory
	kyber.Encoding
}

/*
A Predicate is a composable logic expression in a knowledge proof system,
representing a "knowledge specification set" in Camenisch/Stadler terminology.
Atomic predicates in this system are statements of the form P=x1*B1+...+xn+Bn,
indicating the prover knows secrets x1,...,xn that make the statement true,
where P and B1,...,Bn are public points known to the verifier.
These atomic Rep (representation) predicates may be combined
with logical And and Or combinators to form composite statements.
Predicate objects, once created, are immutable and safe to share
or reuse for any number of proofs and verifications.

After constructing a Predicate using the Rep, And, and Or functions below,
the caller invokes Prover() to create a Sigma-protocol prover.
Prover() requires maps defining the values of both the Scalar variables
and the public Point variables that the Predicate refers to.
If the statement contains logical Or operators, the caller must also pass
a map containing branch choices for each Or predicate
in the "proof-obligated path" down through the Or predicates.
See the examples provded for the Or function for more details.

Similarly, the caller may invoke Verifier() to create
a Sigma-protocol verifier for the predicate.
The caller must pass a map defining the values
of the public Point variables that the proof refers to.
The verifier need not be provided any secrets or branch choices, of course.
(If the verifier needed those then they wouldn't be secret, would they?)

Currently we require that all Or operators be above all And operators
in the expression - i.e., Or-of-And combinations are allowed,
but no And-of-Or predicates.
We could rewrite expressions into this form as Camenisch/Stadler suggest,
but that could run a risk of unexpected exponential blowup in the worst case.
We could avoid this risk by not rewriting the expression tree,
but instead generating Pedersen commits for variables that need to "cross"
from one OR-domain to another non-mutually-exclusive one.
For now we simply require expressions to be in the appropriate form.
*/
type Predicate interface {

	// Create a Prover proving the statement this Predicate represents.
	Prover(suite Suite, secrets map[string]kyber.Scalar,
		points map[string]kyber.Point, choice map[Predicate]int) Prover

	// Create a Verifier for the statement this Predicate represents.
	Verifier(suite Suite, points map[string]kyber.Point) Verifier

	// Produce a human-readable string representation of the predicate.
	String() string

	// precedence-sensitive helper stringifier.
	precString(prec int) string

	// prover/verifier: enumerate the variables named in a predicate
	enumVars(prf *proof)

	// prover: recursively produce all commitments
	commit(prf *proof, w kyber.Scalar, v []kyber.Scalar) error

	// prover: given challenge, recursively produce all responses
	respond(prf *proof, c kyber.Scalar, r []kyber.Scalar) error

	// verifier: get all the commitments required in this predicate,
	// and fill the r slice with empty secrets for responses needed.
	getCommits(prf *proof, r []kyber.Scalar) error

	// verifier: check all commitments against challenges and responses
	verify(prf *proof, c kyber.Scalar, r []kyber.Scalar) error
}

// stringification precedence levels
const (
	precNone = iota
	precOr
	precAnd
	precAtom
)

// Internal prover/verifier state
type proof struct {
	s Suite

	nsvars     int            // number of Scalar variables
	npvars     int            // number of Point variables
	svar, pvar []string       // Scalar and Point variable names
	sidx, pidx map[string]int // Maps from strings to variable indexes

	pval map[string]kyber.Point // values of public Point variables

	// prover-specific state
	pc     ProverContext
	sval   map[string]kyber.Scalar   // values of private Scalar variables
	choice map[Predicate]int         // OR branch choices set by caller
	pp     map[Predicate]*proverPred // per-predicate prover state

	// verifier-specific state
	vc VerifierContext
	vp map[Predicate]*verifierPred // per-predicate verifier state
}
type proverPred struct {
	w  kyber.Scalar   // secret pre-challenge
	v  []kyber.Scalar // secret blinding factor for each variable
	wi []kyber.Scalar // OR predicates: individual sub-challenges
}
type verifierPred struct {
	V kyber.Point    // public commitment produced by verifier
	r []kyber.Scalar // per-variable responses produced by verifier
}

////////// Rep predicate //////////

// A term describes a point-multiplication term in a representation expression.
type term struct {
	S string // Scalar multiplier for this term
	B string // Generator for this term
}

type repPred struct {
	P string // Public point of which a representation is known
	T []term // Terms comprising the known representation
}

// Rep creates a predicate stating that the prover knows
// a representation of a point P with respect to
// one or more secrets and base point pairs.
//
// In its simplest usage, Rep indicates that the prover knows a secret x
// that is the (elliptic curve) discrete logarithm of a public point P
// with respect to a well-known base point B:
//
//	Rep(P,x,B)
//
// Rep can take any number of (Scalar,Base) variable name pairs, however.
// A Rep statement of the form Rep(P,x1,B1,...,xn,Bn)
// indicates that the prover knows secrets x1,...,xn
// such that point P is the sum x1*B1+...+xn*Bn.
//
func Rep(P string, SB ...string) Predicate {
	if len(SB)&1 != 0 {
		panic("mismatched Scalar")
	}
	t := make([]term, len(SB)/2)
	for i := range t {
		t[i].S = SB[i*2]
		t[i].B = SB[i*2+1]
	}
	return &repPred{P, t}
}

// Return a string representation of this proof-of-representation predicate,
// mainly for debugging.
func (rp *repPred) String() string {
	return rp.precString(precNone)
}

func (rp *repPred) precString(prec int) string {
	s := rp.P + "="
	for i := range rp.T {
		if i > 0 {
			s += "+"
		}
		t := &rp.T[i]
		s += t.S
		s += "*"
		s += t.B
	}
	return s
}

func (rp *repPred) enumVars(prf *proof) {
	prf.enumPointVar(rp.P)
	for i := range rp.T {
		prf.enumScalarVar(rp.T[i].S)
		prf.enumPointVar(rp.T[i].B)
	}
}

func (rp *repPred) commit(prf *proof, w kyber.Scalar, pv []kyber.Scalar) error {

	// Create per-predicate prover state
	v := prf.makeScalars(pv)
	pp := &proverPred{w, v, nil}
	prf.pp[rp] = pp

	// Compute commit V=wY+v1G1+...+vkGk
	V := prf.s.Point()
	if w != nil { // We're on a non-obligated branch
		V.Mul(w, prf.pval[rp.P])
	} else { // We're on a proof-obligated branch, so w=0
		V.Null()
	}
	P := prf.s.Point()
	for i := 0; i < len(rp.T); i++ {
		t := rp.T[i] // current term
		s := prf.sidx[t.S]

		// Choose a blinding secret the first time
		// we encounter each variable
		if v[s] == nil {
			v[s] = prf.s.Scalar()
			prf.pc.PriRand(v[s])
		}
		P.Mul(v[s], prf.pval[t.B])
		V.Add(V, P)
	}

	// Encode and send the commitment to the verifier
	return prf.pc.Put(V)
}

func (rp *repPred) respond(prf *proof, c kyber.Scalar,
	pr []kyber.Scalar) error {
	pp := prf.pp[rp]

	// Create a response array for this OR-domain if not done already
	r := prf.makeScalars(pr)

	for i := range rp.T {
		t := rp.T[i] // current term
		s := prf.sidx[t.S]

		// Produce a correct response for each variable
		// the first time we encounter that variable.
		if r[s] == nil {
			if pp.w != nil {
				// We're on a non-proof-obligated branch:
				// w was our challenge, v[s] is our response.
				r[s] = pp.v[s]
				continue
			}

			// We're on a proof-obligated branch,
			// so we need to calculate the correct response
			// as r = v-cx where x is the secret variable
			ri := prf.s.Scalar()
			ri.Mul(c, prf.sval[t.S])
			ri.Sub(pp.v[s], ri)
			r[s] = ri
		}
	}

	// Send our responses if we created the array (i.e., if pr == nil)
	return prf.sendResponses(pr, r)
}

func (rp *repPred) getCommits(prf *proof, pr []kyber.Scalar) error {

	// Create per-predicate verifier state
	V := prf.s.Point()
	r := prf.makeScalars(pr)
	vp := &verifierPred{V, r}
	prf.vp[rp] = vp

	// Get the commitment for this representation
	if e := prf.vc.Get(vp.V); e != nil {
		return e
	}

	// Fill in the r vector with the responses we'll need.
	for i := range rp.T {
		t := rp.T[i] // current term
		s := prf.sidx[t.S]
		if r[s] == nil {
			r[s] = prf.s.Scalar()
		}
	}
	return nil
}

func (rp *repPred) verify(prf *proof, c kyber.Scalar, pr []kyber.Scalar) error {
	vp := prf.vp[rp]
	r := vp.r

	// Get the needed responses if a parent didn't already
	if e := prf.getResponses(pr, r); e != nil {
		return e
	}

	// Recompute commit V=cY+r1G1+...+rkGk
	V := prf.s.Point()
	V.Mul(c, prf.pval[rp.P])
	P := prf.s.Point()
	for i := 0; i < len(rp.T); i++ {
		t := rp.T[i] // current term
		s := prf.sidx[t.S]
		P.Mul(r[s], prf.pval[t.B])
		V.Add(V, P)
	}
	if !V.Equal(vp.V) {
		return errors.New("invalid proof: commit mismatch")
	}

	return nil
}

func (rp *repPred) Prover(suite Suite, secrets map[string]kyber.Scalar,
	points map[string]kyber.Point,
	choice map[Predicate]int) Prover {
	return proof{}.init(suite, rp).prover(rp, secrets, points, choice)
}

func (rp *repPred) Verifier(suite Suite,
	points map[string]kyber.Point) Verifier {
	return proof{}.init(suite, rp).verifier(rp, points)
}

////////// And predicate //////////

type andPred []Predicate

// And predicate states that all of the constituent sub-predicates are true.
// And predicates may contain Rep predicates and/or other And predicates.
func And(sub ...Predicate) Predicate {
	and := andPred(sub)
	return &and
}

// Return a string representation of this AND predicate, mainly for debugging.
func (ap *andPred) String() string {
	return ap.precString(precNone)
}

func (ap *andPred) precString(prec int) string {
	sub := []Predicate(*ap)
	s := sub[0].precString(precAnd)
	for i := 1; i < len(sub); i++ {
		s = s + " && " + sub[i].precString(precAnd)
	}
	if prec != precNone && prec != precAnd {
		s = "(" + s + ")"
	}
	return s
}

func (ap *andPred) enumVars(prf *proof) {
	sub := []Predicate(*ap)
	for i := range sub {
		sub[i].enumVars(prf)
	}
}

func (ap *andPred) commit(prf *proof, w kyber.Scalar, pv []kyber.Scalar) error {
	sub := []Predicate(*ap)

	// Create per-predicate prover state
	v := prf.makeScalars(pv)
	//pp := proverPred{w,v,nil}
	//prf.pp[ap] = pp

	// Recursively generate commitments
	for i := 0; i < len(sub); i++ {
		if e := sub[i].commit(prf, w, v); e != nil {
			return e
		}
	}

	return nil
}

func (ap *andPred) respond(prf *proof, c kyber.Scalar, pr []kyber.Scalar) error {
	sub := []Predicate(*ap)
	//pp := prf.pp[ap]

	// Recursively compute responses in all sub-predicates
	r := prf.makeScalars(pr)
	for i := range sub {
		if e := sub[i].respond(prf, c, r); e != nil {
			return e
		}
	}
	return prf.sendResponses(pr, r)
}

func (ap *andPred) getCommits(prf *proof, pr []kyber.Scalar) error {
	sub := []Predicate(*ap)

	// Create per-predicate verifier state
	r := prf.makeScalars(pr)
	vp := &verifierPred{nil, r}
	prf.vp[ap] = vp

	for i := range sub {
		if e := sub[i].getCommits(prf, r); e != nil {
			return e
		}
	}
	return nil
}

func (ap *andPred) verify(prf *proof, c kyber.Scalar, pr []kyber.Scalar) error {
	sub := []Predicate(*ap)
	vp := prf.vp[ap]
	r := vp.r

	if e := prf.getResponses(pr, r); e != nil {
		return e
	}
	for i := range sub {
		if e := sub[i].verify(prf, c, r); e != nil {
			return e
		}
	}
	return nil
}

func (ap *andPred) Prover(suite Suite, secrets map[string]kyber.Scalar,
	points map[string]kyber.Point,
	choice map[Predicate]int) Prover {
	return proof{}.init(suite, ap).prover(ap, secrets, points, choice)
}

func (ap *andPred) Verifier(suite Suite,
	points map[string]kyber.Point) Verifier {
	return proof{}.init(suite, ap).verifier(ap, points)
}

////////// Or predicate //////////

type orPred []Predicate

// Or predicate states that the prover knows
// at least one of the sub-predicates to be true,
// but the proof does not reveal any information about which.
func Or(sub ...Predicate) Predicate {
	or := orPred(sub)
	return &or
}

// Return a string representation of this OR predicate, mainly for debugging.
func (op *orPred) String() string {
	return op.precString(precNone)
}

func (op *orPred) precString(prec int) string {
	sub := []Predicate(*op)
	s := sub[0].precString(precOr)
	for i := 1; i < len(sub); i++ {
		s = s + " || " + sub[i].precString(precOr)
	}
	if prec != precNone && prec != precOr {
		s = "(" + s + ")"
	}
	return s
}

func (op *orPred) enumVars(prf *proof) {
	sub := []Predicate(*op)
	for i := range sub {
		sub[i].enumVars(prf)
	}
}

func (op *orPred) commit(prf *proof, w kyber.Scalar, pv []kyber.Scalar) error {
	sub := []Predicate(*op)
	if pv != nil { // only happens within an AND expression
		panic("can't have OR predicates within AND predicates")
	}

	// Create per-predicate prover state
	wi := make([]kyber.Scalar, len(sub))
	pp := &proverPred{w, nil, wi}
	prf.pp[op] = pp

	// Choose pre-challenges for our subs.
	if w == nil {
		// We're on a proof-obligated branch;
		// choose random pre-challenges for only non-obligated subs.
		choice, ok := prf.choice[op]
		if !ok || choice < 0 || choice >= len(sub) {
			panic("no choice of proof branch for OR-predicate " +
				op.String())
		}
		for i := 0; i < len(sub); i++ {
			if i != choice {
				wi[i] = prf.s.Scalar()
				prf.pc.PriRand(wi[i])
			} // else wi[i] == nil for proof-obligated sub
		}
	} else {
		// Since w != nil, we're in a non-obligated branch,
		// so choose random pre-challenges for all subs
		// such that they add up to the master pre-challenge w.
		last := len(sub) - 1 // index of last sub
		wl := prf.s.Scalar().Set(w)
		for i := 0; i < last; i++ { // choose all but last
			wi[i] = prf.s.Scalar()
			prf.pc.PriRand(wi[i])
			wl.Sub(wl, wi[i])
		}
		wi[last] = wl
	}

	// Now recursively choose commitments within each sub
	for i := 0; i < len(sub); i++ {
		// Fresh variable-blinding secrets for each pre-commitment
		if e := sub[i].commit(prf, wi[i], nil); e != nil {
			return e
		}
	}

	return nil
}

func (op *orPred) respond(prf *proof, c kyber.Scalar, pr []kyber.Scalar) error {
	sub := []Predicate(*op)
	pp := prf.pp[op]
	if pr != nil {
		panic("OR predicates can't be nested in anything else")
	}

	ci := pp.wi
	if pp.w == nil {
		// Calculate the challenge for the proof-obligated subtree
		cs := prf.s.Scalar().Set(c)
		choice := prf.choice[op]
		for i := 0; i < len(sub); i++ {
			if i != choice {
				cs.Sub(cs, ci[i])
			}
		}
		ci[choice] = cs
	}

	// If there's more than one choice, send all our sub-challenges.
	if len(sub) > 1 {
		if e := prf.pc.Put(ci); e != nil {
			return e
		}
	}

	// Recursively compute responses in all subtrees
	for i := range sub {
		if e := sub[i].respond(prf, ci[i], nil); e != nil {
			return e
		}
	}

	return nil
}

// Get from the verifier all the commitments needed for this predicate
func (op *orPred) getCommits(prf *proof, pr []kyber.Scalar) error {
	sub := []Predicate(*op)
	for i := range sub {
		if e := sub[i].getCommits(prf, nil); e != nil {
			return e
		}
	}
	return nil
}

func (op *orPred) verify(prf *proof, c kyber.Scalar, pr []kyber.Scalar) error {
	sub := []Predicate(*op)
	if pr != nil {
		panic("OR predicates can't be in anything else")
	}

	// Get the prover's sub-challenges
	nsub := len(sub)
	ci := make([]kyber.Scalar, nsub)
	if nsub > 1 {
		if e := prf.vc.Get(ci); e != nil {
			return e
		}

		// Make sure they add up to the parent's composite challenge
		csum := prf.s.Scalar().Zero()
		for i := 0; i < nsub; i++ {
			csum.Add(csum, ci[i])
		}
		if !csum.Equal(c) {
			return errors.New("invalid proof: bad sub-challenges")
		}

	} else { // trivial single-sub OR
		ci[0] = c
	}

	// Recursively verify all subs
	for i := range sub {
		if e := sub[i].verify(prf, ci[i], nil); e != nil {
			return e
		}
	}

	return nil
}

func (op *orPred) Prover(suite Suite, secrets map[string]kyber.Scalar,
	points map[string]kyber.Point,
	choice map[Predicate]int) Prover {
	return proof{}.init(suite, op).prover(op, secrets, points, choice)
}

func (op *orPred) Verifier(suite Suite,
	points map[string]kyber.Point) Verifier {
	return proof{}.init(suite, op).verifier(op, points)
}

/*
type lin struct {
	a1,a2,b kyber.Scalar
	x1,x2 PriVar
}
*/

// Construct a predicate asserting a linear relationship a1x1+a2x2=b,
// where a1,a2,b are public values and x1,x2 are secrets.
/*
func (p *Prover) Linear(a1,a2,b kyber.Scalar, x1,x2 PriVar) {
	return &lin{a1,a2,b,x1,x2}
}
*/

func (prf proof) init(suite Suite, pred Predicate) *proof {
	prf.s = suite

	// Enumerate all the variables in a consistent order.
	// Reserve variable index 0 for convenience.
	prf.svar = []string{""}
	prf.pvar = []string{""}
	prf.sidx = make(map[string]int)
	prf.pidx = make(map[string]int)
	pred.enumVars(&prf)
	prf.nsvars = len(prf.svar)
	prf.npvars = len(prf.pvar)

	return &prf
}

func (prf *proof) enumScalarVar(name string) {
	if prf.sidx[name] == 0 {
		prf.sidx[name] = len(prf.svar)
		prf.svar = append(prf.svar, name)
	}
}

func (prf *proof) enumPointVar(name string) {
	if prf.pidx[name] == 0 {
		prf.pidx[name] = len(prf.pvar)
		prf.pvar = append(prf.pvar, name)
	}
}

// Make a response-array if that wasn't already done in a parent predicate.
func (prf *proof) makeScalars(pr []kyber.Scalar) []kyber.Scalar {
	if pr == nil {
		return make([]kyber.Scalar, prf.nsvars)
	}
	return pr
}

// Transmit our response-array if a corresponding makeScalars() created it.
func (prf *proof) sendResponses(pr []kyber.Scalar, r []kyber.Scalar) error {
	if pr == nil {
		for i := range r {
			// Send responses only for variables
			// that were used in this OR-domain.
			if r[i] != nil {
				if e := prf.pc.Put(r[i]); e != nil {
					return e
				}
			}
		}
	}
	return nil
}

// In the verifier, get the responses at the top of an OR-domain,
// if a corresponding makeScalars() call created it.
func (prf *proof) getResponses(pr []kyber.Scalar, r []kyber.Scalar) error {
	if pr == nil {
		for i := range r {
			if r[i] != nil {
				if e := prf.vc.Get(r[i]); e != nil {
					return e
				}
			}
		}
	}
	return nil
}

func (prf *proof) prove(p Predicate, sval map[string]kyber.Scalar,
	pval map[string]kyber.Point,
	choice map[Predicate]int, pc ProverContext) error {
	prf.pc = pc
	prf.sval = sval
	prf.pval = pval
	prf.choice = choice
	prf.pp = make(map[Predicate]*proverPred)

	// Generate all commitments
	if e := p.commit(prf, nil, nil); e != nil {
		return e
	}

	// Generate top-level challenge from public randomness
	c := prf.s.Scalar()
	if e := pc.PubRand(c); e != nil {
		return e
	}

	// Generate all responses based on master challenge
	return p.respond(prf, c, nil)
}

func (prf *proof) verify(p Predicate, pval map[string]kyber.Point,
	vc VerifierContext) error {
	prf.vc = vc
	prf.pval = pval
	prf.vp = make(map[Predicate]*verifierPred)

	// Get the commitments from the verifier,
	// and calculate the sets of responses we'll need for each OR-domain.
	if e := p.getCommits(prf, nil); e != nil {
		return e
	}

	// Produce the top-level challenge
	c := prf.s.Scalar()
	if e := vc.PubRand(c); e != nil {
		return e
	}

	// Check all the responses and sub-challenges against the commitments.
	return p.verify(prf, c, nil)
}

// Produce a higher-order Prover embodying a given proof predicate.
func (prf *proof) prover(p Predicate, sval map[string]kyber.Scalar,
	pval map[string]kyber.Point,
	choice map[Predicate]int) Prover {

	return Prover(func(ctx ProverContext) error {
		return prf.prove(p, sval, pval, choice, ctx)
	})
}

// Produce a higher-order Verifier embodying a given proof predicate.
func (prf *proof) verifier(p Predicate, pval map[string]kyber.Point) Verifier {

	return Verifier(func(ctx VerifierContext) error {
		return prf.verify(p, pval, ctx)
	})
}
