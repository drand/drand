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
	msg := MessageV2(round)
	sig, _ := key.AuthScheme.Sign(secret, msg)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := VerifyBeaconV2(public, &Beacon{
			Round:     16,
			Signature: sig,
		})
		if err != nil {
			panic(err)
		}
	}
}
