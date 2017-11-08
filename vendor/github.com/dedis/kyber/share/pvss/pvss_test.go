package pvss

import (
	"testing"

	"github.com/dedis/kyber"
	"github.com/dedis/kyber/group/edwards25519"
	"github.com/dedis/kyber/util/random"
	"github.com/stretchr/testify/require"
)

func TestPVSS(test *testing.T) {
	suite := edwards25519.NewAES128SHA256Ed25519()
	G := suite.Point().Base()
	H := suite.Point().Pick(suite.Cipher([]byte("H")))
	n := 10
	t := 2*n/3 + 1
	x := make([]kyber.Scalar, n) // trustee private keys
	X := make([]kyber.Point, n)  // trustee public keys
	for i := 0; i < n; i++ {
		x[i] = suite.Scalar().Pick(random.Stream)
		X[i] = suite.Point().Mul(x[i], nil)
	}

	// Scalar of shared secret
	secret := suite.Scalar().Pick(random.Stream)

	// (1) Share distribution (dealer)
	encShares, pubPoly, err := EncShares(suite, H, X, secret, t)
	require.Equal(test, err, nil)

	// (2) Share decryption (trustees)
	sH := make([]kyber.Point, n)
	for i := 0; i < n; i++ {
		sH[i] = pubPoly.Eval(encShares[i].S.I).V
	}

	var K []kyber.Point  // good public keys
	var E []*PubVerShare // good encrypted shares
	var D []*PubVerShare // good decrypted shares

	for i := 0; i < n; i++ {
		if ds, err := DecShare(suite, H, X[i], sH[i], x[i], encShares[i]); err == nil {
			K = append(K, X[i])
			E = append(E, encShares[i])
			D = append(D, ds)
		}
	}

	// (3) Check decrypted shares and recover secret if possible (dealer/3rd party)
	recovered, err := RecoverSecret(suite, G, K, E, D, t, n)
	require.Equal(test, err, nil)
	require.True(test, suite.Point().Mul(secret, nil).Equal(recovered))
}

func TestPVSSDelete(test *testing.T) {
	suite := edwards25519.NewAES128SHA256Ed25519()
	G := suite.Point().Base()
	H := suite.Point().Pick(suite.Cipher([]byte("H")))
	n := 10
	t := 2*n/3 + 1
	x := make([]kyber.Scalar, n) // trustee private keys
	X := make([]kyber.Point, n)  // trustee public keys
	for i := 0; i < n; i++ {
		x[i] = suite.Scalar().Pick(random.Stream)
		X[i] = suite.Point().Mul(x[i], nil)
	}

	// Scalar of shared secret
	secret := suite.Scalar().Pick(random.Stream)

	// (1) Share distribution (dealer)
	encShares, pubPoly, err := EncShares(suite, H, X, secret, t)
	require.Equal(test, err, nil)

	// Corrupt some of the encrypted shares
	encShares[0].S.V = suite.Point().Null()
	encShares[5].S.V = suite.Point().Null()

	// (2) Share decryption (trustees)
	sH := make([]kyber.Point, n)
	for i := 0; i < n; i++ {
		sH[i] = pubPoly.Eval(encShares[i].S.I).V
	}

	var K []kyber.Point  // good public keys
	var E []*PubVerShare // good encrypted shares
	var D []*PubVerShare // good decrypted shares

	for i := 0; i < n; i++ {
		if ds, err := DecShare(suite, H, X[i], sH[i], x[i], encShares[i]); err == nil {
			K = append(K, X[i])
			E = append(E, encShares[i])
			D = append(D, ds)
		}
	}

	// Corrupt some of the decrypted shares
	D[1].S.V = suite.Point().Null()

	// (3) Check decrypted shares and recover secret if possible (dealer/3rd party)
	recovered, err := RecoverSecret(suite, G, K, E, D, t, n)
	require.Equal(test, err, nil)
	require.True(test, suite.Point().Mul(secret, nil).Equal(recovered))
}

func TestPVSSDeleteFail(test *testing.T) {
	suite := edwards25519.NewAES128SHA256Ed25519()
	G := suite.Point().Base()
	H := suite.Point().Pick(suite.Cipher([]byte("H")))
	n := 10
	t := 2*n/3 + 1
	x := make([]kyber.Scalar, n) // trustee private keys
	X := make([]kyber.Point, n)  // trustee public keys
	for i := 0; i < n; i++ {
		x[i] = suite.Scalar().Pick(random.Stream)
		X[i] = suite.Point().Mul(x[i], nil)
	}

	// Scalar of shared secret
	secret := suite.Scalar().Pick(random.Stream)

	// (1) Share distribution (dealer)
	encShares, pubPoly, err := EncShares(suite, H, X, secret, t)
	require.Equal(test, err, nil)

	// Corrupt some of the encrypted shares
	encShares[0].S.V = suite.Point().Null()
	encShares[5].S.V = suite.Point().Null()

	// (2) Share decryption (trustees)
	sH := make([]kyber.Point, n)
	for i := 0; i < n; i++ {
		sH[i] = pubPoly.Eval(encShares[i].S.I).V
	}

	var K []kyber.Point  // good public keys
	var E []*PubVerShare // good encrypted shares
	var D []*PubVerShare // good decrypted shares

	for i := 0; i < n; i++ {
		if ds, err := DecShare(suite, H, X[i], sH[i], x[i], encShares[i]); err == nil {
			K = append(K, X[i])
			E = append(E, encShares[i])
			D = append(D, ds)
		}
	}

	// Corrupt enough decrypted shares to make the secret unrecoverable
	D[0].S.V = suite.Point().Null()
	D[1].S.V = suite.Point().Null()

	// (3) Check decrypted shares and recover secret if possible (dealer/3rd party)
	_, err = RecoverSecret(suite, G, K, E, D, t, n)
	require.Equal(test, err, errorTooFewShares) // this test is supposed to fail
}

