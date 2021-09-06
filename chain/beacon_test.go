package chain

import (
	"testing"

	"github.com/drand/drand/key"
	"github.com/drand/kyber/util/random"
)

func BenchmarkVerifyBeacon(b *testing.B) {
	benchmarkVerifyBeacon(b, false)
}
func BenchmarkVerifyBeaconDecoupled(b *testing.B) {
	benchmarkVerifyBeacon(b, true)
}

func benchmarkVerifyBeacon(b *testing.B, decouplePrevSig bool) {
	secret := key.KeyGroup.Scalar().Pick(random.New())
	public := key.KeyGroup.Point().Mul(secret, nil)

	var round uint64 = 16
	prevSig := []byte("My Sweet Previous Signature")
	msg := Message(round, prevSig, decouplePrevSig)

	sig, _ := key.AuthScheme.Sign(secret, msg)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b := Beacon{
			PreviousSig: prevSig,
			Round:       16,
			Signature:   sig,
		}
		err := b.Verify(public, decouplePrevSig)
		if err != nil {
			panic(err)
		}
	}
}
