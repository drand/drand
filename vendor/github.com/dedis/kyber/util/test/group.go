package test

import (
	"github.com/dedis/kyber"
	"github.com/dedis/kyber/util/random"
)

// GroupBench is a generic benchmark suite for kyber.groups.
type GroupBench struct {
	g kyber.Group

	// Random secrets and points for testing
	x, y kyber.Scalar
	X, Y kyber.Point
	xe   []byte // encoded Scalar
	Xe   []byte // encoded Point
}

// NewGroupBench returns a new GroupBench.
func NewGroupBench(g kyber.Group) *GroupBench {
	var gb GroupBench
	gb.g = g
	gb.x = g.Scalar().Pick(random.Stream)
	gb.y = g.Scalar().Pick(random.Stream)
	gb.xe, _ = gb.x.MarshalBinary()
	gb.X = g.Point().Pick(random.Stream)
	gb.Y = g.Point().Pick(random.Stream)
	gb.Xe, _ = gb.X.MarshalBinary()
	return &gb
}

// ScalarAdd benchmarks the addition operation for scalars
func (gb GroupBench) ScalarAdd(iters int) {
	for i := 1; i < iters; i++ {
		gb.x.Add(gb.x, gb.y)
	}
}

// ScalarSub benchmarks the substraction operation for scalars
func (gb GroupBench) ScalarSub(iters int) {
	for i := 1; i < iters; i++ {
		gb.x.Sub(gb.x, gb.y)
	}
}

// ScalarNeg benchmarks the negation operation for scalars
func (gb GroupBench) ScalarNeg(iters int) {
	for i := 1; i < iters; i++ {
		gb.x.Neg(gb.x)
	}
}

// ScalarMul benchmarks the multiplication operation for scalars
func (gb GroupBench) ScalarMul(iters int) {
	for i := 1; i < iters; i++ {
		gb.x.Mul(gb.x, gb.y)
	}
}

// ScalarDiv benchmarks the division operation for scalars
func (gb GroupBench) ScalarDiv(iters int) {
	for i := 1; i < iters; i++ {
		gb.x.Div(gb.x, gb.y)
	}
}

// ScalarInv benchmarks the inverse operation for scalars
func (gb GroupBench) ScalarInv(iters int) {
	for i := 1; i < iters; i++ {
		gb.x.Inv(gb.x)
	}
}

// ScalarPick benchmarks the Pick operation for scalars
func (gb GroupBench) ScalarPick(iters int) {
	for i := 1; i < iters; i++ {
		gb.x.Pick(random.Stream)
	}
}

// ScalarEncode benchmarks the marshalling operation for scalars
func (gb GroupBench) ScalarEncode(iters int) {
	for i := 1; i < iters; i++ {
		_, _ = gb.x.MarshalBinary()
	}
}

// ScalarDecode benchmarks the unmarshalling operation for scalars
func (gb GroupBench) ScalarDecode(iters int) {
	for i := 1; i < iters; i++ {
		_ = gb.x.UnmarshalBinary(gb.xe)
	}
}

// PointAdd benchmarks the addition operation for points
func (gb GroupBench) PointAdd(iters int) {
	for i := 1; i < iters; i++ {
		gb.X.Add(gb.X, gb.Y)
	}
}

// PointSub benchmarks the substraction operation for points
func (gb GroupBench) PointSub(iters int) {
	for i := 1; i < iters; i++ {
		gb.X.Sub(gb.X, gb.Y)
	}
}

// PointNeg benchmarks the negation operation for points
func (gb GroupBench) PointNeg(iters int) {
	for i := 1; i < iters; i++ {
		gb.X.Neg(gb.X)
	}
}

// PointMul benchmarks the multiplication operation for points
func (gb GroupBench) PointMul(iters int) {
	for i := 1; i < iters; i++ {
		gb.X.Mul(gb.y, gb.X)
	}
}

// PointBaseMul benchmarks the base multiplication operation for points
func (gb GroupBench) PointBaseMul(iters int) {
	for i := 1; i < iters; i++ {
		gb.X.Mul(gb.y, nil)
	}
}

// PointPick benchmarks the pick-ing operation for points
func (gb GroupBench) PointPick(iters int) {
	for i := 1; i < iters; i++ {
		gb.X.Pick(random.Stream)
	}
}

// PointEncode benchmarks the encoding operation for points
func (gb GroupBench) PointEncode(iters int) {
	for i := 1; i < iters; i++ {
		_, _ = gb.X.MarshalBinary()
	}
}

// PointDecode benchmarks the decoding operation for points
func (gb GroupBench) PointDecode(iters int) {
	for i := 1; i < iters; i++ {
		_ = gb.X.UnmarshalBinary(gb.Xe)
	}
}
