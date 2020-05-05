package key

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	kyber "github.com/drand/kyber"
	"github.com/drand/kyber/util/random"
	"github.com/stretchr/testify/require"
)

func newIds(n int) []*Node {
	ids := make([]*Node, n)
	for i := 0; i < n; i++ {
		ids[i] = &Node{
			Index: uint32(i),
			Identity: &Identity{
				Key:  KeyGroup.Point().Mul(KeyGroup.Scalar().Pick(random.New()), nil),
				Addr: "--",
			},
		}
	}
	return ids
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
}
