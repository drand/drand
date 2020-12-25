package key

import (
	"bytes"
	"encoding/hex"
	"os"
	"strconv"
	"testing"

	"github.com/BurntSushi/toml"
	kyber "github.com/drand/kyber"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/util/random"
	"github.com/stretchr/testify/require"
)

const testAddr = "127.0.0.1:80"

func TestKeyPublic(t *testing.T) {
	kp := NewTLSKeyPair(testAddr)
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

func TestKeySignature(t *testing.T) {
	kp := NewTLSKeyPair(testAddr)
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
	decodedID, err := IdentityFromProto(protoID)
	require.NoError(t, err)
	require.NoError(t, decodedID.ValidSignature())
	protoID.Signature = []byte("I am insane. And you are my insanity")
	decodedID, err = IdentityFromProto(protoID)
	require.NoError(t, err)
	require.Error(t, decodedID.ValidSignature())
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

	var coeffs []string
	for i := 0; i < n; i++ {
		b, _ := publics[i].MarshalBinary()
		coeffs = append(coeffs, hex.EncodeToString(b))
	}
	checkTOML := &DistPublicTOML{Coefficients: coeffs}

	check := &DistPublic{}
	require.NoError(t, check.FromTOML(checkTOML))
}

func TestKeyGroup(t *testing.T) {
	n := 5
	_, group := BatchIdentities(n)
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
	for i := 0; i < n; i++ {
		s.Commits[i] = KeyGroup.Point().Pick(random.New())
	}
	s.Share = &share.PriShare{V: KeyGroup.Scalar().Pick(random.New()), I: 0}

	stoml := s.TOML()
	s2 := new(Share)
	require.NoError(t, s2.FromTOML(stoml))
	poly1 := s.Commits
	poly2 := s2.Commits
	require.Equal(t, len(poly1), len(poly2))
	for i := range poly1 {
		require.Equal(t, poly1[i].String(), poly2[i].String())
	}
}

func BatchIdentities(n int) ([]*Pair, *Group) {
	startPort := 8000
	startAddr := "127.0.0.1:"
	privs := make([]*Pair, n)
	pubs := make([]*Node, n)
	for i := 0; i < n; i++ {
		port := strconv.Itoa(startPort + i)
		addr := startAddr + port
		privs[i] = NewTLSKeyPair(addr)
		pubs[i] = &Node{
			Index:    uint32(i),
			Identity: privs[i].Public,
		}
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