func TestPVSSBatch(test *testing.T) {
	suite := edwards25519.NewAES128SHA256Ed25519()
	G := suite.Point().Base()
	H := suite.Point().Pick(suite.Cipher([]byte("H")))
	n := 5
	t := 2*n/3 + 1
	x := make([]kyber.Scalar, n) // trustee private keys
	X := make([]kyber.Point, n)  // trustee public keys
	for i := 0; i < n; i++ {
		x[i] = suite.Scalar().Pick(random.Stream)
		X[i] = suite.Point().Mul(x[i], nil)
	}

	// (1) Share distribution (multiple dealers)
	s0 := suite.Scalar().Pick(random.Stream)
	e0, p0, err := EncShares(suite, H, X, s0, t)
	require.Equal(test, err, nil)

	s1 := suite.Scalar().Pick(random.Stream)
	e1, p1, err := EncShares(suite, H, X, s1, t)
	require.Equal(test, err, nil)

	s2 := suite.Scalar().Pick(random.Stream)
	e2, p2, err := EncShares(suite, H, X, s2, t)
	require.Equal(test, err, nil)

	sH0 := make([]kyber.Point, n)
	sH1 := make([]kyber.Point, n)
	sH2 := make([]kyber.Point, n)
	for i := 0; i < n; i++ {
		sH0[i] = p0.Eval(e0[i].S.I).V
		sH1[i] = p1.Eval(e1[i].S.I).V
		sH2[i] = p2.Eval(e2[i].S.I).V
	}

	// Batch verification
	X0, E0, err := VerifyEncShareBatch(suite, H, X, sH0, e0)
	require.Equal(test, err, nil)

	X1, E1, err := VerifyEncShareBatch(suite, H, X, sH1, e1)
	require.Equal(test, err, nil)

	X2, E2, err := VerifyEncShareBatch(suite, H, X, sH2, e2)
	require.Equal(test, err, nil)

	// Reorder (some) poly evals, keys, and shares
	P0 := []kyber.Point{p0.Eval(E0[0].S.I).V, p1.Eval(E1[0].S.I).V, p2.Eval(E2[0].S.I).V}
	P1 := []kyber.Point{p0.Eval(E0[1].S.I).V, p1.Eval(E1[1].S.I).V, p2.Eval(E2[1].S.I).V}
	P2 := []kyber.Point{p0.Eval(E0[2].S.I).V, p1.Eval(E1[2].S.I).V, p2.Eval(E2[2].S.I).V}
	P3 := []kyber.Point{p0.Eval(E0[3].S.I).V, p1.Eval(E1[3].S.I).V, p2.Eval(E2[3].S.I).V}

	Y0 := []kyber.Point{X0[0], X1[0], X2[0]}
	Y1 := []kyber.Point{X0[1], X1[1], X2[1]}
	Y2 := []kyber.Point{X0[2], X1[2], X2[2]}
	Y3 := []kyber.Point{X0[3], X1[3], X2[3]}

	Z0 := []*PubVerShare{E0[0], E1[0], E2[0]}
	Z1 := []*PubVerShare{E0[1], E1[1], E2[1]}
	Z2 := []*PubVerShare{E0[2], E1[2], E2[2]}
	Z3 := []*PubVerShare{E0[3], E1[3], E2[3]}

	// (2) Share batch decryption (trustees)
	KD0, ED0, DD0, err := DecShareBatch(suite, H, Y0, P0, x[0], Z0)
	require.Equal(test, err, nil)

	KD1, ED1, DD1, err := DecShareBatch(suite, H, Y1, P1, x[1], Z1)
	require.Equal(test, err, nil)

	KD2, ED2, DD2, err := DecShareBatch(suite, H, Y2, P2, x[2], Z2)
	require.Equal(test, err, nil)

	KD3, ED3, DD3, err := DecShareBatch(suite, H, Y3, P3, x[3], Z3)
	require.Equal(test, err, nil)

	// Re-establish order
	XF0 := []kyber.Point{KD0[0], KD1[0], KD2[0], KD3[0]}
	XF1 := []kyber.Point{KD0[1], KD1[1], KD2[1], KD3[1]}
	XF2 := []kyber.Point{KD0[2], KD1[2], KD2[2], KD3[2]}

	EF0 := []*PubVerShare{ED0[0], ED1[0], ED2[0], ED3[0]}
	EF1 := []*PubVerShare{ED0[1], ED1[1], ED2[1], ED3[1]}
	EF2 := []*PubVerShare{ED0[2], ED1[2], ED2[2], ED3[2]}

	DF0 := []*PubVerShare{DD0[0], DD1[0], DD2[0], DD3[0]}
	DF1 := []*PubVerShare{DD0[1], DD1[1], DD2[1], DD3[1]}
	DF2 := []*PubVerShare{DD0[2], DD1[2], DD2[2], DD3[2]}

	// (3) Recover secrets
	S0, err := RecoverSecret(suite, G, XF0, EF0, DF0, t, n)
	require.Equal(test, err, nil)

	S1, err := RecoverSecret(suite, G, XF1, EF1, DF1, t, n)
	require.Equal(test, err, nil)

	S2, err := RecoverSecret(suite, G, XF2, EF2, DF2, t, n)
	require.Equal(test, err, nil)

	// Verify secrets
	require.True(test, suite.Point().Mul(s0, nil).Equal(S0))
	require.True(test, suite.Point().Mul(s1, nil).Equal(S1))
	require.True(test, suite.Point().Mul(s2, nil).Equal(S2))
}
