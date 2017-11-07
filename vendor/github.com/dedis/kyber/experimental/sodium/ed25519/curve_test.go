// +build experimental
// +build sodium

package ed25519

import (
	"testing"

	"github.com/dedis/kyber/test"
)

var testSuite = NewAES128SHA256Ed25519()

func TestGroup(t *testing.T) {
	test.TestSuite(testSuite)
}

func BenchmarkScalarAdd(b *testing.B) {
	test.NewGroupBench(testSuite).ScalarAdd(b.N)
}

func BenchmarkScalarSub(b *testing.B) {
	test.NewGroupBench(testSuite).ScalarSub(b.N)
}

func BenchmarkScalarNeg(b *testing.B) {
	test.NewGroupBench(testSuite).ScalarNeg(b.N)
}

func BenchmarkScalarMul(b *testing.B) {
	test.NewGroupBench(testSuite).ScalarMul(b.N)
}

func BenchmarkScalarDiv(b *testing.B) {
	test.NewGroupBench(testSuite).ScalarDiv(b.N)
}

func BenchmarkScalarInv(b *testing.B) {
	test.NewGroupBench(testSuite).ScalarInv(b.N)
}

func BenchmarkScalarPick(b *testing.B) {
	test.NewGroupBench(testSuite).ScalarPick(b.N)
}

func BenchmarkScalarEncode(b *testing.B) {
	test.NewGroupBench(testSuite).ScalarEncode(b.N)
}

func BenchmarkScalarDecode(b *testing.B) {
	test.NewGroupBench(testSuite).ScalarDecode(b.N)
}

func BenchmarkPointAdd(b *testing.B) {
	test.NewGroupBench(testSuite).PointAdd(b.N)
}

func BenchmarkPointSub(b *testing.B) {
	test.NewGroupBench(testSuite).PointSub(b.N)
}

func BenchmarkPointNeg(b *testing.B) {
	test.NewGroupBench(testSuite).PointNeg(b.N)
}

func BenchmarkPointMul(b *testing.B) {
	test.NewGroupBench(testSuite).PointMul(b.N)
}

func BenchmarkPointBaseMul(b *testing.B) {
	test.NewGroupBench(testSuite).PointBaseMul(b.N)
}

func BenchmarkPointPick(b *testing.B) {
	test.NewGroupBench(testSuite).PointPick(b.N)
}

func BenchmarkPointEncode(b *testing.B) {
	test.NewGroupBench(testSuite).PointEncode(b.N)
}

func BenchmarkPointDecode(b *testing.B) {
	test.NewGroupBench(testSuite).PointDecode(b.N)
}
