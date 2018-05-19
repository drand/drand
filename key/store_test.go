package key

import (
	"os"
	"path"
	"testing"

	"github.com/dedis/drand/fs"
	kyber "github.com/dedis/kyber"
	"github.com/dedis/kyber/share"
	"github.com/stretchr/testify/require"
)

func TestKeysSaveLoad(t *testing.T) {
	n := 4
	ps, group := BatchIdentities(n)
	tmp := os.TempDir()
	tmp = path.Join(tmp, "drand")
	os.RemoveAll(tmp)
	defer os.RemoveAll(tmp)
	store := NewFileStore(tmp).(*fileStore)
	require.Equal(t, tmp, store.baseFolder)

	// test loading saving private public key

	require.Nil(t, store.SaveKeyPair(ps[0]))
	loadedKey, err := store.LoadKeyPair()
	require.Nil(t, err)
	require.Equal(t, loadedKey.Key.String(), ps[0].Key.String())
	require.Equal(t, loadedKey.Public.Key.String(), ps[0].Public.Key.String())
	require.Equal(t, loadedKey.Public.Address(), ps[0].Public.Address())
	require.True(t, fs.FileExists(path.Join(tmp, KeyFolderName), keyFileName+privateExtension))
	require.True(t, fs.FileExists(path.Join(tmp, KeyFolderName), keyFileName+publicExtension))

	// test group
	require.Nil(t, store.SaveGroup(group))
	loadedGroup, err := store.LoadGroup()
	require.NoError(t, err)
	require.Equal(t, group.Threshold, loadedGroup.Threshold)
	for i := 0; i < n; i++ {
		require.True(t, loadedGroup.Contains(ps[i].Public))
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

	dp := &DistPublic{Key: ps[0].Public.Key}
	require.Nil(t, store.SaveDistPublic(dp))
	loadedDp, err := store.LoadDistPublic()
	require.NoError(t, err)
	require.Equal(t, dp.Key.String(), loadedDp.Key.String())

}
