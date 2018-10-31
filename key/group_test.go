package key

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	kyber "github.com/dedis/kyber"
	"github.com/dedis/kyber/util/random"
	"github.com/stretchr/testify/require"
)

func TestGroupSaveLoad(t *testing.T) {
	n := 3
	ids := make([]*Identity, n)
	dpub := make([]kyber.Point, n)
	for i := 0; i < n; i++ {
		ids[i] = &Identity{
			Key:  G2.Point().Mul(G2.Scalar().Pick(random.New()), nil),
			Addr: "--",
		}
		dpub[i] = ids[i].Key
	}

	group := LoadGroup(ids, &DistPublic{dpub}, DefaultThreshold(n))
	group.Period = time.Second * 4

	gtoml := group.TOML().(*GroupTOML)
	require.NotNil(t, gtoml.PublicKey)

	// faking distributed public key coefficients
	groupFile, err := ioutil.TempFile("", "group.toml")
	require.NoError(t, err)
	groupPath := groupFile.Name()
	groupFile.Close()
	defer os.RemoveAll(groupPath)

	require.NoError(t, Save(groupPath, group, false))

	loaded := &Group{}
	require.NoError(t, Load(groupPath, loaded))

	require.Equal(t, loaded.Nodes, group.Nodes)
	require.Equal(t, loaded.Threshold, group.Threshold)
	require.Equal(t, loaded.PublicKey, group.PublicKey)
	require.Equal(t, loaded.Period, group.Period)
}
