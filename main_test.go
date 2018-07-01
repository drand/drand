package main

import (
	"fmt"
	gnet "net"
	"os"
	"os/exec"
	"path"
	"strconv"
	"testing"

	"github.com/dedis/drand/core"
	"github.com/dedis/drand/fs"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/test"
	"github.com/kabukky/httpscerts"
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

	priv, err := fs.LoadKeyPair()
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
	priv, err := fs.LoadKeyPair()
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
	privs := make([]*key.Pair, n, n)
	for i := 0; i < n; i++ {
		names[i] = path.Join(tmpPath, fmt.Sprintf("drand-%d.public", i))
		privs[i] = key.NewKeyPair("127.0.0.1")
		require.NoError(t, key.Save(names[i], privs[i].Public, false))
		if yes, err := fs.Exists(names[i]); !yes || err != nil {
			t.Fatal(err.Error())
		}
	}
	groupPath := path.Join(tmpPath, gname)
	os.Args = []string{"drand", "group", "--threshold", strconv.Itoa(thr), "--out", groupPath}
	os.Args = append(os.Args, names...)
	main()

	group := new(key.Group)
	require.NoError(t, key.Load(groupPath, group))
	require.Equal(t, thr, group.Threshold)
	for i := 0; i < n; i++ {
		require.True(t, group.Contains(privs[i].Public))
	}
}

func TestClientTLS(t *testing.T) {
	tmpPath := path.Join(os.TempDir(), "drand")
	os.Mkdir(tmpPath, 0777)
	defer os.RemoveAll(tmpPath)

	pubPath := path.Join(tmpPath, "pub.key")
	certPath := path.Join(tmpPath, "server.pem")
	keyPath := path.Join(tmpPath, "key.pem")

	priv := key.NewTLSKeyPair("127.0.0.1:8080")
	require.NoError(t, key.Save(pubPath, priv.Public, false))

	config := core.NewConfig(core.WithConfigFolder(tmpPath))
	fs := key.NewFileStore(config.ConfigFolder())
	fs.SaveKeyPair(priv)

	if httpscerts.Check(certPath, keyPath) != nil {
		fmt.Println("generating on the fly")
		h, _, _ := gnet.SplitHostPort(priv.Public.Address())
		if err := httpscerts.Generate(certPath, keyPath, h); err != nil {
			panic(err)
		}
	}

	// fake group
	_, group := test.BatchTLSIdentities(5)
	group.Nodes[0] = &key.IndexedPublic{
		Identity: priv.Public,
		Index:    0,
	}
	groupPath := path.Join(tmpPath, fmt.Sprintf("group.toml"))
	require.NoError(t, key.Save(groupPath, group, false))

	os.Args = []string{"drand", "--config", tmpPath, "run", "--tls-cert", certPath, "--tls-key", keyPath, groupPath}
	go main()

	installCmd := exec.Command("go", "install")
	_, err := installCmd.Output()
	require.NoError(t, err)

	cmd := exec.Command("drand", "fetch", "private", "--tls-cert", certPath, pubPath)
	out, err := cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(t, err)
}
