package bn256

import (
	"crypto/cipher"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/group/mod"
)

type groupG1 struct {
	common
	*commonSuite
}

func (g *groupG1) String() string {
	return "bn256_G1"
}

func (g *groupG1) PointLen() int {
	return newPointG1(g).MarshalSize()
}

func (g *groupG1) Scalar() kyber.Scalar {
	return &scalarDescribing{
		Int:   g.common.Scalar().(*mod.Int),
		group: g,
	}
}

func (g *groupG1) Point() kyber.Point {
	return newPointG1(g)
}

type groupG2 struct {
	common
	*commonSuite
}

func (g *groupG2) String() string {
	return "BN256_G2"
}

func (g *groupG2) PointLen() int {
	return newPointG2(g).MarshalSize()
}

func (g *groupG2) Point() kyber.Point {
	return newPointG2(g)
}

func (g *groupG2) Scalar() kyber.Scalar {
	return &scalarDescribing{
		Int:   g.common.Scalar().(*mod.Int),
		group: g,
	}
}

type groupGT struct {
	common
	*commonSuite
}

func (g *groupGT) String() string {
	return "BN256_GT"
}

func (g *groupGT) PointLen() int {
	return newPointGT(g).MarshalSize()
}

func (g *groupGT) Point() kyber.Point {
	return newPointGT(g)
}

func (g *groupGT) Scalar() kyber.Scalar {
	return &scalarDescribing{
		Int:   g.common.Scalar().(*mod.Int),
		group: g,
	}
}

// common functionalities across G1, G2, and GT
type common struct{}

func (c *common) ScalarLen() int {
	return mod.NewInt64(0, Order).MarshalSize()
}

func (c *common) Scalar() kyber.Scalar {
	return mod.NewInt64(0, Order)
}

func (c *common) PrimeOrder() bool {
	return true
}

func (c *common) NewKey(rand cipher.Stream) kyber.Scalar {
	return mod.NewInt64(0, Order).Pick(rand)
}

type scalarDescribing struct {
	*mod.Int
	group kyber.Group
}

func (s *scalarDescribing) Group() kyber.Group {
	return s.group
}

func (s *scalarDescribing) Equal(s2 kyber.Scalar) bool {
	return s.Int.Equal(s2.(*scalarDescribing).Int)
}

func (s *scalarDescribing) Set(a kyber.Scalar) kyber.Scalar {
	s.Int.Set(a.(*scalarDescribing).Int)
	return s
}

func (s *scalarDescribing) Add(a, b kyber.Scalar) kyber.Scalar {
	s.Int.Add(a.(*scalarDescribing).Int, b.(*scalarDescribing).Int)
	return s
}

func (s *scalarDescribing) Sub(a, b kyber.Scalar) kyber.Scalar {
	s.Int.Sub(a.(*scalarDescribing).Int, b.(*scalarDescribing).Int)
	return s
}

func (s *scalarDescribing) Neg(a kyber.Scalar) kyber.Scalar {
	s.Int.Neg(a.(*scalarDescribing).Int)
	return s
}

func (s *scalarDescribing) Mul(a, b kyber.Scalar) kyber.Scalar {
	s.Int.Mul(a.(*scalarDescribing).Int, b.(*scalarDescribing).Int)
	return s
}

func (s *scalarDescribing) Div(a, b kyber.Scalar) kyber.Scalar {
	s.Int.Div(a.(*scalarDescribing).Int, b.(*scalarDescribing).Int)
	return s
}

func (s *scalarDescribing) Inv(a kyber.Scalar) kyber.Scalar {
	s.Int.Inv(a.(*scalarDescribing).Int)
	return s
}

func (s *scalarDescribing) Pick(rand cipher.Stream) kyber.Scalar {
	s.Int.Pick(rand)
	return s
}

func (s *scalarDescribing) Clone() kyber.Scalar {
	s2 := s.Int.Clone()
	return &scalarDescribing{
		Int:   s2.(*mod.Int),
		group: s.group,
	}
}

func (s *scalarDescribing) SetInt64(v int64) kyber.Scalar {
	s.Int.SetInt64(v)
	return s
}

func (s *scalarDescribing) Zero() kyber.Scalar {
	s.Int.Zero()
	return s
}

func (s *scalarDescribing) One() kyber.Scalar {
	s.Int.One()
	return s
}

func (s *scalarDescribing) SetBytes(buff []byte) kyber.Scalar {
	s.Int.SetBytes(buff)
	return s
}
