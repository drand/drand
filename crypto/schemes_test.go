package crypto_test

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/kyber/util/random"
)

func TestNamesInList(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"", false},
		{crypto.DefaultSchemeID, true},
		{crypto.ShortSigSchemeID, true},
		{crypto.SigsOnG1ID, true},
		{crypto.UnchainedSchemeID, true},
		{"nonexistentschemename", false},
	}

	for _, tt := range tests {
		t.Run(tt.name+"IsInList", func(t *testing.T) {
			for _, v := range crypto.ListSchemes() {
				if tt.name == v {
					return
				}
			}
			require.False(t, tt.expected)
		})
	}
}

func BenchmarkVerifyBeacon(b *testing.B) {
	sch, err := crypto.GetSchemeFromEnv()
	if err != nil {
		b.Fatal(err)
	}

	secret := sch.KeyGroup.Scalar().Pick(random.New())
	public := sch.KeyGroup.Point().Mul(secret, nil)

	prevSig := []byte("My Sweet Previous Signature")

	msg := sch.DigestBeacon(&common.Beacon{
		PreviousSig: prevSig,
		Round:       16,
	})

	sig, _ := sch.AuthScheme.Sign(secret, msg)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		beacon := &common.Beacon{
			PreviousSig: prevSig,
			Round:       16,
			Signature:   sig,
		}

		err := sch.VerifyBeacon(beacon, public)
		require.NoError(b, err)
	}
}

func BenchmarkSignBeacon(b *testing.B) {
	sch, err := crypto.GetSchemeFromEnv()
	if err != nil {
		b.Fatal(err)
	}
	secret := sch.KeyGroup.Scalar().Pick(random.New())
	public := sch.KeyGroup.Point().Mul(secret, nil)

	prevSig := []byte("My Sweet Previous Signature")

	msg := sch.DigestBeacon(&common.Beacon{
		PreviousSig: prevSig,
		Round:       16,
	})

	var sig []byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sig, _ = sch.AuthScheme.Sign(secret, msg)
	}
	b.StopTimer()

	beacon := &common.Beacon{
		PreviousSig: prevSig,
		Round:       16,
		Signature:   sig,
	}
	err = sch.VerifyBeacon(beacon, public)
	require.NoError(b, err)
}

