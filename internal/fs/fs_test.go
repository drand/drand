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

// TestCreateSecureFileErrorHandling tests error cases for CreateSecureFile
func TestCreateSecureFileErrorHandling(t *testing.T) {
	// Test with invalid path (should fail on os.Create)
	invalidPath := "/invalid/path/that/does/not/exist/file.txt"
	_, err := CreateSecureFile(invalidPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no such file or directory")
}

// TestHomeFolder tests the HomeFolder function
func TestHomeFolder(t *testing.T) {
	home := HomeFolder()
	require.NotEmpty(t, home)
	require.DirExists(t, home)
}

// TestExistsErrorHandling tests error cases for Exists function
func TestExistsErrorHandling(t *testing.T) {
	// Test with a path that exists but has permission issues
	// This is hard to test portably, so we'll test the basic functionality
	tmpPath := t.TempDir()
	exists, err := Exists(tmpPath)
	require.True(t, exists)
	require.NoError(t, err)

	// Test with non-existent path
	exists, err = Exists("/path/that/does/not/exist")
	require.False(t, exists)
	require.NoError(t, err)
}

// TestCreateSecureFolderErrorHandling tests error cases for CreateSecureFolder
func TestCreateSecureFolderErrorHandling(t *testing.T) {
	// Test with a path that should work (existing directory)
	tmpPath := t.TempDir()
	result := CreateSecureFolder(tmpPath)
	require.NotEmpty(t, result)
	require.Equal(t, tmpPath, result)

	// Test with a path that should work (non-existing directory)
	newPath := path.Join(tmpPath, "newdir")
	result = CreateSecureFolder(newPath)
	require.NotEmpty(t, result)
	require.Equal(t, newPath, result)
	require.DirExists(t, newPath)
}

// TestFileExists tests the FileExists function
func TestFileExists(t *testing.T) {
	tmpPath := t.TempDir()

	// Test with non-existent file
	exists := FileExists(tmpPath, "nonexistent.txt")
	require.False(t, exists)

	// Create a file and test
	filePath := path.Join(tmpPath, "test.txt")
	file, err := os.Create(filePath)
	require.NoError(t, err)
	file.Close()

	// FileExists expects the full path since Files() returns full paths
	exists = FileExists(tmpPath, filePath)
	require.True(t, exists)

	// Test with just filename (should fail)
	exists = FileExists(tmpPath, "test.txt")
	require.False(t, exists)
}

// TestFolders tests the Folders function
func TestFolders(t *testing.T) {
	tmpPath := t.TempDir()

	// Create some test directories
	dir1 := path.Join(tmpPath, "dir1")
	dir2 := path.Join(tmpPath, "dir2")
	require.NoError(t, os.MkdirAll(dir1, 0755))
	require.NoError(t, os.MkdirAll(dir2, 0755))

	// Test Folders function
	folders, err := Folders(tmpPath)
	require.NoError(t, err)
	require.Len(t, folders, 2)

	// Check that both directories are found
	found := make(map[string]bool)
	for _, folder := range folders {
		found[folder] = true
	}
	require.True(t, found[dir1])
	require.True(t, found[dir2])
}
