package bls

import "testing"
import "fmt"

func TestBadPointOfG2(t *testing.T) {
	err := Init(CurveFp382_2)
	if err != nil {
		t.Fatal(err)
	}
	var Q G2
	// this value is not in G2 so should return an error
	err = Q.SetString("1 18d3d8c085a5a5e7553c3a4eb628e88b8465bf4de2612e35a0a4eb018fb0c82e9698896031e62fd7633ffd824a859474 1dc6edfcf33e29575d4791faed8e7203832217423bf7f7fbf1f6b36625b12e7132c15fbc15562ce93362a322fb83dd0d 65836963b1f7b6959030ddfa15ab38ce056097e91dedffd996c1808624fa7e2644a77be606290aa555cda8481cfb3cb 1b77b708d3d4f65aeedf54b58393463a42f0dc5856baadb5ce608036baeca398c5d9e6b169473a8838098fd72fd28b50", 16)
	if err == nil {
		t.Error(err)
	}
}

func TestGT(t *testing.T) {
	var x GT
	x.Clear()
	if !x.IsZero() {
		t.Errorf("not zero")
	}
	x.SetInt64(1)
	if !x.IsOne() {
		t.Errorf("not one")
	}
}

func testNegAdd(t *testing.T) {
	var x Fr
	var P1, P2, P3 G1
	var Q1, Q2, Q3 G2
	err := P1.HashAndMapTo([]byte("this"))
	if err != nil {
		t.Error(err)
	}
	err = Q1.HashAndMapTo([]byte("this"))
	if err != nil {
		t.Error(err)
	}
	fmt.Printf("P1=%s\n", P1.GetString(16))
	fmt.Printf("Q1=%s\n", Q1.GetString(16))
	G1Neg(&P2, &P1)
	G2Neg(&Q2, &Q1)
	fmt.Printf("P2=%s\n", P2.GetString(16))
	fmt.Printf("Q2=%s\n", Q2.GetString(16))

	x.SetInt64(-1)
	G1Mul(&P3, &P1, &x)
	G2Mul(&Q3, &Q1, &x)
	if !P2.IsEqual(&P3) {
		t.Errorf("P2 != P3 %s\n", P3.GetString(16))
	}
	if !Q2.IsEqual(&Q3) {
		t.Errorf("Q2 != Q3 %s\n", Q3.GetString(16))
	}

	G1Add(&P2, &P2, &P1)
	G2Add(&Q2, &Q2, &Q1)
	if !P2.IsZero() {
		t.Errorf("P2 is not zero %s\n", P2.GetString(16))
	}
	if !Q2.IsZero() {
		t.Errorf("Q2 is not zero %s\n", Q2.GetString(16))
	}
}

func testPairing(t *testing.T) {
	var a, b, ab Fr
	err := a.SetString("123", 10)
	if err != nil {
		t.Error(err)
		return
	}
	err = b.SetString("456", 10)
	if err != nil {
		t.Error(err)
		return
	}
	FrMul(&ab, &a, &b)
	var P, aP G1
	var Q, bQ G2
	err = P.HashAndMapTo([]byte("this"))
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("P=%s\n", P.GetString(16))
	G1Mul(&aP, &P, &a)
	fmt.Printf("aP=%s\n", aP.GetString(16))
	err = Q.HashAndMapTo([]byte("that"))
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("Q=%s\n", Q.GetString(16))
	G2Mul(&bQ, &Q, &b)
	fmt.Printf("bQ=%s\n", bQ.GetString(16))
	var e1, e2 GT
	Pairing(&e1, &P, &Q)
	fmt.Printf("e1=%s\n", e1.GetString(16))
	Pairing(&e2, &aP, &bQ)
	fmt.Printf("e2=%s\n", e1.GetString(16))
	GTPow(&e1, &e1, &ab)
	fmt.Printf("e1=%s\n", e1.GetString(16))
	if !e1.IsEqual(&e2) {
		t.Errorf("not equal pairing\n%s\n%s", e1.GetString(16), e2.GetString(16))
	}
}

func testMcl(t *testing.T, c int) {
	err := Init(c)
	if err != nil {
		t.Fatal(err)
	}
	testNegAdd(t)
	testPairing(t)
}

func TestMclMain(t *testing.T) {
	t.Logf("GetMaxOpUnitSize() = %d\n", GetMaxOpUnitSize())
	t.Log("CurveFp254BNb")
	testMcl(t, CurveFp254BNb)
	if GetMaxOpUnitSize() == 6 {
		t.Log("CurveFp382_1")
		testMcl(t, CurveFp382_1)
		t.Log("CurveFp382_2")
		testMcl(t, CurveFp382_2)
	}
}
