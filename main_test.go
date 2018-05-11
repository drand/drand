package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"testing"

	"github.com/dedis/drand/core"
	"github.com/dedis/drand/key"
	"github.com/stretchr/testify/require"
)

func TestKeyGen(t *testing.T) {
	tmp := path.Join(os.TempDir(), "drand")
	defer os.RemoveAll(tmp)
	// valid address
	os.Args = []string{"drand", "--config", tmp, "keygen", "127.0.0.1:8081"}
	main()
	config := core.NewConfig(core.WithConfigFolder(tmp))
	fs := key.NewFileStore(config.ConfigFolder())

	priv, err := fs.LoadPrivate()
	require.Nil(t, err)
	require.NotNil(t, priv.Public)
}

// https://stackoverflow.com/questions/26225513/how-to-test-os-exit-scenarios-in-go
func TestKeyGenInvalid(t *testing.T) {
	tmp := path.Join(os.TempDir(), "drand")
	varEnv := "CRASHCRASH"
	if os.Getenv(varEnv) == "1" {
		os.Args = []string{"drand", "--config", tmp, "keygen"}
		fmt.Println("bri")
		main()
		return
	}

	defer os.Remove(tmp)
	cmd := exec.Command(os.Args[0], "-test.run=TestKeyGenInvalid")
	cmd.Env = append(os.Environ(), varEnv+"=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && e.Success() {
		t.Fatalf("KeyGenInvalid should have failed")
	}

	config := core.NewConfig(core.WithConfigFolder(tmp))
	fs := key.NewFileStore(config.ConfigFolder())
	priv, err := fs.LoadPrivate()
	//fmt.Println(priv.Public.Addr)
	require.Error(t, err)
	require.Nil(t, priv)
}

func TestGroupGen(t *testing.T) {
	n := 5
	thr := 4
	tmpPath := path.Join(os.TempDir(), "drand")
	os.Mkdir(tmpPath, 0777)
	defer os.RemoveAll(tmpPath)

	names := make([]string, n, n)
	privs := make([]*key.Private, n, n)
	for i := 0; i < n; i++ {
		names[i] = path.Join(tmpPath, fmt.Sprintf("drand-%d.private", i))
		privs[i] = key.NewKeyPair("127.0.0.1")
		require.Nil(t, key.Save(names[i], privs[i].Public, false))
	}
	os.Args = []string{"drand", "group", "--threshold", strconv.Itoa(thr)}
	os.Args = append(os.Args, names...)
	main()

	group := new(key.Group)
	require.NoError(t, key.Load(gname, group))
	require.Equal(t, thr, group.Threshold)
	for i := 0; i < n; i++ {
		require.True(t, group.Contains(privs[i].Public))
	}
}
