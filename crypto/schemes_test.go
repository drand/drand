package crypto_test

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/key"
	"github.com/drand/kyber/util/random"
)

func BenchmarkVerifyBeacon(b *testing.B) {
	sch, err := crypto.GetSchemeFromEnv()
	if err != nil {
		b.Fatal(err)
	}

	secret := sch.KeyGroup.Scalar().Pick(random.New())
	public := sch.KeyGroup.Point().Mul(secret, nil)

	prevSig := []byte("My Sweet Previous Signature")

	msg := sch.DigestBeacon(&chain.Beacon{
		PreviousSig: prevSig,
		Round:       16,
	})

	sig, _ := sch.AuthScheme.Sign(secret, msg)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		beacon := &chain.Beacon{
			PreviousSig: prevSig,
			Round:       16,
			Signature:   sig,
		}

		err := sch.VerifyBeacon(beacon, public)
		if err != nil {
			panic(err)
		}
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

	msg := sch.DigestBeacon(&chain.Beacon{
		PreviousSig: prevSig,
		Round:       16,
	})

	var sig []byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sig, _ = sch.AuthScheme.Sign(secret, msg)
	}
	b.StopTimer()

	beacon := &chain.Beacon{
		PreviousSig: prevSig,
		Round:       16,
		Signature:   sig,
	}
	err = sch.VerifyBeacon(beacon, public)
	if err != nil {
		panic(err)
	}
}

//nolint:lll
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
			PubKey: "21fca9e03f9ec67ee54f4bf5019ef69d8d19f782117c73a0f1243424767901740d4ac0222f1a284c4d857b7bdf66738340f58cd028c98a74de17faca68e260be28f6d864c4cc6e2607866c23208bb050d5a473679895b7d9f7e3777f8dba85e405f18d641ab8bfe26c607e69315c9961ada206ebd21ee3042adf2f8cb4337d4c",
			Scheme: "bls-bn254-unchained-on-g1",
			Round:  1,
			Sig:    "147d98a0bbadf6d1b2115441654c446039ed61ff2f71abefcdb8aefbfd81c37121bd020cd1814033782226408aa7b0ac86fd1682755c39a023282d0031635b7d",
		},
		{
			PubKey: "138a5e9fc1256b056cc81a1717e23b271131960cda30eb8214bb82da71f06b5f22247bb1b24f379331e2dcf1e930d59b3b7966779658c6583516f61db8c2c1f61ffe07fe6e566fa61aa9fe0c1010cd223d8149f5217c417d317af11317afff3322997c32035ce3a3f6199a7b11302943d91338f0fff1d3c70d0504d6ebdd976f",
			Scheme: "bls-bn254-unchained-on-g1",
			Round:  16,
			Sig:    "1f3bf2503ad726be5f48394a5b61231a59126f43dd41c64b962ef150935853e614683d912aa55bcb45638b6467158d9c9ecccf13e1169948feaec49cf123c284",
		},
		{
			PubKey: "17f6b98af3cdc5293af1d2321b4287d50a4f94d7a923b07101c9d80ce98e3cdd24af0c6120b280cea67efd627ad451daa90d37d04345a70ed37cf182ce5d68cf1dbf8992eb100b6bcbbdb9c664733c6bfb59068187a7c779cc7772043c0a36a120014bf0311f944495ae94272f531c8a7c68947fbcb0bc7927f729741389ee35",
			Scheme: "bls-bn254-unchained-on-g1",
			Round:  5,
			Sig:    "16cc1c0706d5b12ada510471803b50cffbbe7face4835fa92fa511a33ab867b01955c3d58309af9dc2bbc1d8025e737d6272696371708865d7341d57b5dd3225",
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
		err = sch.VerifyBeacon(&chain.Beacon{Round: beacon.Round, Signature: sig, PreviousSig: prev}, public)
		require.NoError(t, err)
	}
}
