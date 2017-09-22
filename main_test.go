package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeyGen(t *testing.T) {

	fs := DefaultFileStore()
	defer os.RemoveAll(fs.KeyFile)
	defer os.RemoveAll(fs.PublicFile)

	// valid address
	os.Args = []string{"drand", "keygen", "127.0.0.1:8080"}
	main()
	priv, err := fs.LoadKey()
	require.Nil(t, err)
	require.NotNil(t, priv.Public)

	// custom file
	tmp := path.Join(os.TempDir(), "drand")
	os.MkdirAll(tmp, 0777)
	fs.KeyFile = path.Join(tmp, defaultKeyFile+privateExtension)
	fs.PublicFile = publicFile(fs.KeyFile)
	//defer os.RemoveAll(fs.KeyFile)
	//defer os.RemoveAll(fs.PublicFile)
	os.Args = []string{"drand", "keygen", "--" + keyFolderFlagName, tmp, "127.0.0.1:8080"}
	main()
	priv, err = fs.LoadKey()
	require.Nil(t, err)
	require.NotNil(t, priv.Public)
}

// https://stackoverflow.com/questions/26225513/how-to-test-os-exit-scenarios-in-go
func TestKeyGenInvalid(t *testing.T) {
	varEnv := "CRASHCRASH"
	if os.Getenv(varEnv) == "1" {
		os.Args = []string{"drand", "keygen"}
		fmt.Println("bri")
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestKeyGenInvalid")
	cmd.Env = append(os.Environ(), varEnv+"=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && e.Success() {
		t.Fatalf("KeyGenInvalid should have failed")
	}
	fs := DefaultFileStore()
	defer os.RemoveAll(fs.KeyFile)
	defer os.RemoveAll(fs.PublicFile)
	// no address
	_, err = fs.LoadKey()
	require.Error(t, err)

}
