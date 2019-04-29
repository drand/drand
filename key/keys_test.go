package key

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
	kyber "go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/share"
	"go.dedis.ch/kyber/v3/util/random"
)

func TestKeyPublic(t *testing.T) {
	addr := "127.0.0.1:80"
	kp := NewTLSKeyPair(addr)
	ptoml := kp.Public.TOML().(*PublicTOML)
	require.Equal(t, kp.Public.Addr, ptoml.Address)
	require.Equal(t, kp.Public.TLS, ptoml.TLS)

	var writer bytes.Buffer
	enc := toml.NewEncoder(&writer)
	require.NoError(t, enc.Encode(ptoml))

	p2 := new(Identity)
	p2toml := new(PublicTOML)
	_, err := toml.DecodeReader(&writer, p2toml)
	require.NoError(t, err)
	require.NoError(t, p2.FromTOML(p2toml))

	require.Equal(t, kp.Public.Addr, p2.Addr)
	require.Equal(t, kp.Public.TLS, p2.TLS)
	require.Equal(t, kp.Public.Key.String(), p2.Key.String())
}

func TestKeyDistributedPublic(t *testing.T) {
	n := 4
	publics := make([]kyber.Point, n)
	for i := range publics {
		key := G2.Scalar().Pick(random.New())
		publics[i] = G2.Point().Mul(key, nil)
	}

	distPublic := &DistPublic{Coefficients: publics}
	dtoml := distPublic.TOML()

	var writer bytes.Buffer
	enc := toml.NewEncoder(&writer)
	require.NoError(t, enc.Encode(dtoml))

	d2 := new(DistPublic)
	d2Toml := new(DistPublicTOML)
	_, err := toml.DecodeReader(&writer, d2Toml)
	require.NoError(t, err)
	require.NoError(t, d2.FromTOML(d2Toml))

	checkTOML := &DistPublicTOML{Coefficients: []string{"0776a00e44dfa3ab8cff6b78b430bf16b9f8d088b54c660722a35f5034abf3ea4deb1a81f6b9241d22185ba07c37f71a67f94070a71493d10cb0c7e929808bd10cf2d72aeb7f4e10a8b0e6ccc27dad489c9a65097d342f01831ed3a9d0a875b770452b9458ec3bca06a5d4b99a5ac7f41ee5a8add2020291eab92b4c7f2d449f", "740d1293f2730092255818b5802787084804d91cf9d92f5068f188b8be6241356ac97ab33193497f9cacb9dca637b0921f9906a5af8159886b7a9a44c6694fa101a83714013b8f8b12c8db8911cf390da35ceedbf9b2371fe23141350e9d99df8136586f3f8f6089fc6cb6de9acc54ab192d78c2c0c70b9df938f7930b1d44a9", "023643d5e595d960b5da67879382051a54df13fd312c256c0beb0dc82442f1308e0510a1ef04706445c79a79bbc7792c99ca82eae23f9ce8e30a5d79ed263fe24e2136e7d1d46a61746b521a443b6951a34dd8ccd36b1f294f0e254d99975ce5515e485a874b3636d60de39b933b0a9a1fcb942590089554e4321af31bf94ca1", "6fb6e322585ff85d976cc43a8ac81d6753850855c08d16ce458a1d6ada16c2ea8d9a54a6ec3d106dd08686abb5e7fee94f27a4d74a452673ec6796eff4766ecc7097117761aed5cb510d96d3955cceb2c3c44f89e3c27f3fa8fd3c0410a805ee018f83999c668ed674498c3fda09fa4f516a4bac6b6a875410a2037e1aa21cae"}}

	check := &DistPublic{}
	require.NoError(t, check.FromTOML(checkTOML))
}

func TestKeyGroup(t *testing.T) {
	n := 5
	_, group := BatchIdentities(n)
	ids := group.Identities()
	for _, p := range ids {
		require.True(t, p.TLS)
	}
	gtoml := group.TOML().(*GroupTOML)
	for _, p := range gtoml.Nodes {
		require.True(t, p.TLS)
	}
}

func TestShare(t *testing.T) {
	n := 5
	s := new(Share)
	s.Commits = make([]kyber.Point, n, n)
	s.PrivatePoly = make([]kyber.Scalar, n, n)
	for i := 0; i < n; i++ {
		s.Commits[i] = G2.Point().Pick(random.New())
		s.PrivatePoly[i] = G2.Scalar().Pick(random.New())
	}
	s.Share = &share.PriShare{V: G2.Scalar().Pick(random.New()), I: 0}

	stoml := s.TOML()
	s2 := new(Share)
	require.NoError(t, s2.FromTOML(stoml))
	poly1 := s.PrivatePoly
	poly2 := s2.PrivatePoly
	require.Equal(t, len(poly1), len(poly2))
	for i := range poly1 {
		require.Equal(t, poly1[i].String(), poly2[i].String())
	}
}

func BatchIdentities(n int) ([]*Pair, *Group) {
	startPort := 8000
	startAddr := "127.0.0.1:"
	privs := make([]*Pair, n)
	pubs := make([]*Identity, n)
	for i := 0; i < n; i++ {
		port := strconv.Itoa(startPort + i)
		addr := startAddr + port
		privs[i] = NewTLSKeyPair(addr)
		pubs[i] = privs[i].Public
	}
	keyStr := "0776a00e44dfa3ab8cff6b78b430bf16b9f8d088b54c660722a35f5034abf3ea4deb1a81f6b9241d22185ba07c37f71a67f94070a71493d10cb0c7e929808bd10cf2d72aeb7f4e10a8b0e6ccc27dad489c9a65097d342f01831ed3a9d0a875b770452b9458ec3bca06a5d4b99a5ac7f41ee5a8add2020291eab92b4c7f2d449f"
	fakeKey, _ := StringToPoint(G2, keyStr)
	distKey := &DistPublic{[]kyber.Point{fakeKey}}
	group := &Group{
		Threshold: DefaultThreshold(n),
		Nodes:     pubs,
		PublicKey: distKey,
	}
	return privs, group
}
