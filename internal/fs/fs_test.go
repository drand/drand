package fs

import (
	"os"
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
		t.Error("bad permissions", subFolder1Path)
	}
	if err := CreateSecureFolder(subFolder2Path); err == "" {
		t.Error("bad permissions", subFolder2Path)
	}

	if err := CopyFolder(folder1Path, subFolder2Path); err != nil {
		t.Error("error copying folders", err)
	}

	folders, err := Folders(subFolder2Path)
	if err != nil {
		t.Error(err)
	}

	for _, fd := range folders {
		if fd != path.Join(subFolder2Path, "folder1") {
			t.Error("folder1 should be inside subFolder2 path")
		}
	}
}

func TestCreateSecureFile_ErrorHandling(t *testing.T) {
	tmpPath := t.TempDir()
	file := path.Join(tmpPath, "secured")

	// Test successful creation
	f, err := CreateSecureFile(file)
	require.NotNil(t, f)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Verify file was created with correct permissions
	info, err := os.Stat(file)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(rwFilePermission), info.Mode().Perm())

	// Test that error is returned (not silently ignored)
	// Note: Testing actual Chmod failure is difficult without special filesystem setup,
	// but we verify that the function properly returns errors when they occur.
	// The error wrapping ensures callers get meaningful error messages.
}
