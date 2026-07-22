package fs

import (
	"os"
	"os/user"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHomeFolder(t *testing.T) {
	home := HomeFolder()
	require.NotEmpty(t, home)

	u, err := user.Current()
	require.NoError(t, err)
	require.Equal(t, u.HomeDir, home)
}

func TestExists(t *testing.T) {
	tmp := t.TempDir()

	// existing directory
	ok, err := Exists(tmp)
	require.NoError(t, err)
	require.True(t, ok)

	// existing file
	f := path.Join(tmp, "afile")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
	ok, err = Exists(f)
	require.NoError(t, err)
	require.True(t, ok)

	// missing path
	ok, err = Exists(path.Join(tmp, "nope"))
	require.NoError(t, err)
	require.False(t, ok)
}

func TestExistsErrorOnNonDirParent(t *testing.T) {
	tmp := t.TempDir()
	// create a regular file and then treat it as a directory in the path.
	f := path.Join(tmp, "file")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))

	// stat-ing a path whose parent is a file yields ENOTDIR (not IsNotExist),
	// so Exists must surface the error and report exists==true.
	ok, err := Exists(path.Join(f, "child"))
	require.Error(t, err)
	require.True(t, ok)
}

func TestCreateSecureFolderCreatesAndReuses(t *testing.T) {
	tmp := t.TempDir()
	target := path.Join(tmp, "sub", "secure")

	got := CreateSecureFolder(target)
	require.Equal(t, target, got)

	info, err := os.Stat(target)
	require.NoError(t, err)
	require.True(t, info.IsDir())
	require.Equal(t, os.FileMode(defaultDirectoryPermission), info.Mode().Perm())

	// calling again on an existing folder returns the same path.
	again := CreateSecureFolder(target)
	require.Equal(t, target, again)
}

func TestCreateSecureFolderWrongPermissions(t *testing.T) {
	tmp := t.TempDir()
	target := path.Join(tmp, "weirdperm")
	require.NoError(t, os.Mkdir(target, 0o755))

	// Existing folder with non-default permissions: the function warns but
	// still returns the path.
	got := CreateSecureFolder(target)
	require.Equal(t, target, got)
}

func TestFilesAndFolders(t *testing.T) {
	tmp := t.TempDir()

	fileA := path.Join(tmp, "a.txt")
	require.NoError(t, os.WriteFile(fileA, []byte("a"), 0o600))
	require.NoError(t, os.Mkdir(path.Join(tmp, "dirA"), 0o755))

	files, err := Files(tmp)
	require.NoError(t, err)
	require.Equal(t, []string{fileA}, files)

	folders, err := Folders(tmp)
	require.NoError(t, err)
	require.Equal(t, []string{path.Join(tmp, "dirA")}, folders)
}

func TestFilesAndFoldersErrorOnMissingPath(t *testing.T) {
	missing := path.Join(t.TempDir(), "does-not-exist")

	_, err := Files(missing)
	require.Error(t, err)

	_, err = Folders(missing)
	require.Error(t, err)
}

func TestFileExists(t *testing.T) {
	tmp := t.TempDir()
	fileA := path.Join(tmp, "a.txt")
	require.NoError(t, os.WriteFile(fileA, []byte("a"), 0o600))

	// Note: FileExists compares against the full path returned by Files.
	require.True(t, FileExists(tmp, fileA))
	require.False(t, FileExists(tmp, path.Join(tmp, "missing.txt")))

	// missing directory -> Files errors -> false
	require.False(t, FileExists(path.Join(tmp, "nope"), "whatever"))
}

func TestFolderExists(t *testing.T) {
	tmp := t.TempDir()
	dir := path.Join(tmp, "dirA")
	require.NoError(t, os.Mkdir(dir, 0o755))

	require.True(t, FolderExists(tmp, dir))
	require.False(t, FolderExists(tmp, path.Join(tmp, "missing")))

	// missing directory -> Folders errors -> false
	require.False(t, FolderExists(path.Join(tmp, "nope"), "whatever"))
}

func TestCopyFile(t *testing.T) {
	tmp := t.TempDir()
	src := path.Join(tmp, "src.txt")
	dst := path.Join(tmp, "dst.txt")

	content := []byte("hello drand copy")
	require.NoError(t, os.WriteFile(src, content, 0o644))

	require.NoError(t, CopyFile(src, dst))

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, content, got)

	info, err := os.Stat(dst)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(rwFilePermission), info.Mode().Perm())
}

func TestCopyFileErrorMissingSource(t *testing.T) {
	tmp := t.TempDir()
	err := CopyFile(path.Join(tmp, "missing"), path.Join(tmp, "dst"))
	require.Error(t, err)
}

func TestCopyFileErrorBadDest(t *testing.T) {
	tmp := t.TempDir()
	src := path.Join(tmp, "src.txt")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o600))

	// destination directory doesn't exist -> os.Create fails.
	err := CopyFile(src, path.Join(tmp, "nodir", "dst.txt"))
	require.Error(t, err)
}

func TestCopyFolderWithNestedFiles(t *testing.T) {
	tmp := t.TempDir()
	srcRoot := path.Join(tmp, "src")
	srcSub := path.Join(srcRoot, "sub")
	require.NoError(t, os.MkdirAll(srcSub, 0o755))
	require.NoError(t, os.WriteFile(path.Join(srcRoot, "top.txt"), []byte("top"), 0o600))
	require.NoError(t, os.WriteFile(path.Join(srcSub, "nested.txt"), []byte("nested"), 0o600))

	dstRoot := CreateSecureFolder(path.Join(tmp, "dst"))
	require.NotEmpty(t, dstRoot)

	require.NoError(t, CopyFolder(srcRoot, dstRoot))

	top, err := os.ReadFile(path.Join(dstRoot, "top.txt"))
	require.NoError(t, err)
	require.Equal(t, []byte("top"), top)

	nested, err := os.ReadFile(path.Join(dstRoot, "sub", "nested.txt"))
	require.NoError(t, err)
	require.Equal(t, []byte("nested"), nested)
}

func TestCopyFolderErrorMissingSource(t *testing.T) {
	tmp := t.TempDir()
	err := CopyFolder(path.Join(tmp, "missing"), tmp)
	require.Error(t, err)
}

func TestTestWrite(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, TestWrite(tmp))

	// writing into a non-existent directory fails.
	require.Error(t, TestWrite(path.Join(tmp, "does-not-exist")))
}
