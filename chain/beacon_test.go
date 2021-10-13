package chain

import (
	"testing"

	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/key"
	"github.com/drand/kyber/util/random"
)

func BenchmarkVerifyBeacon(b *testing.B) {
	secret := key.KeyGroup.Scalar().Pick(random.New())
	public := key.KeyGroup.Point().Mul(secret, nil)

	sch := scheme.GetSchemeFromEnv()
	verifier := NewVerifier(sch)

	var round uint64 = 16
	prevSig := []byte("My Sweet Previous Signature")

	msg := verifier.DigestMessage(round, prevSig)

	sig, _ := key.AuthScheme.Sign(secret, msg)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b := Beacon{
			PreviousSig: prevSig,
			Round:       16,
			Signature:   sig,
		}

		err := verifier.VerifyBeacon(b, public)
		if err != nil {
			panic(err)
		}
	}
}
