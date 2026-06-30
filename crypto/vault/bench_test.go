package vault

import (
	"testing"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/testlogger"
	"github.com/drand/drand/v2/crypto"
)

// The benchmarks below exercise the threshold-BLS hot path that a running drand
// network hits on every round: each node produces a partial signature, partials
// are verified, then a threshold of them is recovered into the final beacon
// signature which is verified against the group key. These are the operations
// whose cost is most sensitive to the kyber dependency, so they double as
// performance-regression guards for the upgrade. Run e.g.:
//
//	go test -run '^$' -bench . -benchmem ./crypto/vault/
//
// against the current dependency and against the kyber/v4 branch and diff with
// benchstat.

const benchN, benchThr = 10, 6

// benchSchemes keeps the matrix small but representative: one chained (sigs on
// G2), one unchained-on-G1 (sigs on G1) and the bn254 curve.
var benchSchemes = []string{
	crypto.DefaultSchemeID,
	crypto.SigsOnG1ID,
	crypto.BN254UnchainedOnG1SchemeID,
}

func benchSetup(b *testing.B, name string) (*crypto.Scheme, []*Vault, []byte) {
	b.Helper()
	sch, err := crypto.SchemeFromName(name)
	if err != nil {
		b.Fatal(err)
	}
	group, shares := newGroupWithShares(b, sch, benchN, benchThr)
	vaults := make([]*Vault, benchN)
	for i := range vaults {
		vaults[i] = NewVault(testlogger.New(b), group, shares[i], sch)
	}
	msg := sch.DigestBeacon(&common.Beacon{Round: 1234, PreviousSig: []byte("prev")})
	return sch, vaults, msg
}

func BenchmarkSignPartial(b *testing.B) {
	for _, name := range benchSchemes {
		name := name
		b.Run(name, func(b *testing.B) {
			_, vaults, msg := benchSetup(b, name)
			v := vaults[0]
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := v.SignPartial(msg); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkVerifyPartial(b *testing.B) {
	for _, name := range benchSchemes {
		name := name
		b.Run(name, func(b *testing.B) {
			sch, vaults, msg := benchSetup(b, name)
			pub := vaults[0].GetPub()
			partial, err := vaults[0].SignPartial(msg)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := sch.ThresholdScheme.VerifyPartial(pub, msg, partial); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkRecoverAndVerifyBeacon(b *testing.B) {
	for _, name := range benchSchemes {
		name := name
		b.Run(name, func(b *testing.B) {
			sch, vaults, msg := benchSetup(b, name)
			pub := vaults[0].GetPub()
			groupKey := vaults[0].GetGroup().PublicKey.Key()

			partials := make([][]byte, benchThr)
			for i := 0; i < benchThr; i++ {
				p, err := vaults[i].SignPartial(msg)
				if err != nil {
					b.Fatal(err)
				}
				partials[i] = p
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				sig, err := sch.ThresholdScheme.Recover(pub, msg, partials, benchThr, benchN)
				if err != nil {
					b.Fatal(err)
				}
				beacon := &common.Beacon{Round: 1234, PreviousSig: []byte("prev"), Signature: sig}
				if err := sch.VerifyBeacon(beacon, groupKey); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
