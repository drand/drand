package main

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeyGen(t *testing.T) {

	os.Args = []string{"drand", "keygen"}
	main()
	fs := DefaultFileStore()
	defer os.RemoveAll(fs.KeyFile)
	defer os.RemoveAll(fs.PublicFile)
	// no address
	_, err := fs.LoadKey()
	require.Error(t, err)

	// valid address
	os.Args = []string{"drand", "keygen", "127.0.0.1:8080"}
	main()
	priv, err := fs.LoadKey()
	require.Nil(t, err)
	require.NotNil(t, priv.Public)

	// custom file
	tmp := os.TempDir()
	fullPath := path.Join(tmp, defaultKeyFile+privateExtension)
	fs.KeyFile = fullPath
	fs.PublicFile = publicFile(fullPath)
	defer os.RemoveAll(fs.KeyFile)
	defer os.RemoveAll(fs.PublicFile)
	os.Args = []string{"drand", "keygen", "--" + keyFileFlagName, fullPath, "127.0.0.1:8080"}
	main()
	priv, err = fs.LoadKey()
	require.Nil(t, err)
	require.NotNil(t, priv.Public)
}
