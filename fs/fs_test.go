package fs

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecureDirAlreadyHere(t *testing.T) {
	tmpPath := path.Join(os.TempDir(), "config")
	os.Mkdir(tmpPath, 0740)
	defer os.RemoveAll(tmpPath)
	fpath := CreateSecureFolder(tmpPath)
	require.NotNil(t, fpath)

	npath := CreateSecureFolder(tmpPath)
	require.Equal(t, fpath, npath)
	b, e := Exists(npath)
	require.True(t, b)
	require.NoError(t, e)
	b, e = Exists(path.Join(tmpPath, "blou"))
	require.False(t, b)
	require.NoError(t, e)

	file := path.Join(tmpPath, "secured")
	f, err := CreateSecureFile(file)
	require.NotNil(t, f)
	require.NoError(t, err)
	file2 := path.Join(tmpPath, "secured")

	files, err := Files(tmpPath)
	require.NoError(t, err)
	for _, f := range files {
		var found bool
		for _, toFind := range []string{file, file2} {
			if toFind == f {
				found = true
				break
			}
		}
		require.True(t, found)
	}

	for _, f := range []string{file, file2} {
		require.True(t, FileExists(tmpPath, f))
	}
}
