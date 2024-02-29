package key

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/protobuf/drand"
	"github.com/drand/kyber"
	"github.com/drand/kyber/util/random"
)

func newIds(t *testing.T, n int) []*Node {
	ids := make([]*Node, n)
	for i := 0; i < n; i++ {
		key, err := NewKeyPair("127.0.0.1:3000", nil)
		require.NoError(t, err)

		ids[i] = &Node{
			Index:    uint32(i),
			Identity: key.Public,
		}
	}
	return ids
}

func TestGroupProtobuf(t *testing.T) {
	type testVector struct {
		group  *Group
		change func(*drand.GroupPacket)
		isErr  bool
	}

	var vectors []testVector
	n := 9
	thr := 5
	ids := newIds(t, n)
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)

	dpub := []kyber.Point{sch.KeyGroup.Point().Pick(random.New())}
	group := LoadGroup(ids, 1, &DistPublic{dpub}, 30*time.Second, 61, sch, "test_beacon")
	group.Threshold = thr
	group.Period = time.Second * 4
	group.GenesisTime = time.Now().Add(10 * time.Second).Unix()
	group.TransitionTime = time.Now().Add(10 * time.Second).Unix()
	genesis := group.GenesisTime
	transition := group.TransitionTime

	vectors = append(vectors, testVector{
		group:  group,
		change: nil,
		isErr:  true,
	})

	var dpub2 []kyber.Point
	for i := 0; i < thr; i++ {
		dpub2 = append(dpub2, sch.KeyGroup.Point().Pick(random.New()))
	}
	group2 := *group
	group2.PublicKey = &DistPublic{dpub2}
	vectors = append(vectors, testVector{
		group:  &group2,
		change: nil,
		isErr:  false,
	})

	group3 := group2
	var nodes = make([]*Node, len(group3.Nodes))
	copy(nodes, group3.Nodes)
	nodes[0], nodes[1] = nodes[1], nodes[0]
	group3.Nodes = nodes
	vectors = append(vectors, testVector{
		group:  &group3,
		change: nil,
		isErr:  false,
	})

	version := common.GetAppVersion()
	for i, tv := range vectors {
		protoGroup := tv.group.ToProto(version)
		if tv.change != nil {
			tv.change(protoGroup)
		}

		loaded, err := GroupFromProto(protoGroup, nil)
		if tv.isErr {
			require.Error(t, err)
			continue
		}

		require.NoError(t, err)

		// load the seed after
		seed := tv.group.GetGenesisSeed()
		require.Equal(t, len(loaded.Nodes), len(tv.group.Nodes), "test %d", i)
		require.Equal(t, loaded.Threshold, tv.group.Threshold)
		require.True(t, loaded.PublicKey.Equal(tv.group.PublicKey), "test %d: %v \nvs %v", i, loaded.PublicKey, group.PublicKey)
		require.Equal(t, loaded.Period, tv.group.Period)
		require.Equal(t, seed, loaded.GetGenesisSeed())
		require.Equal(t, genesis, loaded.GenesisTime)
		require.Equal(t, transition, loaded.TransitionTime)
		require.Equal(t, tv.group.Hash(), loaded.Hash())
	}
}

func TestGroupUnsignedIdentities(t *testing.T) {
	ids := newIds(t, 5)
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)

	group := LoadGroup(ids, 1, &DistPublic{[]kyber.Point{sch.KeyGroup.Point()}}, 30*time.Second, 61, sch, "test_beacon")
	require.Nil(t, group.UnsignedIdentities())

	ids[0].Signature = nil
	require.Len(t, group.UnsignedIdentities(), 1)

	ids[1].Signature = []byte("silver linings")
	require.Len(t, group.UnsignedIdentities(), 2)
}
func TestGroupSaveLoad(t *testing.T) {
	n := 3
	ids := newIds(t, n)
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)

	dpub := []kyber.Point{sch.KeyGroup.Point().Pick(random.New())}

	group := LoadGroup(ids, 1, &DistPublic{dpub}, 30*time.Second, 61, sch, "test_beacon")
	group.Threshold = 3
	group.Period = time.Second * 4
	group.GenesisTime = time.Now().Add(10 * time.Second).Unix()
	group.TransitionTime = time.Now().Add(10 * time.Second).Unix()

	genesis := group.GenesisTime
	transition := group.TransitionTime

	gtoml := group.TOML().(*GroupTOML)
	require.NotNil(t, gtoml.PublicKey)

	// faking distributed public key coefficients
	groupFile, err := os.CreateTemp(t.TempDir(), "group.toml")
	require.NoError(t, err)
	groupPath := groupFile.Name()
	groupFile.Close()

	require.NoError(t, Save(groupPath, group, false))
	// load the seed after
	seed := group.GetGenesisSeed()

	loaded := &Group{}
	require.NoError(t, Load(groupPath, loaded))

	require.Equal(t, len(loaded.Nodes), len(group.Nodes))
	require.Equal(t, loaded.Threshold, group.Threshold)
	require.True(t, loaded.PublicKey.Equal(group.PublicKey))
	require.Equal(t, loaded.Period, group.Period)
	require.Equal(t, seed, loaded.GetGenesisSeed())
	require.Equal(t, genesis, loaded.GenesisTime)
	require.Equal(t, transition, loaded.TransitionTime)

	require.Equal(t, group.Hash(), loaded.Hash())
}

