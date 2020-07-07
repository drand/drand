package chain

import (
	"testing"

	"github.com/drand/drand/key"
	"github.com/drand/kyber/util/random"
)

func BenchmarkVerifyBeacon(b *testing.B) {
	secret := key.KeyGroup.Scalar().Pick(random.New())
	public := key.KeyGroup.Point().Mul(secret, nil)
	var round uint64 = 16
	prevSig := []byte("My Sweet Previous Signature")
	msg := Message(round, prevSig)
	sig, _ := key.AuthScheme.Sign(secret, msg)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := VerifyBeacon(public, &Beacon{
			PreviousSig: prevSig,
			Round:       16,
			Signature:   sig,
		})
		if err != nil {
			panic(err)
		}
	}
}
