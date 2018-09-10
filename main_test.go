package main

import (
	"fmt"
	gnet "net"
	"os"
	"os/exec"
	"path"
	"testing"

	"github.com/dedis/drand/core"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/test"
	"github.com/kabukky/httpscerts"

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

func TestStartAndStop(t *testing.T) {
	tmpPath := path.Join(os.TempDir(), "drand")
	os.Mkdir(tmpPath, 0740)
	defer os.RemoveAll(tmpPath)
	varEnv := "CRASHCRASH"
	n := 5
	_, group := test.BatchIdentities(n)
	groupPath := path.Join(tmpPath, fmt.Sprintf("group.toml"))
	require.NoError(t, key.Save(groupPath, group, false))

	cmd := exec.Command("drand", "--folder", tmpPath, "start", groupPath, "--tls-disable")
	cmd.Env = append(os.Environ(), varEnv+"=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && e.Success() {
		t.Fatal(err)
	}
	cmd = exec.Command("drand", "-c", tmpPath, "stop")
	cmd.Env = append(os.Environ(), varEnv+"=1")
	err = cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && e.Success() {
		t.Fatal(err)
	}
}

func TestStartBeacon(t *testing.T) {
	tmpPath := path.Join(os.TempDir(), "drand")
	os.Mkdir(tmpPath, 0740)
	defer os.RemoveAll(tmpPath)
	varEnv := "CRASHCRASH"
	n := 5
	_, group := test.BatchIdentities(n)
	groupPath := path.Join(tmpPath, fmt.Sprintf("group.toml"))
	require.NoError(t, key.Save(groupPath, group, false))

	cmd := exec.Command("drand", "--folder", tmpPath, "start", "--tls-disable")
	cmd.Env = append(os.Environ(), varEnv+"=1")
	out, err := cmd.Output()
	fmt.Print(string(out))
	if e, ok := err.(*exec.ExitError); ok && e.Success() {
		t.Fatal(err)
	}
}

func TestClientTLS(t *testing.T) {
	tmpPath := path.Join(os.TempDir(), "drand")
	os.Mkdir(tmpPath, 0740)
	defer os.RemoveAll(tmpPath)

	pubPath := path.Join(tmpPath, "pub.key")
	certPath := path.Join(tmpPath, "server.pem")
	keyPath := path.Join(tmpPath, "key.pem")

	addr := "127.0.0.1:8082"

	priv := key.NewTLSKeyPair(addr)
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
	groupPath := path.Join(tmpPath, fmt.Sprintf("groups/drand_group.toml"))
	require.NoError(t, key.Save(groupPath, group, false))

	os.Args = []string{"drand", "--folder", tmpPath, "start", "--tls-cert", certPath, "--tls-key", keyPath, groupPath}
	go main()

	installCmd := exec.Command("go", "install")
	_, err := installCmd.Output()
	require.NoError(t, err)

	cmd := exec.Command("drand", "get", "private", "--tls-cert", certPath, "--nodes", addr, groupPath)
	out, err := cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(t, err)
}
