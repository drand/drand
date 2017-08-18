package pbc

import (
	"crypto/cipher"
	"errors"
	"io"
	"runtime"

	"github.com/dfinity/go-dfinity-crypto/bls"

	"gopkg.in/dedis/kyber.v1"
	"gopkg.in/dedis/kyber.v1/util/random"
)

type pointG1 struct {
	g         bls.G1
	generator string
}

func newPointG1(gen string) *pointG1 {
	pg1 := &pointG1{g: bls.G1{}, generator: gen}
	runtime.SetFinalizer(&pg1.g, clear)
	return pg1
}

func (p *pointG1) Equal(p2 kyber.Point) bool {
	pg := p2.(*pointG1)
	return p.g.IsEqual(&pg.g)
}

func (p *pointG1) Null() kyber.Point {
	p.g.Clear()
	return p
}

func (p *pointG1) Base() kyber.Point {
	if err := p.g.HashAndMapTo([]byte(p.generator)); err != nil {
		panic(err)
	}
	return p
}

func (p *pointG1) Add(p1, p2 kyber.Point) kyber.Point {
	pg1 := p1.(*pointG1)
	pg2 := p2.(*pointG1)
	bls.G1Add(&p.g, &pg1.g, &pg2.g)
	return p
}

func (p *pointG1) Sub(p1, p2 kyber.Point) kyber.Point {
	pg1 := p1.(*pointG1)
	pg2 := p2.(*pointG1)
	bls.G1Sub(&p.g, &pg1.g, &pg2.g)
	return p
}

func (p *pointG1) Neg(p1 kyber.Point) kyber.Point {
	pg1 := p1.(*pointG1)
	bls.G1Neg(&p.g, &pg1.g)
	return p
}

func (p *pointG1) Mul(s kyber.Scalar, p1 kyber.Point) kyber.Point {
	if p1 == nil {
		p1 = newPointG1(p.generator).Base()
	}
	sc := s.(*scalar)
	pg1 := p1.(*pointG1)
	bls.G1Mul(&p.g, &pg1.g, &sc.fe)
	return p
}

func (p *pointG1) MarshalBinary() (buff []byte, err error) {
	return marshalBinary(&p.g)
}

func (p *pointG1) MarshalTo(w io.Writer) (int, error) {
	return PointMarshalTo(p, w)
}

func (p *pointG1) UnmarshalBinary(buff []byte) error {
	return p.g.Deserialize(buff)
}

func (p *pointG1) UnmarshalFrom(r io.Reader) (int, error) {
	return PointUnmarshalFrom(p, r)
}

func (p *pointG1) MarshalSize() int {
	return bls.GetOpUnitSize() * 8
}

func (p *pointG1) String() string {
	return p.g.GetString(16)
}

func (p *pointG1) Pick(rand cipher.Stream) kyber.Point {
	return p.Embed(nil, rand)
}

func (p *pointG1) EmbedLen() int {
	// 8 bits for the randomness and 8 bits for the size of the message
	return (bls.GetOpUnitSize() * 8) - 1 - 1
}

func (p *pointG1) Embed(data []byte, rand cipher.Stream) kyber.Point {
	embed(p, data, rand)
	return p
}

func (p *pointG1) Data() ([]byte, error) {
	return data(p)
}

func (p *pointG1) Clone() kyber.Point {
	p2 := clone(p, newPointG1(p.generator))
	return p2.(kyber.Point)
}

func (p *pointG1) Set(p2 kyber.Point) kyber.Point {
	clone(p2, p)
	return p
}

type pointG2 struct {
	g         bls.G2
	generator string
}

func newPointG2(gen string) *pointG2 {
	pg := &pointG2{g: bls.G2{}, generator: gen}
	runtime.SetFinalizer(&pg.g, clear)
	return pg
}

func (p *pointG2) Equal(p2 kyber.Point) bool {
	pg := p2.(*pointG2)
	return p.g.IsEqual(&pg.g)
}

func (p *pointG2) Null() kyber.Point {
	p.g.Clear()
	return p
}

func (p *pointG2) Base() kyber.Point {
	if err := p.g.HashAndMapTo([]byte(p.generator)); err != nil {
		panic(err)
	}
	return p
}

func (p *pointG2) Add(p1, p2 kyber.Point) kyber.Point {
	pg1 := p1.(*pointG2)
	pg2 := p2.(*pointG2)
	bls.G2Add(&p.g, &pg1.g, &pg2.g)
	return p
}

func (p *pointG2) Sub(p1, p2 kyber.Point) kyber.Point {
	pg1 := p1.(*pointG2)
	pg2 := p2.(*pointG2)
	bls.G2Sub(&p.g, &pg1.g, &pg2.g)
	return p
}

func (p *pointG2) Neg(p1 kyber.Point) kyber.Point {
	pg1 := p1.(*pointG2)
	bls.G2Neg(&p.g, &pg1.g)
	return p
}

