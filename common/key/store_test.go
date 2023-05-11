package key

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	commonutils "github.com/drand/drand/common"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/share/dkg"
)

func TestKeysSaveLoad(t *testing.T) {
	n := 4
	ps, group := BatchIdentities(t, n)
	// we don't use the function from the test package here to avoid a circular dependency
	beaconID := commonutils.GetCanonicalBeaconID(os.Getenv("BEACON_ID"))

	tmp := path.Join(t.TempDir(), "drand-key")

	store := NewFileStore(tmp, beaconID).(*fileStore)
	require.Equal(t, tmp, store.baseFolder)

	// test loading saving private public key
	ps[0].Public.TLS = true
	require.NoError(t, store.SaveKeyPair(ps[0]))
	loadedKey, err := store.LoadKeyPair(nil)
	require.NoError(t, err)

	require.Equal(t, loadedKey.Key.String(), ps[0].Key.String())
	require.Equal(t, loadedKey.Public.Key.String(), ps[0].Public.Key.String())
	require.Equal(t, loadedKey.Public.Scheme.Name, ps[0].Public.Scheme.Name)
	require.Equal(t, loadedKey.Public.Address(), ps[0].Public.Address())
	require.True(t, loadedKey.Public.IsTLS())

	_, err = os.Stat(store.privateKeyFile)
	require.Nil(t, err)
	_, err = os.Stat(store.publicKeyFile)
	require.Nil(t, err)

	// test group
	require.Nil(t, store.SaveGroup(group))
	loadedGroup, err := store.LoadGroup()
	require.NoError(t, err)
	require.Equal(t, group.Threshold, loadedGroup.Threshold)

	// TODO remove that ordering thing it's useless
	for _, lid := range loadedGroup.Nodes {
		var found bool
		for _, k := range ps {
			if lid.Addr != k.Public.Addr {
				continue
			}
			found = true
			require.Equal(t, k.Public.Key.String(), lid.Key.String(), "public key should hold")
			require.Equal(t, k.Public.IsTLS(), lid.IsTLS(), "tls property should hold")
		}
		require.True(t, found, "not found key ", lid.Addr)
	}

	// test share / dist key
	testShare := &Share{
		DistKeyShare: dkg.DistKeyShare{
			Commits: []kyber.Point{ps[0].Public.Key, ps[1].Public.Key},
			Share:   &share.PriShare{V: ps[0].Key, I: 0},
		},
		Scheme: group.Scheme,
	}
	require.Nil(t, store.SaveShare(testShare))
	loadedShare, err := store.LoadShare(group.Scheme)

	require.NoError(t, err)
	require.Equal(t, testShare.Share.V, loadedShare.Share.V)
	require.Equal(t, testShare.Share.I, loadedShare.Share.I)
}