// BatchIdentities generates n identities
func makeGroup(t *testing.T) *Group {
	t.Helper()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)

	fakeKey := sch.KeyGroup.Point().Pick(random.New())
	group := LoadGroup([]*Node{}, 1, &DistPublic{Coefficients: []kyber.Point{fakeKey}}, 30*time.Second, 0, sch, "test_beacon")
	group.Threshold = MinimumT(0)
	return group
}

func TestConvertGroup(t *testing.T) {
	group := makeGroup(t)
	group.Period = 5 * time.Second
	group.TransitionTime = time.Now().Unix()
	group.GenesisTime = time.Now().Unix()
	version := common.GetAppVersion()

	proto := group.ToProto(version)
	received, err := GroupFromProto(proto, nil)
	require.NoError(t, err)
	require.True(t, received.Equal(group))
}

func TestLenReturnsNonMissingNodes(t *testing.T) {
	groupFile := `Threshold = 7
Period = "25s"
CatchupPeriod = "15s"
GenesisTime = 1590445175
TransitionTime = 1684938875
GenesisSeed = "4dd408e5fdff9323c76a9b6f087ba8fdc5a6da907bd9217d9d10f2287d081957"
SchemeID = "pedersen-bls-chained"
ID = "default"

[[Nodes]]
  Address = "pl3-rpc.testnet.drand.sh:443"
  Key = "869f9e68169523c164b4fa114abd7ffd9032f94cf7662f27fd295e37fc8264f1896fee6a6366ccfe55b02e3aa42ad994"
  TLS = true
  Signature = "b45da17ea324f23c7c9cefdb715cecd0159bb7a1a54a4418a1a4a45604abf957fbdbedd31eb4703e2f4586f1d1a660760ce52b536161f2bc84678a9b45a9d5710265d9b78e9315dccfccefebc4c3bd6fbc52c69877d839d408ba560d79ab1be1"
  SchemeName = "pedersen-bls-chained"
  Index = 0

[[Nodes]]
  Address = "drand1.ethdevops.io:4444"
  Key = "8e3618936ed612d5c52841e5336168614ee38dd1bfa0901c32d6b66e638d41797213a456a005e5ee1a4436c1d61464ba"
  TLS = true
  Signature = "b5223ca31710d422bcc246e09b19027f03ac0750aed8f65e4ea882b30825d23707cd3bce5e9428f3dc29119403a602aa0cd020dc1ef353d4083caeb59c690f124688cb62b3f07848364d1c8def99931c85a6478f8619a518ef52b2aa4d23e9ac"
  SchemeName = "pedersen-bls-chained"
  Index = 2

[[Nodes]]
  Address = "drand.kencloud.com:443"
  Key = "93ada21006fd556f8467f081ce224de504e9692072e90b75eb66b4678aa7704ae6a68a2fd69701ffa4fcfa568695b4d6"
  TLS = true
  Signature = "b6f9579b5a627cdd36cc6e7e34d32de7b5bbebdc169c7036bf740e98cdbc2d95ce38fbc125a04a243cd4a7409949be7a025505a507ec0e562aeca3ee7ab722c110a3cc19ffb1a375bc5a6077cbee598725c6bad562b0d6219e71e5ecb9f389ad"
  SchemeName = "pedersen-bls-chained"
  Index = 3

[[Nodes]]
  Address = "testnet-1.drand.theqrl.org:4321"
  Key = "95deda10079d8d95c7a37c119242ea583ad6b6be6a7e6b0fa648c20c381c086ebda54410e0ba0016eea9cd31b1d00c71"
  TLS = true
  Signature = "95e6285653d2984a28084494bb9e9cb95d57d8ca326884e48a7c1b3e1c7afec24f23e0ab5e36de2f9fea58453faffe0e027c33d0bcc6264efeb0eab2b1b7ec08a639632e0a9e1eaf0582e9df6e5c7562668a4fdb8852c496562a6c642e7436f2"
  SchemeName = "pedersen-bls-chained"
  Index = 4

[[Nodes]]
  Address = "drandtest.ata.network:443"
  Key = "96e824230b70dfb1282fc7788caad55d0ee82a9fc909d6752a18c56b70a797cd14ec880e93916cb7eb96725bc1010717"
  TLS = true
  Signature = "932603278c8acdbca9f0832a7670311ee2edab5595d076fd9be330eae900407c8a83ee35696bdf654e9da364552a482d020c4875e2b0b9b88af7f157791b133d67d47aedd99c38fa726347e2a269dcea6810d1bd3114abc8c8b7dc7802efdd05"
  SchemeName = "pedersen-bls-chained"
  Index = 5

[[Nodes]]
  Address = "testnet.drand.storswift.com:8269"
  Key = "994ffb0a61ac8645d3f1dac6d5e3420660a77c9e03aba62b5826a4c6721572ae7212412485000ebc5f627e203639e6bd"
  TLS = true
  Signature = "8ea2006cb860046b527c3d35fd67b030b927df3778c2bb604c10ff7b5ccd99c62550505828d0533ea77d7a677ca6dc2d0bb1f451705c4cec2703802214deb059bcf231c32fa76b7a8f56d295c025c45e76b4c0b068aa20e5a3980e85f3cb643c"
  SchemeName = "pedersen-bls-chained"
  Index = 7

[[Nodes]]
  Address = "drand-testnet.clabs.co:443"
  Key = "a602f9762292f2533b268757b9337181bedb290d3bc6768504e2573758651c0196f001eec498f2316a2022ecc4c77062"
  TLS = true
  Signature = "b1f7a05966ee6cd5bf5aff51a15e48e0a0f08abf8002d54f1f3aa754b59c9740cb71cf0ec94638312c7a7ac4a65e69f20b599e26275d46b357661c7f6ce13da1bb25977dab451b2f4d858c395653e816ae7db2cfa7323bef5c5229a19aff1a63"
  SchemeName = "pedersen-bls-chained"
  Index = 8

[[Nodes]]
  Address = "testnet0-rpc.drand.cloudflare.com:443"
  Key = "a636ccd4d5568ffb2e445a5a6326ca84326781d3d2d013fc123130e7a39fa0b87aab1798225471ea84961d5e19df2279"
  TLS = true
  Signature = "848a366ab500f71aec7cceae8d735b4e7b487dc09f243d92590d0fec35caff037e9696faa746ac5a0031b8829a6fbed91918ec5a312e689538ff69951df0cc99f01c2ae981cfc832f88ffa559bb0eddd5e9d8e3d6b9f44cc5756d514f157bce1"
  SchemeName = "pedersen-bls-chained"
  Index = 9

[[Nodes]]
  Address = "pl2-rpc.testnet.drand.sh:443"
  Key = "a6a39e3b38f02f26d096384a7ba7b9fd50f18912ccc3d50e6c83a9240774ffa418db28b3b439674e841d0b7b5a018f26"
  TLS = true
  Signature = "838970d91b5d3864c27c0f06a45c3b748da43c0ac5c62ef5c552d5cf840dd6f146a1a270787fc3bff6bc5d5a46c3ddbb08034299062cfb221672270617e4c484e749775b712c81096dc33a8c1dd9fd8ff06dee2fd63dec91f07f8f15abe6df31"
  SchemeName = "pedersen-bls-chained"
  Index = 10

[[Nodes]]
  Address = "pl1-rpc.testnet.drand.sh:443"
  Key = "b7a9f696992863f223ee67465a42054450da99883fd56cd2fd45cb9a6b52f77a0d347026dc82239a4e133b1343f71c46"
  TLS = true
  Signature = "87aa9f2ee2d9a6d03b826d90e6cf7906c4847740907b8bc869b9c7ef02eb24d0ceae5fac0cc85337b7eb27f2a9ca22250a9fabd3b6cc54db65a8813158a2680672abb8078720ebad4cbe9293441ce7685cd4b663f0244ff8186271ec9ff42b5f"
  SchemeName = "pedersen-bls-chained"
  Index = 11

[PublicKey]
  Coefficients = ["922a2e93828ff83345bae533f5172669a26c02dc76d6bf59c80892e12ab1455c229211886f35bb56af6d5bea981024df", "94355069e98565fba9785f5af8ffa209924b49aa50de8493cf7a89f317b80d6fb7537bf575c445ef37694adf13bdd84a", "84edb18a32590997ca1e6e50825ad377144146dd36e210e908f623c3bcc89320d75b96d563aaee0046cd292b570e0e9b", "a4249d6afa8bfa0f547568898b84c8fb15798c931bec168c904c76fc483ccb3fa6b50ad78882c0264f82d776071f7d48", "b1ba1d6e257272454e72f4aa9ca1953146b2bc832f093ca7410fa0f8e0b472ed41f61e4541ab55d8d664969ec00239ce", "8ca06d4afd25f6c652f012464de5a899d87079eff8cd79dd8c1b849dc2336e670337ee5840bd0963c82ff9ef1ba37b91", "b2e0335face7ebe436d00b4851e5e447c2158084b0ee8a22f585dcefa6e941b5513e08ca9e69404b2981d8c6dc29ce77"]`

	groupToml := new(GroupTOML)
	_, err := toml.NewDecoder(bytes.NewReader([]byte(groupFile))).Decode(&groupToml)
	require.NoError(t, err)

	g := new(Group)
	err = g.FromTOML(groupToml)

	require.NoError(t, err)
	// even though there are 12 indexes, we expect the len to be 10 as some are missing
	require.Equal(t, 10, g.Len())
}
