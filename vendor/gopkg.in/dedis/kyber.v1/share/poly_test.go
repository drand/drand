package share

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/dedis/kyber.v1/group/edwards25519"
	"gopkg.in/dedis/kyber.v1/util/random"
)

func TestSecretRecovery(test *testing.T) {
	g := edwards25519.NewAES128SHA256Ed25519()
	n := 10
	t := n/2 + 1
	poly := NewPriPoly(g, t, nil, random.Stream)
	shares := poly.Shares(n)

	recovered, err := RecoverSecret(g, shares, t, n)
	if err != nil {
		test.Fatal(err)
	}

	if !recovered.Equal(poly.Secret()) {
		test.Fatal("recovered secret does not match initial value")
	}
}

func TestSecretRecoveryDelete(test *testing.T) {
	g := edwards25519.NewAES128SHA256Ed25519()
	n := 10
	t := n/2 + 1
	poly := NewPriPoly(g, t, nil, random.Stream)
	shares := poly.Shares(n)

	// Corrupt a few shares
	shares[2] = nil
	shares[5] = nil
	shares[7] = nil
	shares[8] = nil

	recovered, err := RecoverSecret(g, shares, t, n)
	if err != nil {
		test.Fatal(err)
	}

	if !recovered.Equal(poly.Secret()) {
		test.Fatal("recovered secret does not match initial value")
	}
}

func TestSecretRecoveryDeleteFail(test *testing.T) {
	g := edwards25519.NewAES128SHA256Ed25519()
	n := 10
	t := n/2 + 1

	poly := NewPriPoly(g, t, nil, random.Stream)
	shares := poly.Shares(n)

	// Corrupt one more share than acceptable
	shares[1] = nil
	shares[2] = nil
	shares[5] = nil
	shares[7] = nil
	shares[8] = nil

	_, err := RecoverSecret(g, shares, t, n)
	if err == nil {
		test.Fatal("recovered secret unexpectably")
	}
}

func TestSecretPolyEqual(test *testing.T) {
	g := edwards25519.NewAES128SHA256Ed25519()
	n := 10
	t := n/2 + 1

	p1 := NewPriPoly(g, t, nil, random.Stream)
	p2 := NewPriPoly(g, t, nil, random.Stream)
	p3 := NewPriPoly(g, t, nil, random.Stream)

	p12, _ := p1.Add(p2)
	p13, _ := p1.Add(p3)

	p123, _ := p12.Add(p3)
	p132, _ := p13.Add(p2)

	if !p123.Equal(p132) {
		test.Fatal("private polynomials not equal")
	}
}

func TestPublicCheck(test *testing.T) {
	g := edwards25519.NewAES128SHA256Ed25519()
	n := 10
	t := n/2 + 1

	priPoly := NewPriPoly(g, t, nil, random.Stream)
	priShares := priPoly.Shares(n)
	pubPoly := priPoly.Commit(nil)

	for i, share := range priShares {
		if !pubPoly.Check(share) {
			test.Fatalf("private share %v not valid with respect to the public commitment polynomial", i)
		}
	}
}

func TestPublicRecovery(test *testing.T) {
	g := edwards25519.NewAES128SHA256Ed25519()
	n := 10
	t := n/2 + 1

	priPoly := NewPriPoly(g, t, nil, random.Stream)
	pubPoly := priPoly.Commit(nil)
	pubShares := pubPoly.Shares(n)

	recovered, err := RecoverCommit(g, pubShares, t, n)
	if err != nil {
		test.Fatal(err)
	}

	if !recovered.Equal(pubPoly.Commit()) {
		test.Fatal("recovered commit does not match initial value")
	}
}

func TestPublicRecoveryDelete(test *testing.T) {
	g := edwards25519.NewAES128SHA256Ed25519()
	n := 10
	t := n/2 + 1

	priPoly := NewPriPoly(g, t, nil, random.Stream)
	pubPoly := priPoly.Commit(nil)
	shares := pubPoly.Shares(n)

	// Corrupt a few shares
	shares[2] = nil
	shares[5] = nil
	shares[7] = nil
	shares[8] = nil

	recovered, err := RecoverCommit(g, shares, t, n)
	if err != nil {
		test.Fatal(err)
	}

	if !recovered.Equal(pubPoly.Commit()) {
		test.Fatal("recovered commit does not match initial value")
	}
}

