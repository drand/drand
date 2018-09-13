package key

import (
	"bytes"
	"reflect"
	"strconv"
	"testing"

	"github.com/BurntSushi/toml"
	kyber "github.com/dedis/kyber"
	"github.com/dedis/kyber/share"
	"github.com/dedis/kyber/util/random"
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
	require.True(t, reflect.DeepEqual(s, s2))
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
	keyStr := "012067064287f0d81a03e575109478287da0183fcd8f3eda18b85042d1c8903ec8160c56eb6d5884d8c519c30bfa3bf5181f42bcd2efdbf4ba42ab0f31d13c97e9552543be1acf9912476b7da129d7c7e427fbafe69ac5b635773f488b8f46f3fc40c673b93a08a20c0e30fd84de8a89adb6fb95eca61ef2fff66527b3be4912de"
	fakeKey, _ := stringToPoint(G2, keyStr)
	distKey := &DistPublic{Key: fakeKey}
	group := &Group{
		Threshold: DefaultThreshold(n),
		Nodes:     toIndexedList(pubs),
		CoKey:     distKey,
	}
	return privs, group
}
