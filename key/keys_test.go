package key

import (
	"bytes"
	"os"
	"strconv"
	"testing"

	"github.com/BurntSushi/toml"
	kyber "github.com/drand/kyber"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/util/random"
	"github.com/stretchr/testify/require"
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
		key := KeyGroup.Scalar().Pick(random.New())
		publics[i] = KeyGroup.Point().Mul(key, nil)
	}

	distPublic := &DistPublic{Coefficients: publics}
	dtoml := distPublic.TOML()

	var writer bytes.Buffer
	enc := toml.NewEncoder(&writer)
	require.NoError(t, enc.Encode(dtoml))
	toml.NewEncoder(os.Stdout).Encode(dtoml)

	d2 := new(DistPublic)
	d2Toml := new(DistPublicTOML)
	_, err := toml.DecodeReader(&writer, d2Toml)
	require.NoError(t, err)
	require.NoError(t, d2.FromTOML(d2Toml))

	// TODO : just create the struct manually: otherwise when changing curves,
	// test fails.
	checkTOML := &DistPublicTOML{Coefficients: []string{"96ea03d91b9f30315c06169396bcaa6a1a0c7def89980bec8d1f93c21273a77cd1f85f2226dc2b1dad5c1217cc3f65b1043afcbc9a1c374fbf8c1444a1eea4d44729c759074e08d994cc1add37ac90624a07584c9be47b52aafa7253df672754", "b42b8ca5c1ce1fd607c3607d82a4e2ff6b737a86be5271994c786781390e7cbd786f0a414371ffbbe50224164068b1d012cbeed230d3cc0bda514dcf91b2f3c0b9cb05fa3f27857891ba11e3df840128a83053fd8cf373002d27b80a67f40caf", "91ec46451b74b240b21f492baa60d6f4c8f6aef0de843f34336a9f6bfb75d278b795f9b09296430ca1decfbd6235424503f9e4ce8539e71f1b7e8b0870f745abf0bada7905050480f3e0306f353266a1f4d465a355d33fc02f28374cd3273244", "a4aa79ce57d03cb19259b44da6ef7fd0821ee095b8e16111b0b44cdc068d7d56d27a11ee86a868af3a16c085d2be9fbc03315023c385837dc892f9dbd59763bbf66667510bae8e95b61d9a54647bab2bef6e5563c7509cbc4fceacb578115c0b"}}

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
		s.Commits[i] = KeyGroup.Point().Pick(random.New())
		s.PrivatePoly[i] = KeyGroup.Scalar().Pick(random.New())
	}
	s.Share = &share.PriShare{V: KeyGroup.Scalar().Pick(random.New()), I: 0}

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
	fakeDistKey := KeyGroup.Point().Pick(random.New())
	distKey := &DistPublic{[]kyber.Point{fakeDistKey}}
	group := &Group{
		Threshold: DefaultThreshold(n),
		Nodes:     pubs,
		PublicKey: distKey,
	}
	return privs, group
}
