package key

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	kyber "go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/share"
)

func TestKeysSaveLoad(t *testing.T) {
	n := 4
	ps, group := BatchIdentities(n)
	tmp := os.TempDir()
	tmp = path.Join(tmp, "drand-key")
	os.RemoveAll(tmp)
	defer os.RemoveAll(tmp)
	store := NewFileStore(tmp).(*fileStore)
	require.Equal(t, tmp, store.baseFolder)

	// test loading saving private public key
	ps[0].Public.TLS = true
	require.NoError(t, store.SaveKeyPair(ps[0]))
	loadedKey, err := store.LoadKeyPair()
	require.NoError(t, err)
	require.Equal(t, loadedKey.Key.String(), ps[0].Key.String())
	require.Equal(t, loadedKey.Public.Key.String(), ps[0].Public.Key.String())
	require.Equal(t, loadedKey.Public.Address(), ps[0].Public.Address())
	require.True(t, loadedKey.Public.IsTLS())

	_, err = os.Stat(store.privateKeyFile)
	require.Nil(t, err)
	_, err = os.Stat(store.publicKeyFile)
	require.Nil(t, err)
	//require.True(t, fs.FileExists(store.privateKeyFile))
	//require.True(t, fs.FileExists(store.publicKeyFile))

	// test group
	require.Nil(t, store.SaveGroup(group))
	loadedGroup, err := store.LoadGroup()
	require.NoError(t, err)
	require.Equal(t, group.Threshold, loadedGroup.Threshold)
	// TODO remove that ordering thing it's useless
	for _, lid := range loadedGroup.Identities() {
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
	share := &Share{
		Commits: []kyber.Point{ps[0].Public.Key, ps[1].Public.Key},
		Share:   &share.PriShare{V: ps[0].Key, I: 0},
	}
	require.Nil(t, store.SaveShare(share))
	loadedShare, err := store.LoadShare()
	require.NoError(t, err)
	require.Equal(t, share.Share.V, loadedShare.Share.V)
	require.Equal(t, share.Share.I, loadedShare.Share.I)

	dp := &DistPublic{[]kyber.Point{ps[0].Public.Key}}
	require.Nil(t, store.SaveDistPublic(dp))
	loadedDp, err := store.LoadDistPublic()
	require.NoError(t, err)
	require.Equal(t, dp.Key().String(), loadedDp.Key().String())

}
