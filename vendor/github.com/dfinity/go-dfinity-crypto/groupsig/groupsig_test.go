package groupsig

import (
	"math/big"
	"testing"

	"github.com/dfinity/go-dfinity-crypto/bls"
        "github.com/dfinity/go-dfinity-crypto/rand"
)

type Expect struct {
	bitLen int
	ok     []byte
}

func testPubkey(t *testing.T) {
	t.Log("testPubkey")
	r := rand.NewRand()
	sec := NewSeckeyFromRand(r.Deri(1))
	if sec == nil {
		t.Fatal("NewSeckeyFromRand")
	}
	pub := NewPubkeyFromSeckey(*sec)
	if pub == nil {
		t.Log("NewPubkeyFromSeckey")
	}
	{
		var pub2 Pubkey
		err := pub2.SetHexString(pub.GetHexString())
		if err != nil || !pub.IsEqual(pub2) {
			t.Log("pub != pub2")
		}
	}
	{
		var pub2 Pubkey
		err := pub2.Deserialize(pub.Serialize())
		if err != nil || !pub.IsEqual(pub2) {
			t.Log("pub != pub2")
		}
	}
}
func testComparison(t *testing.T) {
	t.Log("testComparison")
	var b = new(big.Int)
	b.SetString("16798108731015832284940804142231733909759579603404752749028378864165570215948", 10)
	sec := NewSeckeyFromBigInt(b)
	t.Log("sec.Hex: ", sec.GetHexString())
	t.Log("sec.DecimalString: ", sec.GetDecimalString())

	// Add Seckeys
	sum := AggregateSeckeys([]Seckey{*sec, *sec})
	if sum == nil {
		t.Log("AggregateSeckeys")
	}

	// Pubkey
	pub := NewPubkeyFromSeckey(*sec)
	if pub == nil {
		t.Log("NewPubkeyFromSeckey")
	}

	// Sig
	sig := Sign(*sec, []byte("hi"))
	asig := AggregateSigs([]Signature{sig, sig})
	if !VerifyAggregateSig([]Pubkey{*pub, *pub}, []byte("hi"), asig) {
		t.Error("Aggregated signature does not verify")
	}
	{
		var sig2 Signature
		err := sig2.SetHexString(sig.GetHexString())
		if err != nil || !sig.IsEqual(sig2) {
			t.Error("sig2.SetHexString")
		}
	}
	{
		var sig2 Signature
		err := sig2.Deserialize(sig.Serialize())
		if err != nil || !sig.IsEqual(sig2) {
			t.Error("sig2.Deserialize")
		}
	}
}

func testSeckey(t *testing.T) {
	t.Log("testSeckey")
	s := "401035055535747319451436327113007154621327258807739504261475863403006987855"
	var b = new(big.Int)
	b.SetString(s, 10)
	sec := NewSeckeyFromBigInt(b)
	{
		var sec2 Seckey
		err := sec2.SetHexString(sec.GetHexString())
		if err != nil || !sec.IsEqual(sec2) {
			t.Error("bad SetHexString")
		}
	}
	{
		var sec2 Seckey
		err := sec2.SetDecimalString(sec.GetDecimalString())
		if err != nil || !sec.IsEqual(sec2) {
			t.Error("bad DecimalString")
		}
		if s != sec.GetDecimalString() {
			t.Error("bad GetDecimalString")
		}
	}
	{
		var sec2 Seckey
		err := sec2.Deserialize(sec.Serialize())
		if err != nil || !sec.IsEqual(sec2) {
			t.Error("bad Serialize")
		}
	}
}

func testAggregation(t *testing.T) {
	t.Log("testAggregation")
	//    m := 5
	n := 3
	//    groupPubkeys := make([]Pubkey, m)
	r := rand.NewRand()
	seckeyContributions := make([]Seckey, n)
	for i := 0; i < n; i++ {
		seckeyContributions[i] = *NewSeckeyFromRand(r.Deri(i))
	}
	groupSeckey := AggregateSeckeys(seckeyContributions)
	groupPubkey := NewPubkeyFromSeckey(*groupSeckey)
	t.Log("Group pubkey:", groupPubkey.GetHexString())
}

func AggregateSeckeysByBigInt(secs []Seckey) *Seckey {
	secret := big.NewInt(0)
	for _, s := range secs {
		secret.Add(secret, s.GetBigInt())
	}
	secret.Mod(secret, curveOrder)
	return NewSeckeyFromBigInt(secret)
}

func testAggregateSeckeys(t *testing.T) {
	t.Log("testAggregateSeckeys")
	n := 100
	r := rand.NewRand()
	secs := make([]Seckey, n)
	// init secs
	for i := 0; i < n; i++ {
		secs[i] = *NewSeckeyFromRand(r.Deri(i))
	}
	s1 := AggregateSeckeysByBigInt(secs)
	s2 := AggregateSeckeys(secs)
	if !s1.value.IsEqual(&s2.value) {
		t.Errorf("not same %s %s\n", s1.GetHexString(), s2.GetHexString())
	}
}

