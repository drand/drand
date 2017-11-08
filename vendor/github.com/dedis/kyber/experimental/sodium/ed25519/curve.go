// +build experimental
// +build sodium

// Package ed25519 implements Go wrappers for
// a high-speed implementation of the Ed25519 curve written in C,
// derived from the implementation in the Sodium crypto library.
package ed25519

// #include "sc.h"
// #include "ge.h"
// #include "fe.h"
//
// void ge_p3_neg(ge_p3 *r,ge_p3 *a) {
//	fe_neg(r->X,a->X);
//	fe_copy(r->Y,a->Y);
//	fe_copy(r->Z,a->Z);
//	fe_neg(r->T,a->T);
// }
//
// void ge_p3_add(ge_p3 *r,ge_p3 *a,ge_p3 *b) {
//	ge_cached bc;
//	ge_p1p1 t;
//	ge_p3_to_cached(&bc,b);
//	ge_add(&t,a,&bc);
//	ge_p1p1_to_p3(r,&t);
// }
//
// void ge_p3_sub(ge_p3 *r,ge_p3 *a,ge_p3 *b) {
//	ge_cached bc;
//	ge_p1p1 t;
//	ge_p3_to_cached(&bc,b);
//	ge_sub(&t,a,&bc);
//	ge_p1p1_to_p3(r,&t);
// }
//
import "C"

import (
	"bytes"
	"errors"
	"fmt"
	"hash"
	"io"
	"time"
	"unsafe"
	//"runtime"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"math/big"

	"github.com/dedis/kyber/abstract"
	"github.com/dedis/kyber/group"
	"github.com/dedis/kyber/nist"
	"github.com/dedis/kyber/random"
	"github.com/dedis/kyber/sha3"
)

// prime order of base point = 2^252 + 27742317777372353535851937790883648493
var primeOrder, _ = new(nist.Int).SetString("7237005577332262213973186563042994240857116359379907606001950938285454250989", "", 10)

// curve's cofactor
var cofactor = nist.NewInt64(8, &primeOrder.V)

var nullPoint = new(point).Null()

type point struct {
	p C.ge_p3
}

type curve struct {
}

// Convert little-endian byte slice to hex string
func tohex(s []byte) string {
	b := make([]byte, len(s))
	for i := range b { // byte-swap to big-endian for display
		b[i] = s[31-i]
	}
	return hex.EncodeToString(b)
}

func (p *point) String() string {
	return hex.EncodeToString(p.Encode())
}

func (p *point) Equal(p2 abstract.Point) bool {
	return bytes.Equal(p.Encode(), p2.(*point).Encode())
}

func (p *point) Null() abstract.Point {
	C.ge_p3_0(&p.p)
	return p
}

func (p *point) Base() abstract.Point {

	// Way to kill a fly with a sledgehammer...
	C.ge_scalarmult_base(&p.p, (*C.uchar)(unsafe.Pointer(&s1.b[0])))
	return p
}

func (p *point) PickLen() int {
	// Reserve at least 8 most-significant bits for randomness,
	// and the least-significant 8 bits for embedded data length.
	// (Hopefully it's unlikely we'll need >=2048-bit curves soon.)
	return (255 - 8 - 8) / 8
}

