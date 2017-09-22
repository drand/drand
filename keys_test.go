package main

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

type TmpKeyValue struct {
	values map[string]string
}

func NewTmpKeyValue(folder string) KeyValue {
	return &TmpKeyValue{
		values: map[string]string{
			keyFolderFlagName: folder,
			groupFileFlagName: path.Join(folder, defaultGroupFile_+groupExtension),
			sigFolderFlagName: path.Join(folder, defaultSigFolder_),
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

func TestKeysSaveLoad(t *testing.T) {
	ps, group := BatchIdentities(4)
	tmp := os.TempDir()
	defer os.RemoveAll(tmp)
	kv := NewTmpKeyValue(tmp)
	store := NewFileStore(kv)

	// test loading saving private public key
	require.Nil(t, store.SaveKey(ps[0]))
	loadedKey, err := store.LoadKey()
	require.Nil(t, err)
	require.Equal(t, loadedKey.Key.String(), ps[0].Key.String())
	require.Equal(t, loadedKey.Public.Key.String(), ps[0].Public.Key.String())
	require.True(t, fileExists(tmp, defaultKeyFile+privateExtension))
	require.True(t, fileExists(tmp, defaultKeyFile+publicExtension))

	// test group
	groupPath := path.Join(tmp, defaultGroupFile_+groupExtension)
	require.Nil(t, store.Save(groupPath, group, false))

	_, err = store.LoadGroup()
	require.Nil(t, err)
	// XXX do testing on group inerts
}

func TestKeysGroupPoint(t *testing.T) {
	n := 5
	_, group := BatchIdentities(n)
	points := group.Points()
	for i, p := range points {
		k := group.Public(i).Key
		require.Equal(t, p.String(), k.String())
	}
}
