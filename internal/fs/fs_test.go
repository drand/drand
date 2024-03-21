package fs

import (
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecureDirAlreadyHere(t *testing.T) {
	tmpPath := path.Join(t.TempDir(), "config")

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

func TestCopyFolder(t *testing.T) {
	tmpPath := path.Join(t.TempDir(), "tmp")
	folder1Path := path.Join(tmpPath, "folder1")
	subFolder1Path := path.Join(folder1Path, "folder1")

	folder2Path := path.Join(tmpPath, "folder2")
	subFolder2Path := path.Join(folder2Path, "folder2")

	if err := CreateSecureFolder(subFolder1Path); err == "" {
		t.Errorf(err)
	}
	if err := CreateSecureFolder(subFolder2Path); err == "" {
		t.Errorf(err)
	}

	if err := CopyFolder(folder1Path, subFolder2Path); err != nil {
		t.Errorf(err.Error())
	}

	folders, err := Folders(subFolder2Path)
	if err != nil {
		t.Errorf(err.Error())
	}

	for _, fd := range folders {
		if fd != path.Join(subFolder2Path, "folder1") {
			t.Errorf("folder1 should be inside subFolder2 path")
		}
	}
}
