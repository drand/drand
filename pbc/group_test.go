package pbc

import (
	"fmt"
	"testing"

	"gopkg.in/dedis/kyber.v1/util/test"

	"github.com/stretchr/testify/require"
)

func TestPrintConstants(t *testing.T) {
	t.Skip("test generating the generators")
	var p0 = NewPairingFp254BNb()
	p0g1 := p0.G1().Point().(*pointG1)
	printSeed(Fp254_G1_Base_Seed, &p0g1.g, t)
	p0g2 := p0.G2().Point().(*pointG2)
	printSeed(Fp254_G2_Base_Seed, &p0g2.g, t)

	fmt.Println()
	var p1 = NewPairingFp382_1()
	p1g1 := p1.G1().Point().(*pointG1)
	printSeed(Fp382_1_G1_Base_Seed, &p1g1.g, t)
	p1g2 := p1.G2().Point().(*pointG2)
	printSeed(Fp382_1_G2_Base_Seed, &p1g2.g, t)

	fmt.Println()
	var p2 = NewPairingFp382_2()
	p2g1 := p2.G1().Point().(*pointG1)
	printSeed(Fp382_2_G1_Base_Seed, &p2g1.g, t)
	p2g2 := p2.G2().Point().(*pointG2)
	printSeed(Fp382_2_G2_Base_Seed, &p2g2.g, t)

}

type hashmap interface {
	HashAndMapTo([]byte) error
	GetString(int) string
}

func printSeed(name string, h hashmap, t *testing.T) {
	err := h.HashAndMapTo([]byte(name))
	require.Nil(t, err)
	fmt.Println(name + " : " + h.GetString(16))
}

func TestG2(t *testing.T) {
	var p0 = NewPairingFp382_2()
	g2 := p0.GT()
	q1 := g2.Point().Base()  // q1 = base
	q2 := g2.Point().Neg(q1) // q2 =  -base
	s1 := g2.Scalar().SetInt64(-1)
	q3 := g2.Point().Mul(s1, q1) // q3 = (-1) * base

	if !q2.Equal(q3) {
		t.Fail()
	}

	q3.Add(q3, q1)
	if !q3.Equal(q2.Null()) {
		t.Fail()
	}

}

func TestP0(t *testing.T) {
	var p0 = NewPairingFp254BNb()
	test.GroupTest(p0.G1())
	//test.GroupTest(p0.G2())
	//test.GroupTest(p0.GT())
}

func TestP1(t *testing.T) {
	var p1 = NewPairingFp382_1()
	test.GroupTest(p1.G1())
	//test.GroupTest(p1.G2())
	//test.GroupTest(p1.GT())
}

func TestP2(t *testing.T) {
	//var p2 = NewPairingFp382_2()
	//test.GroupTest(p2.G2())
	//test.GroupTest(p2.G1())
	//test.GroupTest(p2.GT())
}
