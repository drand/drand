package key

import (
	"os"
	"path"
	"testing"

	kyber "github.com/dedis/kyber"
	"github.com/dedis/kyber/share"
	"github.com/stretchr/testify/require"
)

type TmpKeyValue struct {
	values map[string]string
}

func NewTmpKeyValue(folder string) KeyValue {
	return &TmpKeyValue{
		values: map[string]string{
			ConfigFolderFlag: folder,
		},
	}
}

func (t *TmpKeyValue) String(key string) string {
	s, ok := t.values[key]
	if !ok {
		panic("wrong testing man")
	}
	return s
}

func (t *TmpKeyValue) IsSet(key string) bool {
	_, ok := t.values[key]
	return ok
}

func TestKeysSaveLoad(t *testing.T) {
	n := 4
	ps, group := BatchIdentities(n)
	tmp := os.TempDir()
	tmp = path.Join(tmp, "drand")
	os.RemoveAll(tmp)
	defer os.RemoveAll(tmp)
	kv := NewTmpKeyValue(tmp)
	store := NewFileStore(kv).(*fileStore)
	require.Equal(t, tmp, store.baseFolder)

	// test loading saving private public key

	require.Nil(t, store.SavePrivate(ps[0]))
	loadedKey, err := store.LoadPrivate()
	require.Nil(t, err)
	require.Equal(t, loadedKey.Key.String(), ps[0].Key.String())
	require.Equal(t, loadedKey.Public.Key.String(), ps[0].Public.Key.String())
	require.Equal(t, loadedKey.Public.Address(), ps[0].Public.Address())
	require.True(t, fileExists(path.Join(tmp, keyFolderName), keyFileName+privateExtension))
	require.True(t, fileExists(path.Join(tmp, keyFolderName), keyFileName+publicExtension))

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
