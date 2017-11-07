// +build experimental

// Package poly implements polynomial commitments, openings,
// and verifiable Shamir secret sharing.
package poly

import (
	"crypto/cipher"
	"errors"
	"fmt"
	"io"

	"github.com/dedis/kyber/abstract"
	"github.com/dedis/kyber/random"
)

// Clique protocol outline:
// 1. Leader sents Init message to all members
//	Fresh DH pubkey, max #members, loc hints?
// 2. Subset of members respond with InitAck
//	Responder DH pubkey, sig_resp(init-pubkey,resp-pubkey)
//	optional authentication here??
// 3. Leader announces pubkeys of all members
//	list of signed pubkeys, sig_int(list of signed pubkeys)
// 4. All members create secret key share, Shamir share it,
//	encrypt shares for all other members, send to leader
// 5. Leader rebroadcasts all shares, and combination, to all members
//	may be a subset of members if not all initial members responded

// Private polynomial for Shamir secret sharing.
type PriPoly struct {
	g abstract.Group    // Cryptographic group in use
	s []abstract.Scalar // Coefficients of secret polynomial
}

// Create a fresh sharing polynomial in the Scalar space of a given group.
// Shares the provided Scalar s, or picks a random one if s == nil.
func (p *PriPoly) Pick(g abstract.Group, k int, s0 abstract.Scalar,
	rand cipher.Stream) *PriPoly {
	p.g = g
	s := make([]abstract.Scalar, k)
	if s0 == nil { // Choose secret to share if none provided
		s0 = g.Scalar().Pick(rand)
	}
	s[0] = s0
	for i := 1; i < k; i++ {
		s[i] = g.Scalar().Pick(rand)
	}
	p.s = s
	return p
}

// Return the shared secret from a private sharing polynomial.
func (p *PriPoly) Secret() abstract.Scalar {
	return p.s[0]
}

// Test polynomials for equality component-wise.
// Assumes they are of the same degree and from the same Scalar field.
func (p1 *PriPoly) Equal(p2 *PriPoly) bool {
	k := len(p1.s)
	if p1.g != p2.g || k != len(p2.s) {
		panic("Mismatched polynomials")
	}
	for i := 0; i < len(p1.s); i++ {
		if !p1.s[i].Equal(p2.s[i]) {
			return false
		}
	}
	return true
}

// Evaluate the polynomial to produce the secret for party i.
func (p *PriPoly) Eval(i int) abstract.Scalar {
	g := p.g
	k := len(p.s)
	xi := g.Scalar().SetInt64(1 + int64(i)) // x-coordinate of this share
	sv := g.Scalar().Zero()
	for i = k - 1; i >= 0; i-- {
		sv.Mul(sv, xi)
		sv.Add(sv, p.s[i])
	}
	return sv
}

// Set to the component-wise addition of two polynomials,
// which are assumed to be of the same degree and from the same Scalar field.
func (p *PriPoly) Add(p1, p2 *PriPoly) *PriPoly {
	g := p1.g
	k := len(p1.s)
	if g != p2.g || k != len(p2.s) {
		panic("Mismatched polynomials")
	}
	s := make([]abstract.Scalar, k)
	for i := 0; i < k; i++ {
		s[i] = g.Scalar().Add(p1.s[i], p2.s[i])
	}
	p.g = g
	p.s = s
	return p
}

// Dump a string representation of the polynomial.
func (p *PriPoly) String() string {
	k := len(p.s)
	s := p.s[0].String()
	for i := 1; i < k; i++ {
		s += fmt.Sprintf("+%s*x", p.s[i].String())
		if i > 1 {
			s += fmt.Sprintf("^%d", i)
		}
	}
	return s
}

// Secret shares generated from a private polynomial.
type PriShares struct {
	g abstract.Group    // Cryptographic group in use
	k int               // Reconstruction threshold
	s []abstract.Scalar // Secret shares, one per sharing party.
}

// Create a desired number of secret-shares from a private polynomial,
// each of which is typically to be distributed to a distinct party.
// Any k of these shares may be used to reconstruct the original secret.
// Amounts to evaluating the private polynomial at positions 1, ..., n.
func (ps *PriShares) Split(p *PriPoly, n int) *PriShares {
	g := p.g
	k := len(p.s)
	s := make([]abstract.Scalar, n)
	for i := 0; i < n; i++ {
		s[i] = p.Eval(i)
	}
	ps.g = g
	ps.k = k
	ps.s = s
	return ps
}

// Return a given node i's share.
func (ps *PriShares) Share(i int) abstract.Scalar {
	return ps.s[i]
}