func TestPublicRecoveryDeleteFail(test *testing.T) {
	g := edwards25519.NewAES128SHA256Ed25519()
	n := 10
	t := n/2 + 1

	priPoly := NewPriPoly(g, t, nil, random.Stream)
	pubPoly := priPoly.Commit(nil)
	shares := pubPoly.Shares(n)

	// Corrupt one more share than acceptable
	shares[1] = nil
	shares[2] = nil
	shares[5] = nil
	shares[7] = nil
	shares[8] = nil

	_, err := RecoverCommit(g, shares, t, n)
	if err == nil {
		test.Fatal("recovered commit unexpectably")
	}
}

func TestPrivateAdd(test *testing.T) {
	g := edwards25519.NewAES128SHA256Ed25519()
	n := 10
	t := n/2 + 1

	p := NewPriPoly(g, t, nil, random.Stream)
	q := NewPriPoly(g, t, nil, random.Stream)

	r, err := p.Add(q)
	if err != nil {
		test.Fatal(err)
	}

	ps := p.Secret()
	qs := q.Secret()
	rs := g.Scalar().Add(ps, qs)

	if !rs.Equal(r.Secret()) {
		test.Fatal("addition of secret sharing polynomials failed")
	}
}

func TestPublicAdd(test *testing.T) {
	g := edwards25519.NewAES128SHA256Ed25519()
	n := 10
	t := n/2 + 1

	G := g.Point().Pick(random.Stream)
	H := g.Point().Pick(random.Stream)

	p := NewPriPoly(g, t, nil, random.Stream)
	q := NewPriPoly(g, t, nil, random.Stream)

	P := p.Commit(G)
	Q := q.Commit(H)

	R, err := P.Add(Q)
	if err != nil {
		test.Fatal(err)
	}

	shares := R.Shares(n)
	recovered, err := RecoverCommit(g, shares, t, n)
	if err != nil {
		test.Fatal(err)
	}

	x := P.Commit()
	y := Q.Commit()
	z := g.Point().Add(x, y)

	if !recovered.Equal(z) {
		test.Fatal("addition of public commitment polynomials failed")
	}
}

func TestPublicPolyEqual(test *testing.T) {
	g := edwards25519.NewAES128SHA256Ed25519()
	n := 10
	t := n/2 + 1

	G := g.Point().Pick(random.Stream)

	p1 := NewPriPoly(g, t, nil, random.Stream)
	p2 := NewPriPoly(g, t, nil, random.Stream)
	p3 := NewPriPoly(g, t, nil, random.Stream)

	P1 := p1.Commit(G)
	P2 := p2.Commit(G)
	P3 := p3.Commit(G)

	P12, _ := P1.Add(P2)
	P13, _ := P1.Add(P3)

	P123, _ := P12.Add(P3)
	P132, _ := P13.Add(P2)

	if !P123.Equal(P132) {
		test.Fatal("public polynomials not equal")
	}
}

func TestPriPolyMul(test *testing.T) {
	suite := edwards25519.NewAES128SHA256Ed25519()
	n := 10
	t := n/2 + 1
	a := NewPriPoly(suite, t, nil, random.Stream)
	b := NewPriPoly(suite, t, nil, random.Stream)

	c := a.Mul(b)
	assert.Equal(test, len(a.coeffs)+len(b.coeffs)-1, len(c.coeffs))
	nul := suite.Scalar().Zero()
	for _, coeff := range c.coeffs {
		assert.NotEqual(test, nul.String(), coeff.String())
	}

	a0 := a.coeffs[0]
	b0 := b.coeffs[0]
	mul := suite.Scalar().Mul(b0, a0)
	c0 := c.coeffs[0]
	assert.Equal(test, c0.String(), mul.String())

	at := a.coeffs[len(a.coeffs)-1]
	bt := b.coeffs[len(b.coeffs)-1]
	mul = suite.Scalar().Mul(at, bt)
	ct := c.coeffs[len(c.coeffs)-1]
	assert.Equal(test, ct.String(), mul.String())
}

func TestRecoverPriPoly(test *testing.T) {
	suite := edwards25519.NewAES128SHA256Ed25519()
	n := 10
	t := n/2 + 1
	a := NewPriPoly(suite, t, nil, random.Stream)

	shares := a.Shares(n)
	reverses := make([]*PriShare, len(shares))
	l := len(shares) - 1
	for i := range shares {
		reverses[l-i] = shares[i]
	}
	recovered, err := RecoverPriPoly(suite, shares, t, n)
	assert.Nil(test, err)

	reverseRecovered, err := RecoverPriPoly(suite, reverses, t, n)
	assert.Nil(test, err)

	for i := 0; i < t; i++ {
		assert.Equal(test, recovered.Eval(i).V.String(), a.Eval(i).V.String())
		assert.Equal(test, reverseRecovered.Eval(i).V.String(), a.Eval(i).V.String())
	}
}
