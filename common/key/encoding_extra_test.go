package key

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/crypto"
	proto "github.com/drand/drand/v2/protobuf/drand"
	"go.dedis.ch/kyber/v4"
	"go.dedis.ch/kyber/v4/share"
	"go.dedis.ch/kyber/v4/util/random"
)

// schemes returns every registered scheme so we can exercise the code paths
// across all curve configurations.
func schemes(t *testing.T) []*crypto.Scheme {
	t.Helper()
	ids := crypto.ListSchemes()
	out := make([]*crypto.Scheme, 0, len(ids))
	for _, id := range ids {
		sch, err := crypto.GetSchemeByID(id)
		require.NoError(t, err)
		out = append(out, sch)
	}
	require.NotEmpty(t, out)
	return out
}

func batchForScheme(t *testing.T, n int, sch *crypto.Scheme) ([]*Pair, *Group) {
	t.Helper()
	privs := make([]*Pair, n)
	pubs := make([]*Node, n)
	for i := 0; i < n; i++ {
		addr := "127.0.0.1:" + strconv.Itoa(9000+i)
		p, err := NewKeyPair(addr, sch)
		require.NoError(t, err)
		privs[i] = p
		pubs[i] = &Node{Index: uint32(i), Identity: p.Public}
	}
	fakeDistKey := sch.KeyGroup.Point().Pick(random.New())
	group := &Group{
		Threshold: DefaultThreshold(n),
		Period:    30 * time.Second,
		Nodes:     pubs,
		PublicKey: &DistPublic{Coefficients: []kyber.Point{fakeDistKey}},
		Scheme:    sch,
		ID:        "default",
	}
	return privs, group
}

func TestEncodingPointScalarRoundTrip(t *testing.T) {
	for _, sch := range schemes(t) {
		sch := sch
		t.Run(sch.Name, func(t *testing.T) {
			scalar := sch.KeyGroup.Scalar().Pick(random.New())
			point := sch.KeyGroup.Point().Mul(scalar, nil)

			ps := PointToString(point)
			gotP, err := StringToPoint(sch.KeyGroup, ps)
			require.NoError(t, err)
			require.True(t, point.Equal(gotP))

			ss := ScalarToString(scalar)
			gotS, err := StringToScalar(sch.KeyGroup, ss)
			require.NoError(t, err)
			require.True(t, scalar.Equal(gotS))
		})
	}
}

func TestEncodingErrors(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)

	// invalid hex
	_, err = StringToPoint(sch.KeyGroup, "zzzz")
	require.Error(t, err)
	_, err = StringToScalar(sch.KeyGroup, "zzzz")
	require.Error(t, err)

	// valid hex but wrong length for a point
	_, err = StringToPoint(sch.KeyGroup, "00112233")
	require.Error(t, err)
}

func TestPairTOMLRoundTrip(t *testing.T) {
	for _, sch := range schemes(t) {
		sch := sch
		t.Run(sch.Name, func(t *testing.T) {
			p, err := NewKeyPair("127.0.0.1:7000", sch)
			require.NoError(t, err)
			require.Equal(t, sch.Name, p.Scheme().Name)

			ptoml := p.TOML().(*PairTOML)
			require.Equal(t, sch.Name, ptoml.SchemeName)

			p2 := new(Pair)
			require.IsType(t, &PairTOML{}, p2.TOMLValue())
			require.NoError(t, p2.FromTOML(ptoml))
			require.True(t, p.Key.Equal(p2.Key))
			require.Equal(t, sch.Name, p2.Public.Scheme.Name)
		})
	}
}

func TestPairFromTOMLErrors(t *testing.T) {
	p := new(Pair)
	// wrong concrete type
	require.Error(t, p.FromTOML(&PublicTOML{}))
	// unknown scheme name
	require.Error(t, p.FromTOML(&PairTOML{Key: "00", SchemeName: "does-not-exist"}))
	// invalid scalar hex
	require.Error(t, p.FromTOML(&PairTOML{Key: "zz", SchemeName: crypto.DefaultSchemeID}))
}