// Initialize a set of secret-shares to an initially empty list,
// before populating using SetShare() and reconstruction.
func (ps *PriShares) Empty(g abstract.Group, k, n int) {
	ps.g = g
	ps.k = k
	ps.s = make([]abstract.Scalar, n)
}

// Set node i's share.
func (ps *PriShares) SetShare(i int, s abstract.Scalar) {
	ps.s[i] = s
}

// Create an array of x-coordinates we need for Lagrange interpolation.
// In the returned array, exactly k x-coordinates are non-nil.
func (ps *PriShares) xCoords() []abstract.Scalar {
	x := make([]abstract.Scalar, len(ps.s))
	c := 0
	for i := range ps.s {
		if ps.s[i] != nil {
			x[i] = ps.g.Scalar().SetInt64(1 + int64(i))
			c++
			if c >= ps.k {
				break // have enough shares, ignore any more
			}
		}
	}
	if c < ps.k {
		panic("Not enough shares to reconstruct secret")
	}
	return x
}

// Use Lagrange interpolation to reconstruct a secret,
// from a private share array of which
// at least a threshold k of shares are populated (non-nil).
func (ps *PriShares) Secret() abstract.Scalar {

	// compute Lagrange interpolation for point x=0 (the shared secret)
	x := ps.xCoords()
	a := ps.g.Scalar().Zero() // sum accumulator
	n := ps.g.Scalar()        // numerator temporary
	d := ps.g.Scalar()        // denominator temporary
	t := ps.g.Scalar()        // temporary
	for i := range x {
		if x[i] == nil {
			continue
		}
		n.Set(ps.s[i])
		d.One()
		for j := range x {
			if j == i || x[j] == nil {
				continue
			}
			n.Mul(n, x[j])
			d.Mul(d, t.Sub(x[j], x[i]))
		}
		a.Add(a, n.Div(n, d))
	}
	return a
}

func (ps *PriShares) String() string {
	s := "{"
	for i := range ps.s {
		if i > 0 {
			s += ","
		}
		if ps.s[i] != nil {
			s += ps.s[i].String()
		} else {
			s += "<missing>"
		}
	}
	s += "}"
	return s
}

// A public commitment to a secret-sharing polynomial.
type PubPoly struct {
	g abstract.Group   // Cryptographic group in use
	b abstract.Point   // Base point, nil for standard base
	p []abstract.Point // Commitments to polynomial coefficients
}

// Initialize to an empty polynomial for a given group and threshold (degree),
// typically before using Decode() to fill in from a received message.
func (pub *PubPoly) Init(g abstract.Group, k int, b abstract.Point) *PubPoly {
	pub.g = g
	pub.b = b
	pub.p = make([]abstract.Point, k)
	return pub
}

// InitNull does the same thing as Init PLUS initialize every points / coef to the Null
// Identity Element so we can use it like a "temp" / "aggregate" variable to add with others poly
func (pub *PubPoly) InitNull(g abstract.Group, k int, b abstract.Point) *PubPoly {
	pub.Init(g, k, b)
	for i, _ := range pub.p {
		pub.p[i] = g.Point().Null()
	}
	return pub
}

// Return k : the number of shares needed to reconstruct a secret from the corresponding pripoly
func (pub *PubPoly) GetK() int {
	return len(pub.p)
}

// Initialize to a public commitment to a given private polynomial.
// Create commitments as encryptions of a given base point b,
// or the standard base if b == nil.
func (pub *PubPoly) Commit(pri *PriPoly, b abstract.Point) *PubPoly {
	g := pri.g
	k := len(pri.s)
	p := make([]abstract.Point, k)
	for i := 0; i < k; i++ {
		p[i] = g.Point().Mul(b, pri.s[i])
	}
	pub.g = g
	pub.b = b
	pub.p = p
	return pub
}

// Return the secret commit (constant term) from this polynomial.
func (pub *PubPoly) SecretCommit() abstract.Point {
	return pub.p[0]
}

// Return the encoded length of this polynomial commitment.
func (pub *PubPoly) MarshalSize() int {
	return pub.g.PointLen() * len(pub.p)
}

// Encode this polynomial into a byte slice exactly Len() bytes long.
func (pub *PubPoly) MarshalBinary() ([]byte, error) {
	pl := pub.g.PointLen()
	b := make([]byte, pub.MarshalSize())
	for i := range pub.p {
		pb, _ := pub.p[i].MarshalBinary()
		if len(pb) != pl {
			panic("Encoded point wrong length")
		}
		copy(b[i*pl:], pb)
	}
	return b, nil
}

