package key

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/drand/drand/protobuf/drand"
	kyber "github.com/drand/kyber"
	"github.com/drand/kyber/util/random"
	"github.com/stretchr/testify/require"
)

func newIds(n int) []*Node {
	ids := make([]*Node, n)
	for i := 0; i < n; i++ {
		ids[i] = &Node{
			Index:    uint32(i),
			Identity: NewKeyPair("127.0.0.1:3000").Public,
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
	ids := newIds(n)

	dpub := []kyber.Point{KeyGroup.Point().Pick(random.New())}
	group := LoadGroup(ids, 1, &DistPublic{dpub}, 30*time.Second, 61)
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
		dpub2 = append(dpub2, KeyGroup.Point().Pick(random.New()))
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

	for i, tv := range vectors {
		protoGroup := tv.group.ToProto()
		if tv.change != nil {
			tv.change(protoGroup)
		}

		loaded, err := GroupFromProto(protoGroup)
		if tv.isErr {
			require.Error(t, err)
			continue
		}
		// load the seed after
		seed := tv.group.GetGenesisSeed()
		require.Equal(t, len(loaded.Nodes), len(tv.group.Nodes), "test %d", i)
		require.Equal(t, loaded.Threshold, tv.group.Threshold)
		require.True(t, loaded.PublicKey.Equal(tv.group.PublicKey), "test %d: %v vs %v", i, loaded.PublicKey, group.PublicKey)
		require.Equal(t, loaded.Period, tv.group.Period)
		require.Equal(t, seed, loaded.GetGenesisSeed())
		require.Equal(t, genesis, loaded.GenesisTime)
		require.Equal(t, transition, loaded.TransitionTime)
		require.Equal(t, tv.group.Hash(), loaded.Hash())
	}
}

func TestGroupUnsignedIdentities(t *testing.T) {
	ids := newIds(5)
	group := LoadGroup(ids, 1, &DistPublic{[]kyber.Point{KeyGroup.Point()}}, 30*time.Second, 61)
	require.Nil(t, group.UnsignedIdentities())

	ids[0].Signature = nil
	require.Len(t, group.UnsignedIdentities(), 1)

	ids[1].Signature = []byte("silver linings")
	require.Len(t, group.UnsignedIdentities(), 2)
}
func TestGroupSaveLoad(t *testing.T) {
	n := 3
	ids := newIds(n)
	dpub := []kyber.Point{KeyGroup.Point().Pick(random.New())}
	group := LoadGroup(ids, 1, &DistPublic{dpub}, 30*time.Second, 61)
	group.Threshold = 3
	group.Period = time.Second * 4
	group.GenesisTime = time.Now().Add(10 * time.Second).Unix()
	group.TransitionTime = time.Now().Add(10 * time.Second).Unix()

	genesis := group.GenesisTime
	transition := group.TransitionTime

	gtoml := group.TOML().(*GroupTOML)
	require.NotNil(t, gtoml.PublicKey)

	// faking distributed public key coefficients
	groupFile, err := ioutil.TempFile("", "group.toml")
	require.NoError(t, err)
	groupPath := groupFile.Name()
	groupFile.Close()
	defer os.RemoveAll(groupPath)

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

// BatchIdentities generates n insecure identities
func makeGroup(t *testing.T) *Group {
	t.Helper()

	fakeKey := KeyGroup.Point().Pick(random.New())
	group := LoadGroup([]*Node{}, 1, &DistPublic{Coefficients: []kyber.Point{fakeKey}}, 30*time.Second, 0)
	group.Threshold = MinimumT(0)
	return group
}

func TestConvertGroup(t *testing.T) {
	group := makeGroup(t)
	group.Period = 5 * time.Second
	group.TransitionTime = time.Now().Unix()
	group.GenesisTime = time.Now().Unix()

	proto := group.ToProto()
	received, err := GroupFromProto(proto)
	require.NoError(t, err)
	require.True(t, received.Equal(group))
}
