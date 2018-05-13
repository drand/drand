// +build vartime

package nist

import (
	"crypto/cipher"
	"crypto/dsa"
	"errors"
	"fmt"
	"io"
	"math/big"
	//"encoding/hex"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/group/internal/marshalling"
	"github.com/dedis/kyber/group/mod"
	"github.com/dedis/kyber/util/random"
)

var one = big.NewInt(1)
var two = big.NewInt(2)

type residuePoint struct {
	big.Int
	g *ResidueGroup
}

// Steal value from DSA, which uses recommendation from FIPS 186-3
const numMRTests = 64

// Probabilistically test whether a big integer is prime.
func isPrime(i *big.Int) bool {
	return i.ProbablyPrime(numMRTests)
}

func (p *residuePoint) String() string { return p.Int.String() }

func (p *residuePoint) Equal(p2 kyber.Point) bool {
	return p.Int.Cmp(&p2.(*residuePoint).Int) == 0
}

func (p *residuePoint) Null() kyber.Point {
	p.Int.SetInt64(1)
	return p
}

func (p *residuePoint) Base() kyber.Point {
	p.Int.Set(p.g.G)
	return p
}

func (p *residuePoint) Set(p2 kyber.Point) kyber.Point {
	p.g = p2.(*residuePoint).g
	p.Int = p2.(*residuePoint).Int
	return p
}

func (p *residuePoint) Clone() kyber.Point {
	return &residuePoint{g: p.g, Int: p.Int}
}

func (p *residuePoint) Valid() bool {
	return p.Int.Sign() > 0 && p.Int.Cmp(p.g.P) < 0 &&
		new(big.Int).Exp(&p.Int, p.g.Q, p.g.P).Cmp(one) == 0
}

func (p *residuePoint) EmbedLen() int {
	// Reserve at least 8 most-significant bits for randomness,
	// and the least-significant 16 bits for embedded data length.
	return (p.g.P.BitLen() - 8 - 16) / 8
}

func (p *residuePoint) Pick(rand cipher.Stream) kyber.Point {
	return p.Embed(nil, rand)
}

// Embed the given data with some pseudo-random bits.
// This will only work efficiently for quadratic residue groups!
func (p *residuePoint) Embed(data []byte, rand cipher.Stream) kyber.Point {

	l := p.g.PointLen()
	dl := p.EmbedLen()
	if dl > len(data) {
		dl = len(data)
	}

	for {
		b := random.Bits(uint(p.g.P.BitLen()), false, rand)
		if data != nil {
			b[l-1] = byte(dl) // Encode length in low 16 bits
			b[l-2] = byte(dl >> 8)
			copy(b[l-dl-2:l-2], data) // Copy in embedded data
		}
		p.Int.SetBytes(b)
		if p.Valid() {
			return p
		}
	}
}

// Extract embedded data from a Residue group element
func (p *residuePoint) Data() ([]byte, error) {
	b := p.Int.Bytes()
	l := p.g.PointLen()
	if len(b) < l { // pad leading zero bytes if necessary
		b = append(make([]byte, l-len(b)), b...)
	}
	dl := int(b[l-2])<<8 + int(b[l-1])
	if dl > p.EmbedLen() {
		return nil, errors.New("invalid embedded data length")
	}
	return b[l-dl-2 : l-2], nil
}

func (p *residuePoint) Add(a, b kyber.Point) kyber.Point {
	p.Int.Mul(&a.(*residuePoint).Int, &b.(*residuePoint).Int)
	p.Int.Mod(&p.Int, p.g.P)
	return p
}

func (p *residuePoint) Sub(a, b kyber.Point) kyber.Point {
	binv := new(big.Int).ModInverse(&b.(*residuePoint).Int, p.g.P)
	p.Int.Mul(&a.(*residuePoint).Int, binv)
	p.Int.Mod(&p.Int, p.g.P)
	return p
}

func (p *residuePoint) Neg(a kyber.Point) kyber.Point {
	p.Int.ModInverse(&a.(*residuePoint).Int, p.g.P)
	return p
}

func (p *residuePoint) Mul(s kyber.Scalar, b kyber.Point) kyber.Point {
	if b == nil {
		return p.Base().Mul(s, p)
	}
	// to protect against golang/go#22830
	var tmp big.Int
	tmp.Exp(&b.(*residuePoint).Int, &s.(*mod.Int).V, p.g.P)
	p.Int = tmp
	return p
}

func (p *residuePoint) MarshalSize() int {
	return (p.g.P.BitLen() + 7) / 8
}

func (p *residuePoint) MarshalBinary() ([]byte, error) {
	b := p.Int.Bytes() // may be shorter than len(buf)
	if pre := p.MarshalSize() - len(b); pre != 0 {
		return append(make([]byte, pre), b...), nil
	}
	return b, nil
}

func (p *residuePoint) UnmarshalBinary(data []byte) error {
	p.Int.SetBytes(data)
	if !p.Valid() {
		return errors.New("invalid Residue group element")
	}
	return nil
}