// Decode this polynomial from a slice exactly Len() bytes long.
func (pub *PubPoly) UnmarshalBinary(b []byte) error {
	k := len(pub.p)
	pl := pub.g.PointLen()
	if len(b) != k*pl {
		return errors.New("Encoded polynomial commitment wrong length")
	}
	for i := 0; i < k; i++ {
		pub.p[i] = pub.g.Point()
		if err := pub.p[i].UnmarshalBinary(b[i*pl : i*pl+pl]); err != nil {
			return err
		}
	}
	return nil
}

func (pub *PubPoly) MarshalTo(w io.Writer) (int, error) {
	pubb, _ := pub.MarshalBinary()
	return w.Write(pubb)
}

func (pub *PubPoly) UnmarshalFrom(r io.Reader) (int, error) {
	buf := make([]byte, pub.MarshalSize())
	n, err := io.ReadFull(r, buf)
	if err != nil {
		return n, err
	}
	return n, pub.UnmarshalBinary(buf)
}

// Test polynomial commitments for equality.
// Assumes they are of the same degree and from the same group.
func (p1 *PubPoly) Equal(p2 *PubPoly) bool {
	k := len(p1.p)
	if p1.g.String() != p2.g.String() || k != len(p2.p) {
		panic("Mismatched polynomial commitments")
	}
	for i := 0; i < len(p1.p); i++ {
		if !p1.p[i].Equal(p2.p[i]) {
			return false
		}
	}
	return true
}

// Homomorphically evaluate a commitment to the share for party i.
func (pub *PubPoly) Eval(i int) abstract.Point {
	g := pub.g
	k := len(pub.p)
	xi := g.Scalar().SetInt64(1 + int64(i)) // x-coordinate of this share
	pv := g.Point().Null()
	for i = k - 1; i >= 0; i-- {
		pv.Mul(pv, xi)
		pv.Add(pv, pub.p[i])
	}
	return pv
}

// Homomorphically add two public polynomial commitments,
// to form a public commitment to the sum of the two polynomials.
func (pub *PubPoly) Add(p1, p2 *PubPoly) *PubPoly {
	g := p1.g
	k := len(p1.p)
	if p1.g.String() != p2.g.String() {
		panic("Mismatched Group of polynomial commitments")
	} else if k != len(p2.p) {
		panic("Mismatched polynomial commitments")
	}
	p := make([]abstract.Point, k)
	for i := 0; i < k; i++ {
		p[i] = g.Point().Add(p1.p[i], p2.p[i])
	}
	pub.g = g
	pub.p = p
	return pub
}

// Check a secret share against a public polynomial commitment.
// This amounts to evaluating the polynomial under homomorphic encryption.
func (pub *PubPoly) Check(i int, share abstract.Scalar) bool {
	pv := pub.Eval(i)
	ps := pub.g.Point().Mul(pub.b, share)
	return pv.Equal(ps)
}

// Dump a string representation of the polynomial commitment.
func (p *PubPoly) String() string {
	k := len(p.p)
	if k < 1 {
		return "Empty PubPoly"
	}
	s := ""
	for i := 0; i < k; i++ {
		if p.p[i] != nil {
			s += fmt.Sprintf(",%s", p.p[i].String())
		} else {
			s += ",nil"
		}
	}
	return s
}

// Public commitments to shares generated from a private polynomial.
type PubShares struct {
	g abstract.Group   // Cryptographic group in use
	k int              // Reconstruction threshold
	b abstract.Point   // Base point, nil for standard base
	p []abstract.Point // Commitment shares, one per sharing party.
}

// Create individual share commitments from a polynomial commitment,
// one for each of n parties.
// Homomorphically evaluates the polynomial at positions 1, ..., n.
func (ps *PubShares) Split(pub *PubPoly, n int) *PubShares {
	g := pub.g
	k := len(pub.p)
	p := make([]abstract.Point, n)
	for i := 0; i < n; i++ {
		p[i] = pub.Eval(i)
	}
	ps.g = g
	ps.k = k
	ps.b = pub.b
	ps.p = p
	return ps
}

// Return the share commitment for a given party i.
func (ps *PubShares) Share(i int) abstract.Point {
	return ps.p[i]
}

// Set node i's share commitment.
func (ps *PubShares) SetShare(i int, p abstract.Point) {
	ps.p[i] = p
}

