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

func newIds(n int) []*Identity {
	ids := make([]*Identity, n)
	for i := 0; i < n; i++ {
		ids[i] = &Identity{
			Key:  KeyGroup.Point().Mul(KeyGroup.Scalar().Pick(random.New()), nil),
			Addr: "--",
		}
	}
	return ids
}

func TestGroupSaveLoad(t *testing.T) {
	n := 3
	ids := newIds(n)
	dpub := []kyber.Point{KeyGroup.Point().Pick(random.New())}
	group := LoadGroup(ids, &DistPublic{dpub}, DefaultThreshold(n))
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