func TestIdentityTOMLRoundTripAndSig(t *testing.T) {
	for _, sch := range schemes(t) {
		sch := sch
		t.Run(sch.Name, func(t *testing.T) {
			p, err := NewKeyPair("127.0.0.1:7001", sch)
			require.NoError(t, err)
			require.NoError(t, p.Public.ValidSignature())

			id2 := new(Identity)
			require.IsType(t, &PublicTOML{}, id2.TOMLValue())
			require.NoError(t, id2.FromTOML(p.Public.TOML()))
			require.NoError(t, id2.ValidSignature())
			require.True(t, p.Public.Equal(id2))
			require.Equal(t, p.Public.Address(), id2.Address())
			require.NotEmpty(t, id2.String())
			require.NotEmpty(t, id2.Hash())
		})
	}
}

func TestIdentityTOMLNilScheme(t *testing.T) {
	p, err := NewKeyPair("127.0.0.1:7002", nil)
	require.NoError(t, err)
	id := p.Public
	id.Scheme = nil
	ptoml := id.TOML().(*PublicTOML)
	require.Equal(t, "nil scheme", ptoml.SchemeName)
}

func TestIdentityFromTOMLErrors(t *testing.T) {
	id := new(Identity)
	// wrong type
	require.Error(t, id.FromTOML(&PairTOML{}))
	// unknown scheme
	require.Error(t, id.FromTOML(&PublicTOML{SchemeName: "nope", Key: "00"}))
	// invalid key hex
	require.Error(t, id.FromTOML(&PublicTOML{SchemeName: crypto.DefaultSchemeID, Key: "zz"}))

	// valid key but invalid signature hex
	p, err := NewKeyPair("127.0.0.1:7003", nil)
	require.NoError(t, err)
	good := p.Public.TOML().(*PublicTOML)
	good.Signature = "zz"
	require.Error(t, id.FromTOML(good))
}

func TestSelfSignChangesSignature(t *testing.T) {
	p, err := NewKeyPair("127.0.0.1:7004", nil)
	require.NoError(t, err)
	p.Public.Signature = nil
	require.Error(t, p.Public.ValidSignature())
	require.NoError(t, p.SelfSign())
	require.NotNil(t, p.Public.Signature)
	require.NoError(t, p.Public.ValidSignature())
}

func TestIdentityValidSignatureWrong(t *testing.T) {
	p, err := NewKeyPair("127.0.0.1:7005", nil)
	require.NoError(t, err)
	p.Public.Signature = []byte("garbage signature")
	require.Error(t, p.Public.ValidSignature())
}

func TestIdentityEqual(t *testing.T) {
	p1, err := NewKeyPair("127.0.0.1:7006", nil)
	require.NoError(t, err)
	p2, err := NewKeyPair("127.0.0.1:7007", nil)
	require.NoError(t, err)

	require.True(t, p1.Public.Equal(p1.Public))
	// different address
	require.False(t, p1.Public.Equal(p2.Public))
	// same address, different key
	clone := &Identity{Key: p2.Public.Key, Addr: p1.Public.Addr, Scheme: p2.Public.Scheme}
	require.False(t, p1.Public.Equal(clone))
}

func TestIdentityFromProtoErrors(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	p, err := NewKeyPair("127.0.0.1:7008", sch)
	require.NoError(t, err)
	good := p.Public.ToProto()

	// happy path
	id, err := IdentityFromProto(good, sch)
	require.NoError(t, err)
	require.NoError(t, id.ValidSignature())

	// invalid address (no port)
	bad := &proto.Identity{Address: "not-an-addr", Key: good.Key, Signature: good.Signature}
	_, err = IdentityFromProto(bad, sch)
	require.Error(t, err)

	// nil scheme
	_, err = IdentityFromProto(good, nil)
	require.Error(t, err)

	// corrupt key bytes
	corrupt := &proto.Identity{Address: good.Address, Key: []byte{0x00, 0x01}, Signature: good.Signature}
	_, err = IdentityFromProto(corrupt, sch)
	require.ErrorIs(t, err, ErrInvalidKeyScheme)
}