func (p *pointG2) Mul(s kyber.Scalar, p1 kyber.Point) kyber.Point {
	if p1 == nil {
		p1 = newPointG2(p.generator).Base()
	}
	sc := s.(*scalar)
	pg1 := p1.(*pointG2)
	bls.G2Mul(&p.g, &pg1.g, &sc.fe)
	return p
}

func (p *pointG2) MarshalBinary() (buff []byte, err error) {
	return marshalBinary(&p.g)
}

func (p *pointG2) MarshalTo(w io.Writer) (int, error) {
	return PointMarshalTo(p, w)
}

func (p *pointG2) UnmarshalBinary(buff []byte) error {
	return p.g.Deserialize(buff)
}

func (p *pointG2) UnmarshalFrom(r io.Reader) (int, error) {
	return PointUnmarshalFrom(p, r)
}

func (p *pointG2) MarshalSize() int {
	return bls.GetOpUnitSize() * 8 * 2
}

func (p *pointG2) String() string {
	return p.g.GetString(16)
}

func (p *pointG2) Pick(rand cipher.Stream) kyber.Point {
	buff := random.NonZeroBytes(32, rand)
	if err := p.g.HashAndMapTo(buff); err != nil {
		panic(err)
	}
	return p

	//return p.Embed(nil, rand)
}

func (p *pointG2) EmbedLen() int {
	// 8 bits for the randomness and 8 bits for the size of the message
	return p.MarshalSize() - 1 - 1
}

func (p *pointG2) Embed(data []byte, rand cipher.Stream) kyber.Point {
	panic("not working for the moment")
	embed(p, data, rand)
	return p
}

func (p *pointG2) Clone() kyber.Point {
	p2 := clone(p, newPointG2(p.generator))
	return p2.(kyber.Point)
}

func (p *pointG2) Data() ([]byte, error) {
	return data(p)
}

func (p *pointG2) Set(p2 kyber.Point) kyber.Point {
	clone(p2, p)
	return p
}

type pointGT struct {
	g bls.GT
	p *Pairing
}

func newPointGT(p *Pairing) *pointGT {
	pg := &pointGT{g: bls.GT{}, p: p}
	runtime.SetFinalizer(&pg.g, clear)
	return pg
}

func (p *pointGT) Pairing(p1, p2 kyber.Point) kyber.Point {
	pg1 := p1.(*pointG1)
	pg2 := p2.(*pointG2)
	bls.Pairing(&p.g, &pg1.g, &pg2.g)
	return p
}

func (p *pointGT) Equal(p2 kyber.Point) bool {
	pg := p2.(*pointGT)
	return p.g.IsEqual(&pg.g)
}

func (p *pointGT) Null() kyber.Point {
	// multiplicative identity
	p.g.SetInt64(1)
	//p.g.Clear()
	return p
}

// Base point for GT is the point computed using the pairing operation
// over the base point of G1 and G2.
// XXX Is this desirable ? A fixed pre-computed point would be nicer.
// TODO precompute the pairing for each suite...
func (p *pointGT) Base() kyber.Point {
	g1 := p.p.G1().Point().Base()
	g2 := p.p.G2().Point().Base()
	return p.Pairing(g1, g2)
}

func (p *pointGT) Add(p1, p2 kyber.Point) kyber.Point {
	pg1 := p1.(*pointGT)
	pg2 := p2.(*pointGT)
	bls.GTMul(&p.g, &pg1.g, &pg2.g)
	return p
}

func (p *pointGT) Sub(p1, p2 kyber.Point) kyber.Point {
	pg1 := p1.(*pointGT)
	pg2 := p2.(*pointGT)
	bls.GTDiv(&p.g, &pg1.g, &pg2.g)
	return p
}

func (p *pointGT) Neg(p1 kyber.Point) kyber.Point {
	pg1 := p1.(*pointGT)
	bls.GTInv(&p.g, &pg1.g)
	return p
}

func (p *pointGT) Mul(s kyber.Scalar, p1 kyber.Point) kyber.Point {
	if p1 == nil {
		p1 = newPointGT(p.p).Base()
	}
	sc := s.(*scalar)
	pg1 := p1.(*pointGT)
	bls.GTPow(&p.g, &pg1.g, &sc.fe)
	return p
}

func (p *pointGT) MarshalBinary() (buff []byte, err error) {
	return marshalBinary(&p.g)
}

func (p *pointGT) MarshalTo(w io.Writer) (int, error) {
	return PointMarshalTo(p, w)
}

func (p *pointGT) UnmarshalBinary(buff []byte) error {
	return p.g.Deserialize(buff)
}

func (p *pointGT) UnmarshalFrom(r io.Reader) (int, error) {
	return PointUnmarshalFrom(p, r)
}

func (p *pointGT) MarshalSize() int {
	return bls.GetOpUnitSize() * 8 * 12
}

func (p *pointGT) String() string {
	return p.g.GetString(16)
}