func (p *residuePoint) MarshalTo(w io.Writer) (int, error) {
	return marshalling.PointMarshalTo(p, w)
}

func (p *residuePoint) UnmarshalFrom(r io.Reader) (int, error) {
	return marshalling.PointUnmarshalFrom(p, r)
}

/*
A ResidueGroup represents a DSA-style modular integer arithmetic group,
defined by two primes P and Q and an integer R, such that P = Q*R+1.
Points in a ResidueGroup are R-residues modulo P,
and Scalars are integer exponents modulo the group order Q.

In traditional DSA groups P is typically much larger than Q,
and hence use a large multiple R.
This is done to minimize the computational cost of modular exponentiation
while maximizing security against known classes of attacks:
P must be on the order of thousands of bits long
while for security Q is believed to require only hundreds of bits.
Such computation-optimized groups are suitable
for Diffie-Hellman agreement, DSA or ElGamal signatures, etc.,
which depend on Point.Mul() and homomorphic properties.

However, residue groups with large R are less suitable for
public-key cryptographic techniques that require choosing Points
pseudo-randomly or to contain embedded data,
as required by ElGamal encryption for example.
For such purposes quadratic residue groups are more suitable -
representing the special case where R=2 and hence P=2Q+1.
As a result, the Point.Pick() method should be expected to work efficiently
ONLY on quadratic residue groups in which R=2.
*/
type ResidueGroup struct {
	dsa.Parameters
	R *big.Int
}

func (g *ResidueGroup) String() string {
	return fmt.Sprintf("Residue%d", g.P.BitLen())
}

// Return the number of bytes in the encoding of a Scalar
// for this Residue group.
func (g *ResidueGroup) ScalarLen() int { return (g.Q.BitLen() + 7) / 8 }

// Create a Scalar associated with this Residue group,
// with an initial value of nil.
func (g *ResidueGroup) Scalar() kyber.Scalar {
	return mod.NewInt64(0, g.Q)
}

// Return the number of bytes in the encoding of a Point
// for this Residue group.
func (g *ResidueGroup) PointLen() int { return (g.P.BitLen() + 7) / 8 }

// Create a Point associated with this Residue group,
// with an initial value of nil.
func (g *ResidueGroup) Point() kyber.Point {
	p := new(residuePoint)
	p.g = g
	return p
}

// Returns the order of this Residue group, namely the prime Q.
func (g *ResidueGroup) Order() *big.Int {
	return g.Q
}

// Validate the parameters for a Residue group,
// checking that P and Q are prime, P=Q*R+1,
// and that G is a valid generator for this group.
func (g *ResidueGroup) Valid() bool {

	// Make sure both P and Q are prime
	if !isPrime(g.P) || !isPrime(g.Q) {
		return false
	}

	// Validate the equation P = QR+1
	n := new(big.Int)
	n.Mul(g.Q, g.R)
	n.Add(n, one)
	if n.Cmp(g.P) != 0 {
		return false
	}

	// Validate the generator G
	if g.G.Cmp(one) <= 0 || n.Exp(g.G, g.Q, g.P).Cmp(one) != 0 {
		return false
	}

	return true
}

// Explicitly initialize a ResidueGroup with given parameters.
func (g *ResidueGroup) SetParams(P, Q, R, G *big.Int) {
	g.P = P
	g.Q = Q
	g.R = R
	g.G = G
	if !g.Valid() {
		panic("SetParams: bad Residue group parameters")
	}
}

// Initialize Residue group parameters for a quadratic residue group,
// by picking primes P and Q such that P=2Q+1
// and the smallest valid generator G for this group.
func (g *ResidueGroup) QuadraticResidueGroup(bitlen uint, rand cipher.Stream) {
	g.R = two

	// pick primes p,q such that p = 2q+1
	fmt.Printf("Generating %d-bit QR group", bitlen)
	for i := 0; ; i++ {
		if i > 1000 {
			print(".")
			i = 0
		}

		// First pick a prime Q
		b := random.Bits(bitlen-1, true, rand)
		b[len(b)-1] |= 1 // must be odd
		g.Q = new(big.Int).SetBytes(b)
		//println("q?",hex.EncodeToString(g.Q.Bytes()))
		if !isPrime(g.Q) {
			continue
		}

		// Does the corresponding P come out prime too?
		g.P = new(big.Int)
		g.P.Mul(g.Q, two)
		g.P.Add(g.P, one)
		//println("p?",hex.EncodeToString(g.P.Bytes()))
		if uint(g.P.BitLen()) == bitlen && isPrime(g.P) {
			break
		}
	}
	println()
	println("p", g.P.String())
	println("q", g.Q.String())

	// pick standard generator G
	h := new(big.Int).Set(two)
	g.G = new(big.Int)
	for {
		g.G.Exp(h, two, g.P)
		if g.G.Cmp(one) != 0 {
			break
		}
		h.Add(h, one)
	}
	println("g", g.G.String())
}