// Create an array of x-coordinates we need for Lagrange interpolation.
// In the returned array, exactly k x-coordinates are non-nil.
func (ps *PubShares) xCoords() []abstract.Scalar {
	x := make([]abstract.Scalar, len(ps.p))
	c := 0
	for i := range ps.p {
		if ps.p[i] != nil {
			x[i] = ps.g.Scalar().SetInt64(1 + int64(i))
			c++
			if c >= ps.k {
				break // have enough shares, ignore any more
			}
		}
	}
	if c < ps.k {
		panic("Not enough shares to reconstruct secret")
	}
	return x
}

// Use Lagrange interpolation homomorphically
// to reconstruct a secret commitment,
// from an array of share commitments of which
// at least a threshold k of shares are populated (non-nil).
func (ps *PubShares) SecretCommit() abstract.Point {

	// compute Lagrange interpolation for point x=0 (the shared secret)
	// XXX could probably share more code with non-homomorphic version.
	x := ps.xCoords()
	n := ps.g.Scalar()       // numerator temporary
	d := ps.g.Scalar()       // denominator temporary
	t := ps.g.Scalar()       // temporary secret
	A := ps.g.Point().Null() // point accumulator
	P := ps.g.Point()        // temporary point
	for i := range x {
		if x[i] == nil {
			continue
		}
		n.One()
		d.One()
		for j := range x {
			if j == i || x[j] == nil {
				continue
			}
			n.Mul(n, x[j])
			d.Mul(d, t.Sub(x[j], x[i]))
		}
		P.Mul(ps.p[i], n.Div(n, d))
		A.Add(A, P)
	}
	return A
}

func (ps *PubShares) String() string {
	s := "{"
	for i := range ps.p {
		if i > 0 {
			s += ","
		}
		if ps.p[i] != nil {
			s += ps.p[i].String()
		} else {
			s += "<missing>"
		}
	}
	s += "}"
	return s
}

func testSharing(g abstract.Group) {

	k := 4
	n := 10
	p1 := new(PriPoly).Pick(g, k, nil, random.Stream)
	p2 := new(PriPoly).Pick(g, k, nil, random.Stream)
	p3 := new(PriPoly).Add(p1, p2)
	if p1.Equal(p2) || p1.Equal(p3) || !p1.Equal(p1) || !p2.Equal(p2) {
		panic("PriPoly equality doesn't work")
	}

	pub1 := new(PubPoly).Commit(p1, nil)
	pub2 := new(PubPoly).Commit(p2, nil)
	pub3 := new(PubPoly).Commit(p3, nil)
	if pub1.Equal(pub2) || pub1.Equal(pub3) {
		panic("PubPoly equality false positive")
	}
	if !pub1.Equal(pub1) || !pub2.Equal(pub2) {
		panic("PubPoly equality false negative")
	}
	pub3c := new(PubPoly).Add(pub1, pub2)
	if !pub3.Equal(pub3c) {
		panic("PubPoly homomorphic addition doesn't work")
	}

	ps1 := new(PriShares).Split(p1, n)
	if !ps1.Secret().Equal(p1.Secret()) {
		panic("Secret recovery doesn't work!")
	}

	// Share-checking
	for i := 0; i < n; i++ {
		if !pub1.Check(i, ps1.Share(i)) {
			panic("Share checking doesn't work")
		}
		if pub2.Check(i, ps1.Share(i)) {
			panic("Share checking false positive!?")
		}
	}

	// Produce share commitments from the public polynomial commitment.
	pubs1 := new(PubShares).Split(pub1, n)
	for i := 0; i < n; i++ {
		P := g.Point().Mul(nil, ps1.Share(i))
		if !P.Equal(pubs1.Share(i)) {
			panic("Homomorphic share splitting didn't work")
		}
	}

	// Cut out even-numbered shares for fun
	for i := 0; i < n; i += 2 {
		ps1.SetShare(i, nil)
		pubs1.SetShare(i, nil)
	}
	if !ps1.Secret().Equal(p1.Secret()) {
		panic("Secret recovery from partial set doesn't work!")
	}

	// Homomorphic share reconstruction
	P := g.Point().Mul(nil, p1.Secret())
	if !P.Equal(pub1.SecretCommit()) {
		panic("Public polynomial committed wrong secret")
	}
	if !P.Equal(pubs1.SecretCommit()) {
		panic("Homomorphic secret reconstruction didn't work")
	}

	// Cut to the minimum number of shares
	ps1.SetShare(1, nil)
	if !ps1.Secret().Equal(p1.Secret()) {
		panic("Secret recovery from partial set doesn't work!")
	}
	if !P.Equal(pubs1.SecretCommit()) {
		panic("Homomorphic secret reconstruction didn't work")
	}
}