func TestGroupFindAndNode(t *testing.T) {
	privs, group := batchForScheme(t, 5, mustScheme(t))

	// Find returns a fresh copy that is Equal to the original identity
	target := privs[2].Public
	found := group.Find(target)
	require.NotNil(t, found)
	require.True(t, found.Identity.Equal(target))
	require.Equal(t, uint32(2), found.Index)
	// returned object is a copy, not the same pointer
	require.NotSame(t, group.Nodes[2].Identity, found.Identity)

	// unknown identity
	other, err := NewKeyPair("127.0.0.1:19999", mustScheme(t))
	require.NoError(t, err)
	require.Nil(t, group.Find(other.Public))

	// Node lookup by index
	require.NotNil(t, group.Node(3))
	require.Equal(t, uint32(3), group.Node(3).Index)
	require.Nil(t, group.Node(99))
}

func TestGroupHashStableAndDistinct(t *testing.T) {
	_, group := batchForScheme(t, 4, mustScheme(t))
	h1 := group.Hash()
	h2 := group.Hash()
	require.Equal(t, h1, h2)
	require.Len(t, h1, 32)

	// changing the threshold changes the hash
	group.Threshold++
	require.NotEqual(t, h1, group.Hash())
}

func TestGroupEqual(t *testing.T) {
	_, group := batchForScheme(t, 4, mustScheme(t))

	// nil handling
	var nilGroup *Group
	require.True(t, nilGroup.Equal(nil))
	require.False(t, nilGroup.Equal(group))
	require.False(t, group.Equal(nil))

	// equal to a deep-ish copy
	cp := *group
	require.True(t, group.Equal(&cp))

	// different threshold
	diff := *group
	diff.Threshold = group.Threshold + 1
	require.False(t, group.Equal(&diff))

	// different period
	diffP := *group
	diffP.Period = group.Period + time.Second
	require.False(t, group.Equal(&diffP))

	// different beacon id
	diffID := *group
	diffID.ID = "another"
	require.False(t, group.Equal(&diffID))

	// one has a public key, other does not
	noKey := *group
	noKey.PublicKey = nil
	require.False(t, group.Equal(&noKey))
	require.False(t, noKey.Equal(group))
}

func TestGroupHelpers(t *testing.T) {
	_, group := batchForScheme(t, 3, mustScheme(t))
	require.Equal(t, 3, group.Len())
	require.Len(t, group.Points(), 3)
	require.Len(t, group.DKGNodes(), 3)
	require.NotEmpty(t, group.String())
	require.NotNil(t, group.GetGenesisSeed())
}

func TestShareTOMLAcrossSchemes(t *testing.T) {
	for _, sch := range schemes(t) {
		sch := sch
		t.Run(sch.Name, func(t *testing.T) {
			n := 4
			s := new(Share)
			s.Scheme = sch
			s.Commits = make([]kyber.Point, n)
			for i := 0; i < n; i++ {
				s.Commits[i] = sch.KeyGroup.Point().Pick(random.New())
			}
			s.Share = &share.PriShare{V: sch.KeyGroup.Scalar().Pick(random.New()), I: 1}

			require.NotNil(t, s.PubPoly())
			require.NotNil(t, s.PrivateShare())
			require.NotNil(t, s.Public())

			stoml := s.TOML().(*ShareTOML)
			require.Equal(t, sch.Name, stoml.SchemeName)

			s2 := new(Share)
			require.IsType(t, &ShareTOML{}, s2.TOMLValue())
			require.NoError(t, s2.FromTOML(stoml))
			require.Equal(t, s.Share.I, s2.Share.I)
			require.True(t, s.Share.V.Equal(s2.Share.V))
			require.Len(t, s2.Commits, n)
			for i := range s.Commits {
				require.True(t, s.Commits[i].Equal(s2.Commits[i]))
			}
		})
	}
}