func (P *point) Pick(data []byte, rand cipher.Stream) (abstract.Point, []byte) {

	// How many bytes to embed?
	dl := P.PickLen()
	if dl > len(data) {
		dl = len(data)
	}

	for {
		// Pick a random point, with optional embedded data
		var b [32]byte
		rand.XORKeyStream(b[:], b[:])
		if data != nil {
			b[0] = byte(dl)       // Encode length in low 8 bits
			copy(b[1:1+dl], data) // Copy in data to embed
		}
		if C.ge_frombytes_vartime(&P.p, // Try to decode
			(*C.uchar)(unsafe.Pointer(&b[0]))) != 0 {
			continue // invalid point, retry
		}

		// We're using the prime-order subgroup,
		// so we need to make sure the point is in that subgroup.
		// If we're not trying to embed data,
		// we can convert our point into one in the subgroup
		// simply by multiplying it by the cofactor.
		if data == nil {
			P.Mul(P, cofactor) // multiply by cofactor
			if P.Equal(nullPoint) {
				continue // unlucky; try again
			}
			return P, data[dl:] // success
		}

		// Since we need the point's y-coordinate to hold our data,
		// we must simply check if the point is in the subgroup
		// and retry point generation until it is.
		var Q point
		Q.Mul(P, primeOrder)
		if Q.Equal(nullPoint) {
			return P, data[dl:] // success
		}

		// Keep trying...
	}
}

func (p *point) Data() ([]byte, error) {
	var b [32]byte
	C.ge_p3_tobytes((*C.uchar)(unsafe.Pointer(&b[0])), &p.p)
	dl := int(b[0]) // extract length byte
	if dl > p.PickLen() {
		return nil, errors.New("invalid embedded data length")
	}
	return b[1 : 1+dl], nil
}

func (p *point) Add(ca, cb abstract.Point) abstract.Point {
	a := ca.(*point)
	b := cb.(*point)
	C.ge_p3_add(&p.p, &a.p, &b.p)
	return p
}

func (p *point) Sub(ca, cb abstract.Point) abstract.Point {
	a := ca.(*point)
	b := cb.(*point)
	C.ge_p3_sub(&p.p, &a.p, &b.p)
	return p
}

func (p *point) Neg(ca abstract.Point) abstract.Point {
	a := ca.(*point)
	C.ge_p3_neg(&p.p, &a.p)
	return p
}

func (p *point) Mul(ca abstract.Point, cs abstract.Scalar) abstract.Point {

	// Convert the scalar to fixed-length little-endian form.
	sb := cs.(*nist.Int).V.Bytes()
	shi := len(sb) - 1
	var b [32]byte
	for i := range sb {
		b[shi-i] = sb[i]
	}

	if ca == nil {
		// Optimized multiplication by precomputed base point
		C.ge_scalarmult_base(&p.p, (*C.uchar)(unsafe.Pointer(&b[0])))
	} else {
		// General scalar multiplication
		a := ca.(*point)
		C.ge_scalarmult(&p.p, (*C.uchar)(unsafe.Pointer(&b[0])), &a.p)
	}
	return p
}

func (p *point) MarshalSize() int { return 32 }

func (p *point) MarshalBinary() ([]byte, error) {
	buf := [32]byte{}
	C.ge_p3_tobytes((*C.uchar)(unsafe.Pointer(&buf[0])), &p.p)
	return buf[:], nil
}

func (p *point) UnmarshalBinary(buf []byte) error {
	if len(buf) != 32 {
		return errors.New("curve25519 point wrong size")
	}
	if C.ge_frombytes_vartime(&p.p,
		(*C.uchar)(unsafe.Pointer(&buf[0]))) != 0 {
		return errors.New("curve25519 point invalid")
	}
	return nil
}

func (p *point) MarshalTo(w io.Writer) (int, error) {
	return group.PointMarshalTo(p, w)
}

func (p *Point) UnmarshalFrom(r io.Reader) (int, error) {
	return group.PointUnmarshalFrom(p, r)
}

func (p *point) validate() {
	//println("validating:")
	//p.dump()
	p2 := new(point)
	err := p2.Decode(p.Encode())
	if err != nil || !p2.Equal(p) {
		panic("invalid point")
	}
	//p2.(*point).dump()
}

func fetohex(fe *C.fe) string {
	b := [32]byte{}
	C.fe_tobytes((*C.uchar)(unsafe.Pointer(&b[0])), &fe[0])
	return tohex(b[:])
}

func (p *point) dump() {
	println("X", fetohex(&p.p.X))
	println("Y", fetohex(&p.p.Y))
	println("Z", fetohex(&p.p.Z))
	println("T", fetohex(&p.p.T))
}

