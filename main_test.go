package main

import (
	"os"
	"os/exec"
	"path"
	"testing"

	"github.com/dedis/drand/core"
	"github.com/dedis/drand/key"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	cmd := exec.Command("go", "install")
	cmd.Run()
	code := m.Run()
	os.Exit(code)
}

func TestKeyGen(t *testing.T) {
	tmp := path.Join(os.TempDir(), "drand")
	defer os.RemoveAll(tmp)
	os.Args = []string{"drand", "--folder", tmp, "generate-keypair", "127.0.0.1:8081"}
	main()
	config := core.NewConfig(core.WithConfigFolder(tmp))
	fs := key.NewFileStore(config.ConfigFolder())
	priv, err := fs.LoadKeyPair()
	require.Nil(t, err)
	require.NotNil(t, priv.Public)
}