func (p *pointGT) Pick(rand cipher.Stream) kyber.Point {
	return p.Embed(nil, rand)
}

func (p *pointGT) EmbedLen() int {
	// 8 bits for the randomness and 8 bits for the size of the message
	return (bls.GetOpUnitSize() * 8 * 12) - 1 - 1
}

func (p *pointGT) Embed(data []byte, rand cipher.Stream) kyber.Point {
	embed(p, data, rand)
	return p
}

func (p *pointGT) Clone() kyber.Point {
	p2 := clone(p, newPointGT(p.p))
	return p2.(kyber.Point)
}

func (p *pointGT) Data() ([]byte, error) {
	return data(p)
}

func (p *pointGT) Set(p2 kyber.Point) kyber.Point {
	clone(p2, p)
	return p
}

type pbcPoint interface {
	kyber.Marshaling
	EmbedLen() int
}

type serializable interface {
	Serialize() []byte
}

type clearable interface {
	Clear()
}

func marshalBinary(p serializable) (buff []byte, err error) {
	defer func() {
		if e := recover(); e != nil {
			buff = nil
			err = e.(error)
		}
	}()
	buff = p.Serialize()
	return

}

func embed(p pbcPoint, data []byte, rand cipher.Stream) {
	buffSize := p.MarshalSize() // how much data + len + random can we embed
	embedSize := p.EmbedLen()   // how much data can we embed
	if embedSize > len(data) {
		embedSize = len(data)
	}

	for {
		// try filling in random bytes
		// XXX could be optimized by keeping one buffer and doing the "random"
		// part ourselves.
		buff := random.NonZeroBytes(buffSize, rand)
		if data != nil {
			buff[0] = byte(embedSize)       // encode length in low 8 bits
			copy(buff[1:1+embedSize], data) // copy data
		}

		err := p.UnmarshalBinary(buff)
		if err != nil {
			// no luck, try again
			continue
		}

		// Points live in a prime order curve so no cofactor-thing needed. All ok.
		return
	}
}

func clone(p, p2 pbcPoint) pbcPoint {
	buff, err := p.MarshalBinary()
	if err != nil {
		panic(err)
	}
	err = p2.UnmarshalBinary(buff)
	if err != nil {
		panic(err)
	}
	return p2
}

func data(p pbcPoint) ([]byte, error) {
	buff, _ := p.MarshalBinary()
	dl := int(buff[0]) // extract length byte
	if dl > p.EmbedLen() {
		return nil, errors.New("invalid embedded data length")
	}
	return buff[1 : 1+dl], nil
}

func clear(p clearable) {
	p.Clear()
}

var ErrVarTime = errors.New("no constant time implementation available")

func (p *pointG1) SetVarTime(varTime bool) error {
	return ErrVarTime
}

func (p *pointG2) SetVarTime(varTime bool) error {
	return ErrVarTime
}
func (p *pointGT) SetVarTime(varTime bool) error {
	return ErrVarTime
}

// PointMarshalTo provides a generic implementation of Point.EncodeTo
// based on Point.Encode.
func PointMarshalTo(p kyber.Point, w io.Writer) (int, error) {
	buf, err := p.MarshalBinary()
	if err != nil {
		return 0, err
	}
	return w.Write(buf)
}

// PointUnmarshalFrom provides a generic implementation of Point.DecodeFrom,
// based on Point.Decode, or Point.Pick if r is a Cipher or cipher.Stream.
// The returned byte-count is valid only when decoding from a normal Reader,
// not when picking from a pseudorandom source.
func PointUnmarshalFrom(p kyber.Point, r io.Reader) (int, error) {
	if strm, ok := r.(cipher.Stream); ok {
		p.Pick(strm)
		return -1, nil // no byte-count when picking randomly
	}
	buf := make([]byte, p.MarshalSize())
	n, err := io.ReadFull(r, buf)
	if err != nil {
		return n, err
	}
	return n, p.UnmarshalBinary(buf)
}

// ScalarMarshalTo provides a generic implementation of Scalar.EncodeTo
// based on Scalar.Encode.
func ScalarMarshalTo(s kyber.Scalar, w io.Writer) (int, error) {
	buf, err := s.MarshalBinary()
	if err != nil {
		return 0, err
	}
	return w.Write(buf)
}

// ScalarUnmarshalFrom provides a generic implementation of Scalar.DecodeFrom,
// based on Scalar.Decode, or Scalar.Pick if r is a Cipher or cipher.Stream.
// The returned byte-count is valid only when decoding from a normal Reader,
// not when picking from a pseudorandom source.
func ScalarUnmarshalFrom(s kyber.Scalar, r io.Reader) (int, error) {
	if strm, ok := r.(cipher.Stream); ok {
		s.Pick(strm)
		return -1, nil // no byte-count when picking randomly
	}
	buf := make([]byte, s.MarshalSize())
	n, err := io.ReadFull(r, buf)
	if err != nil {
		return n, err
	}
	return n, s.UnmarshalBinary(buf)
}