func TestVerifyBeacon(t *testing.T) {
	t.Parallel()
	testBeacons := []struct {
		Round   uint64
		PubKey  string
		Sig     string
		PrevSig string
		Scheme  string
	}{
		{
			Round:   2634945,
			PubKey:  "868f005eb8e6e4ca0a47c8a77ceaa5309a47978a7c71bc5cce96366b5d7a569937c529eeda66c7293784a9402801af31",
			Sig:     "814778ed1e480406beb43b74af71ce2f0373e0ea1bfdfea8f9ed62c876c20fcbc7f0163860e3da42ed2148756015f4551451898ffe06d384b4d002245025571b6b7a752f7158b40ad92b13b6d703ad31922a617f2c7f6d960b84d56cf1d79eef",
			PrevSig: "8bd96294383b4d1e04e736360bd7a487f9f409f1e7bd800b720656a310d577b3bdb1e1631af6c5782a1d8979c502f395036181eff4058960fc40bb7034cdae1991d3eda518ab204a077d2f7e724974cf87b407e549bd815cf0b8e5a3832f675d",
			Scheme:  "pedersen-bls-chained",
		},
		{
			PubKey:  "922a2e93828ff83345bae533f5172669a26c02dc76d6bf59c80892e12ab1455c229211886f35bb56af6d5bea981024df",
			Scheme:  "pedersen-bls-chained",
			Round:   3361396,
			Sig:     "9904b4ec42e82cb42ad53f171cf0510a5eedff8b5e02e2db5a187489f7875307746998b9a6cf82130d291126d4b83cea1048c9b3f07a067e632c20391dc059d22d6a8e835f3980c8bd0183fb6df00a8fbbe6b8c9f61e888dfa76e12af4d4e355",
			PrevSig: "a2377f4e0403f0fd05f709a3292be1b2b59fe990a673ad7b7561b5bd5982b882a2378d36e39befb6ea3bb7aac113c50a18fb07aa4f9a59f95f1aaa7826dafbfcdbf22347c29996c294286fd11b402ad83edd83fa21fe6735fccb65785edbed47",
		},
		{
			PubKey: "8200fc249deb0148eb918d6e213980c5d01acd7fc251900d9260136da3b54836ce125172399ddc69c4e3e11429b62c11",
			Scheme: "pedersen-bls-unchained",
			Round:  7601003,
			Sig:    "af7eac5897b72401c0f248a26b612c5ef68e0ff830b4d78927988c89b5db3e997bfcdb7c24cb19f549830cd02cb854a1143fd53a1d4e0713ded471260869439060d170a77187eb6371742840e43eccfa225657c4cc2d9619f7c3d680470c9743",
		},
		{
			PubKey: "876f6fa8073736e22f6ff4badaab35c637503718f7a452d178ce69c45d2d8129a54ad2f988ab10c9666f87ab603c59bf013409a5b500555da31720f8eec294d9809b8796f40d5372c71a44ca61226f1eb978310392f98074a608747f77e66c5a",
			Scheme: "bls-unchained-on-g1",
			Round:  3,
			Sig:    "ac7c3ca14bc88bd014260f22dc016b4fe586f9313c3a549c83d195811a99a5d2d4999d4df6daec73ff51fafadd6d5bb5",
		},
		{
			PubKey: "a0b862a7527fee3a731bcb59280ab6abd62d5c0b6ea03dc4ddf6612fdfc9d01f01c31542541771903475eb1ec6615f8d0df0b8b6dce385811d6dcf8cbefb8759e5e616a3dfd054c928940766d9a5b9db91e3b697e5d70a975181e007f87fca5e",
			Scheme: "bls-unchained-on-g1",
			Round:  2,
			Sig:    "a050676d1a1b6ceedb5fb3281cdfe88695199971426ff003c0862460b3a72811328a07ecd53b7d57fc82bb67f35efaf1",
		},
		{
			PubKey: "00e3e43df8fcc6a8e57a419a72cee58dc97ad27b2cd17db52ca6e173fe2962971d9d20260c7006980bb49ce8a152bb81e43862f0b6a2c49c3a19b457c2892b7302eb4c1d3ebefde8b9eefeabcdc2d8dcef925f270a345c298a6c31a2df23bd4f1319c6bb3b5376e85f1e0ee12359ecc28928593163c4df2d0b9c6d3505e2c02f",
			Scheme: "bls-bn254-unchained-on-g1",
			Round:  1,
			Sig:    "256867706c495afda16143b5cb7013dc582ee698a096220bb2a7a12e9091603427a95923355cf492d540e4d428e949e46e4e293165f4f30b8b12c51fae591e37",
		},
		{
			PubKey: "047033cd6e8a37849271c7a2b624176ded162fa8d8f309610f0d32cb1b4e647124a1d10d86e51d268e5927b12a772997c47bc39396ef44daf73e29d246b9f56c210e4bb108f94165a6d8005fa82f3265bfde96289bc2fcca42c643997693aedf2e75abeb76348f0f7f96a02bbc7c9cc68aba524008b7b20e9c27353589297096",
			Scheme: "bls-bn254-unchained-on-g1",
			Round:  16,
			Sig:    "0367ff3a4ae82eb060decfe4d79549d07dbbf490e6c0d800ffe1e5a4edca03af231486e2358564a5c34be1a010bb8afc84e8d347fdf27807e2e987bf430cc222",
		},
		{
			PubKey: "2a8fde29149e45235ddf09a79873f9e830294decd247722ccbf0552c15d1c5231550c146413b9326b9c4f425d16962964e458211c1e4c86f70bd354fa3fbf1d417fcf10dd2edbc8e5f95f27bb975ada01d9625033051e272085e3d25244d3dec19bf704f8c41e0b8dc56e36b6b0ae624448e46f511c4da2a95e5e32c3e270ab8",
			Scheme: "bls-bn254-unchained-on-g1",
			Round:  5,
			Sig:    "27014bdeb181aac8afe771f67d6c168c46e5a184b6a62cd4c2a155650a992eda2df062eb8aaa87e2712d71bf64e66ea8cf0845b4cfbf4151ba595ae7bce72555",
		},
	}

	for _, beacon := range testBeacons {
		sch, err := crypto.SchemeFromName(beacon.Scheme)
		require.NoError(t, err)
		public, err := key.StringToPoint(sch.KeyGroup, beacon.PubKey)
		require.NoError(t, err)
		sig, err := hex.DecodeString(beacon.Sig)
		require.NoError(t, err)
		prev, err := hex.DecodeString(beacon.PrevSig)
		require.NoError(t, err)
		err = sch.VerifyBeacon(&common.Beacon{Round: beacon.Round, Signature: sig, PreviousSig: prev}, public)
		require.NoError(t, err)
	}
}

func TestGetSchemeByID(t *testing.T) {
	tests := []struct {
		name      string
		isDefault bool
		want      bool
	}{
		{"", true, true},
		{crypto.DefaultSchemeID, true, true},
		{crypto.ShortSigSchemeID, false, true},
		{crypto.SigsOnG1ID, false, true},
		{crypto.UnchainedSchemeID, false, true},
		{"nonexistentschemename", false, false},
		{crypto.DefaultSchemeID + "wrong", false, false},
		{"wrong" + crypto.ShortSigSchemeID, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name+"byID", func(t *testing.T) {
			got, err := crypto.GetSchemeByID(tt.name)
			gotBool := err == nil
			// special case "" is considered to be the default beacon
			if gotBool && got.Name != tt.name && tt.name != "" {
				t.Errorf("GetSchemeByID() got = %v, want %v", got, tt.name)
			}
			if tt.isDefault && got.Name != crypto.DefaultSchemeID {
				t.Errorf("UnexpectedDefaultName got = %v", got.Name)
			}
			if gotBool != tt.want {
				t.Errorf("GetSchemeByID() gotBool = %v, want %v", gotBool, tt.want)
			}
		})
	}
}
