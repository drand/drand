package fs

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecureDirAlreadyHere(t *testing.T) {
	tmpPath := path.Join(os.TempDir(), "config")
	os.Mkdir(tmpPath, 0777)
	defer os.RemoveAll(tmpPath)
	path := CreateSecureFolder(tmpPath)
	require.NotNil(t, path)
}

func TestSecureDirAlreadyHereWrongPerm(t *testing.T) {
	tmpPath := path.Join(os.TempDir(), "config")
	os.Mkdir(tmpPath, 0700)
	defer os.RemoveAll(tmpPath)
	path := CreateSecureFolder(tmpPath)
	require.Equal(t, "", path)
}