func TestShareFromTOMLErrors(t *testing.T) {
	s := new(Share)
	require.Error(t, s.FromTOML(&PairTOML{}))
	require.Error(t, s.FromTOML(&ShareTOML{SchemeName: "unknown"}))
	require.Error(t, s.FromTOML(&ShareTOML{SchemeName: crypto.DefaultSchemeID, Commits: []string{"zz"}}))
	require.Error(t, s.FromTOML(&ShareTOML{SchemeName: crypto.DefaultSchemeID, Share: "zz"}))
}

func TestDistPublicTOMLAndHelpers(t *testing.T) {
	for _, sch := range schemes(t) {
		sch := sch
		t.Run(sch.Name, func(t *testing.T) {
			n := 3
			coeffs := make([]kyber.Point, n)
			for i := 0; i < n; i++ {
				coeffs[i] = sch.KeyGroup.Point().Pick(random.New())
			}
			d := &DistPublic{Coefficients: coeffs}
			require.NotNil(t, d.PubPoly(sch))
			require.True(t, coeffs[0].Equal(d.Key()))
			require.Len(t, d.Hash(), 32)

			dtoml := d.TOML().(*DistPublicTOML)
			d2 := new(DistPublic)
			require.IsType(t, &DistPublicTOML{}, d2.TOMLValue())
			require.NoError(t, d2.FromTOML(sch, dtoml))
			require.True(t, d.Equal(d2))
		})
	}
}

func TestDistPublicEqualAndErrors(t *testing.T) {
	sch := mustScheme(t)
	a := sch.KeyGroup.Point().Pick(random.New())
	b := sch.KeyGroup.Point().Pick(random.New())

	d1 := &DistPublic{Coefficients: []kyber.Point{a, b}}
	d2 := &DistPublic{Coefficients: []kyber.Point{a, b}}
	require.True(t, d1.Equal(d2))

	// different length
	require.False(t, d1.Equal(&DistPublic{Coefficients: []kyber.Point{a}}))
	// different coefficient
	require.False(t, d1.Equal(&DistPublic{Coefficients: []kyber.Point{a, a}}))

	// FromTOML errors
	d := new(DistPublic)
	require.Error(t, d.FromTOML(sch, &PairTOML{}))
	require.Error(t, d.FromTOML(sch, &DistPublicTOML{Coefficients: []string{"zz"}}))
}

func TestNodeTOMLRoundTrip(t *testing.T) {
	p, err := NewKeyPair("127.0.0.1:7100", mustScheme(t))
	require.NoError(t, err)
	n := &Node{Index: 7, Identity: p.Public}

	require.Len(t, n.Hash(), 32)
	require.IsType(t, &NodeTOML{}, n.TOMLValue())

	ntoml := n.TOML().(*NodeTOML)
	require.Equal(t, uint32(7), ntoml.Index)

	n2 := new(Node)
	require.NoError(t, n2.FromTOML(ntoml))
	require.True(t, n.Equal(n2))

	// not equal: different index
	n3 := &Node{Index: 8, Identity: p.Public}
	require.False(t, n.Equal(n3))
}

func TestMinimumTAndDefaultThreshold(t *testing.T) {
	require.Equal(t, 1, MinimumT(1))
	require.Equal(t, 2, MinimumT(2))
	require.Equal(t, 2, MinimumT(3))
	require.Equal(t, 3, MinimumT(4))
	require.Equal(t, MinimumT(5), DefaultThreshold(5))
}

func mustScheme(t *testing.T) *crypto.Scheme {
	t.Helper()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	return sch
}