func RecoverSeckeyByBigInt(secs []Seckey, ids []ID) *Seckey {
	secret := big.NewInt(0)
	k := len(secs)
	xs := make([]*big.Int, len(ids))
	for i := 0; i < len(xs); i++ {
		xs[i] = ids[i].GetBigInt()
	}
	// need len(ids) = k > 0
	for i := 0; i < k; i++ {
		// compute delta_i depending on ids only
		var delta, num, den, diff *big.Int = big.NewInt(1), big.NewInt(1), big.NewInt(1), big.NewInt(0)
		for j := 0; j < k; j++ {
			if j != i {
				num.Mul(num, xs[j])
				num.Mod(num, curveOrder)
				diff.Sub(xs[j], xs[i])
				den.Mul(den, diff)
				den.Mod(den, curveOrder)
			}
		}
		// delta = num / den
		den.ModInverse(den, curveOrder)
		delta.Mul(num, den)
		delta.Mod(delta, curveOrder)
		// apply delta to secs[i]
		delta.Mul(delta, secs[i].GetBigInt())
		// skip reducing delta modulo curveOrder here
		secret.Add(secret, delta)
		secret.Mod(secret, curveOrder)
	}
	return NewSeckeyFromBigInt(secret)
}

func testRecoverSeckey(t *testing.T) {
	t.Log("testRecoverSeckey")
	n := 50
	r := rand.NewRand()

	secs := make([]Seckey, n)
	ids := make([]ID, n)
	for i := 0; i < n; i++ {
		ids[i] = *NewIDFromInt64(int64(i + 3))
		secs[i] = *NewSeckeyFromRand(r.Deri(i))
	}
	s1 := RecoverSeckey(secs, ids)
	s2 := RecoverSeckeyByBigInt(secs, ids)
	if !s1.value.IsEqual(&s2.value) {
		t.Errorf("Mismatch in recovered secret key:\n  %s\n  %s.", s1.GetHexString(), s2.GetHexString())
	}
}

func ShareSeckeyByBigInt(msec []Seckey, id ID) *Seckey {
	secret := big.NewInt(0)
	// degree of polynomial, need k >= 1, i.e. len(msec) >= 2
	k := len(msec) - 1
	// msec = c_0, c_1, ..., c_k
	// evaluate polynomial f(x) with coefficients c0, ..., ck
	secret.Set(msec[k].GetBigInt())
	x := id.GetBigInt()
	for j := k - 1; j >= 0; j-- {
		secret.Mul(secret, x)
		//sec.secret.Mod(&sec.secret, curveOrder)
		secret.Add(secret, msec[j].GetBigInt())
		secret.Mod(secret, curveOrder)
	}
	return NewSeckeyFromBigInt(secret)
}

func testShareSeckey(t *testing.T) {
	t.Log("testShareSeckey")
	n := 100
	msec := make([]Seckey, n)
	r := rand.NewRand()
	for i := 0; i < n; i++ {
		msec[i] = *NewSeckeyFromRand(r.Deri(i))
	}
	id := *NewIDFromInt64(123)
	s1 := ShareSeckeyByBigInt(msec, id)
	s2 := ShareSeckey(msec, id)
	if !s1.value.IsEqual(&s2.value) {
		t.Errorf("bad sec\n%s\n%s", s1.GetHexString(), s2.GetHexString())
	}
}

func testID(t *testing.T) {
	t.Log("testString")
	b := new(big.Int)
	b.SetString("1234567890abcdef", 16)
	id1 := NewIDFromBigInt(b)
	if id1 == nil {
		t.Error("NewIDFromBigInt")
	}
	{
		var id2 ID
		err := id2.SetHexString(id1.GetHexString())
		if err != nil || !id1.IsEqual(id2) {
			t.Errorf("not same\n%s\n%s", id1.GetHexString(), id2.GetHexString())
		}
	}
	{
		var id2 ID
		err := id2.Deserialize(id1.Serialize())
		if err != nil || !id1.IsEqual(id2) {
			t.Errorf("not same\n%s\n%s", id1.GetHexString(), id2.GetHexString())
		}
	}
}

func test(t *testing.T, c int) {
	Init(c)
	testID(t)
	testSeckey(t)
	testPubkey(t)
	testAggregation(t)
	testComparison(t)
	testAggregateSeckeys(t)
	testRecoverSeckey(t)
	testShareSeckey(t)
}

func TestMain(t *testing.T) {
	t.Logf("GetMaxOpUnitSize() = %d\n", bls.GetMaxOpUnitSize())
	t.Log("CurveFp254BNb")
	test(t, bls.CurveFp254BNb)
	if bls.GetMaxOpUnitSize() == 6 {
		t.Log("CurveFp382_1")
		test(t, bls.CurveFp382_1)
		t.Log("CurveFp382_2")
		test(t, bls.CurveFp382_2)
	}
}
