package key

import (
	"bytes"
	"encoding/hex"
	"os"
	"strconv"
	"testing"

	"github.com/drand/drand/crypto"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"

	"github.com/drand/kyber"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/util/random"
)

const testAddr = "127.0.0.1:80"

func TestKeyPublic(t *testing.T) {
	kp, err := NewTLSKeyPair(testAddr, nil)
	require.NoError(t, err)
	ptoml := kp.Public.TOML().(*PublicTOML)
	require.Equal(t, kp.Public.Addr, ptoml.Address)
	require.Equal(t, kp.Public.TLS, ptoml.TLS)

	var writer bytes.Buffer
	enc := toml.NewEncoder(&writer)
	require.NoError(t, enc.Encode(ptoml))

	p2 := new(Identity)
	p2toml := new(PublicTOML)
	_, err = toml.NewDecoder(&writer).Decode(p2toml)
	require.NoError(t, err)
	require.NoError(t, p2.FromTOML(p2toml))

	require.Equal(t, kp.Public.Addr, p2.Addr)
	require.Equal(t, kp.Public.TLS, p2.TLS)
	require.Equal(t, kp.Public.Key.String(), p2.Key.String())
}

func TestKeySignature(t *testing.T) {
	kp, err := NewTLSKeyPair(testAddr, nil)
	require.NoError(t, err)
	validSig := kp.Public.Signature
	require.NoError(t, kp.Public.ValidSignature())
	kp.Public.Signature = []byte("no justice, no peace")
	require.Error(t, kp.Public.ValidSignature())
	kp.Public.Signature = validSig

	ptoml := kp.Public.TOML().(*PublicTOML)
	id2 := new(Identity)
	require.NoError(t, id2.FromTOML(ptoml))
	require.NoError(t, id2.ValidSignature())
	ptoml.Signature = ""
	id2.Signature = nil
	require.NoError(t, id2.FromTOML(ptoml))
	require.Error(t, id2.ValidSignature(), id2.Signature)

	protoID := kp.Public.ToProto()
	decodedID, err := IdentityFromProto(protoID, kp.Public.Scheme)
	require.NoError(t, err)
	require.NoError(t, decodedID.ValidSignature())
	protoID.Signature = []byte("I am insane. And you are my insanity")
	decodedID, err = IdentityFromProto(protoID, kp.Public.Scheme)
	require.NoError(t, err)
	require.Error(t, decodedID.ValidSignature())
}

func TestKeyDistributedPublic(t *testing.T) {
	n := 4
	publics := make([]kyber.Point, n)
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	for i := range publics {
		key := sch.KeyGroup.Scalar().Pick(random.New())
		publics[i] = sch.KeyGroup.Point().Mul(key, nil)
	}

	distPublic := &DistPublic{Coefficients: publics}
	dtoml := distPublic.TOML()

	var writer bytes.Buffer
	enc := toml.NewEncoder(&writer)
	require.NoError(t, enc.Encode(dtoml))
	require.NoError(t, toml.NewEncoder(os.Stdout).Encode(dtoml))

	d2 := new(DistPublic)
	d2Toml := new(DistPublicTOML)
	_, err = toml.NewDecoder(&writer).Decode(d2Toml)
	require.NoError(t, err)
	require.NoError(t, d2.FromTOML(sch, d2Toml))

	var coeffs []string
	for i := 0; i < n; i++ {
		b, _ := publics[i].MarshalBinary()
		coeffs = append(coeffs, hex.EncodeToString(b))
	}
	checkTOML := &DistPublicTOML{Coefficients: coeffs}

	check := &DistPublic{}
	require.NoError(t, check.FromTOML(sch, checkTOML))
}

func TestKeyGroup(t *testing.T) {
	n := 5
	_, group := BatchIdentities(t, n)
	ids := group.Nodes
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
	s.Commits = make([]kyber.Point, n)
	s.Scheme, _ = crypto.GetSchemeFromEnv()
	for i := 0; i < n; i++ {
		s.Commits[i] = s.Scheme.KeyGroup.Point().Pick(random.New())
	}
	s.Share = &share.PriShare{V: s.Scheme.KeyGroup.Scalar().Pick(random.New()), I: 0}

	stoml := s.TOML()
	s2 := new(Share)
	s2.Scheme = s.Scheme
	require.NoError(t, s2.FromTOML(stoml))
	poly1 := s.Commits
	poly2 := s2.Commits
	require.Equal(t, len(poly1), len(poly2))
	for i := range poly1 {
		require.Equal(t, poly1[i].String(), poly2[i].String())
	}
}

func BatchIdentities(t *testing.T, n int) ([]*Pair, *Group) {
	startPort := 8000
	startAddr := "127.0.0.1:"
	privs := make([]*Pair, n)
	pubs := make([]*Node, n)

	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	for i := 0; i < n; i++ {
		port := strconv.Itoa(startPort + i)
		addr := startAddr + port
		privs[i], _ = NewTLSKeyPair(addr, sch)
		pubs[i] = &Node{
			Index:    uint32(i),
			Identity: privs[i].Public,
		}
	}
	fakeDistKey := sch.KeyGroup.Point().Pick(random.New())
	distKey := &DistPublic{Coefficients: []kyber.Point{fakeDistKey}}
	group := &Group{
		Threshold: DefaultThreshold(n),
		Nodes:     pubs,
		PublicKey: distKey,
		Scheme:    sch,
	}
	return privs, group
}