func (c *curve) String() string {
	return "Curve25519"
}

func (c *curve) ScalarLen() int {
	return 32
}

func (c *curve) Scalar() abstract.Scalar {
	return nist.NewInt64(0, &primeOrder.V)
}

func (c *curve) PointLen() int {
	return 32
}

func (c *curve) Point() abstract.Point {
	return new(point)
}

func (c *curve) Order() *big.Int {
	return new(big.Int) // XXX
}

func (c *curve) PrimeOrder() bool {
	return true
}

func NewCurve25519() abstract.Group {
	return new(curve)
}

type suite struct {
	curve
}

// XXX non-NIST ciphers?

// SHA256 hash function
func (s *suite) Hash() hash.Hash {
	return sha256.New()
}

// AES128-CTR stream cipher
func (s *suite) Stream(key []byte) cipher.Stream {
	aes, err := aes.NewCipher(key)
	if err != nil {
		panic("can't instantiate AES: " + err.Error())
	}
	iv := make([]byte, 16)
	return cipher.NewCTR(aes, iv)
}

// SHA3/SHAKE128 sponge
func (s *suite) Sponge() abstract.Sponge {
	return sha3.NewSponge128()
}

// Ciphersuite based on AES-128, SHA-256, and the Ed25519 curve.
func NewAES128SHA256Ed25519() abstract.Suite {
	suite := new(suite)
	return suite
}

func TestCurve25519() {

	var x point

	p0 := point{}
	C.ge_p3_0(&p0.p)
	println("zero", p0.String())
	p0.validate()

	b := point{}
	b.Base()
	println("base", b.String())
	b.dump()
	b.validate()

	x.Base()
	x.Mul(&x, &s0)
	println("base*0", x.String())
	x.validate()

	x.Base()
	x.Mul(&x, &s1)
	println("base*1", x.String())
	x.validate()

	bx2 := point{}
	bx2.Mul(&b, &s2)
	println("base*2", bx2.String())
	bx2.validate()

	r := C.ge_p1p1{} // check against doubling function
	C.ge_p3_dbl(&r, &b.p)
	C.ge_p1p1_to_p3(&x.p, &r)
	println("base*2", x.String())
	x.validate()

	bx4 := point{}
	bx4.Mul(&b, &s4)
	println("base*4", bx4.String())
	bx4.validate()

	bx2x2 := point{}
	bx2x2.Mul(&bx2, &s2)
	println("base*2*2", bx2x2.String())
	bx2x2.validate()

	x.Add(&b, &p0)
	println("base+0", x.String())
	x.Add(&p0, &b)
	println("0+base", x.String())
	x.validate()
	x.validate()

	x.Add(&b, &b)
	println("base+base", x.String())
	//x.validate()

	x.Add(&b, &bx2)
	println("base+base*2", x.String())
	//x.validate()

	x.Add(&x, &b)
	println("base+base*3", x.String())
	//x.validate()

	//	g := NewCurve25519()
	//	crypto.TestGroup(g)
}

func BenchCurve25519() {

	g := NewCurve25519()

	// Point addition
	b := g.Point().Base()
	p := g.Point().Base()
	beg := time.Now()
	iters := 50000
	for i := 1; i < iters; i++ {
		p.Add(p, b)
	}
	end := time.Now()
	fmt.Printf("Point.Add: %f ops/sec\n",
		float64(iters)/
			(float64(end.Sub(beg))/1000000000.0))

	// Point encryption
	s := g.Scalar().Pick(random.Stream)
	beg = time.Now()
	iters = 5000
	for i := 1; i < iters; i++ {
		p.Mul(p, s)
	}
	end = time.Now()
	fmt.Printf("Point.Mul: %f ops/sec\n",
		float64(iters)/
			(float64(end.Sub(beg))/1000000000.0))
}
