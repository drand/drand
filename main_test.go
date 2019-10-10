package main

import (
	"bytes"
	"fmt"
	gnet "net"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/dedis/drand/core"
	"github.com/dedis/drand/fs"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/test"
	"github.com/kabukky/httpscerts"
	"github.com/nikkolasg/slog"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/pairing/bn256"
	"go.dedis.ch/kyber/v3/share"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	cmd := exec.Command("go", "install")
	if err := cmd.Run(); err != nil {
		slog.Fatalf("test failing: %s", err)
	}
	code := m.Run()
	os.Exit(code)
}

func TestKeyGen(t *testing.T) {
	tmp := path.Join(os.TempDir(), "drand")
	defer os.RemoveAll(tmp)
	cmd := exec.Command("drand", "--folder", tmp, "generate-keypair", "127.0.0.1:8081")
	out, err := cmd.Output()
	require.Nil(t, err)
	fmt.Println(string(out))
	config := core.NewConfig(core.WithConfigFolder(tmp))
	fs := key.NewFileStore(config.ConfigFolder())
	priv, err := fs.LoadKeyPair()
	require.Nil(t, err)
	require.NotNil(t, priv.Public)

	tmp2 := path.Join(os.TempDir(), "drand2")
	defer os.RemoveAll(tmp2)
	cmd = exec.Command("drand", "--folder", tmp2, "generate-keypair")
	out, err = cmd.Output()
	require.Error(t, err)
	fmt.Println(string(out))
	config = core.NewConfig(core.WithConfigFolder(tmp2))
	fs = key.NewFileStore(config.ConfigFolder())
	priv, err = fs.LoadKeyPair()
	require.Error(t, err)
	require.Nil(t, priv)
}

//tests valid commands and then invalid commands
func TestGroup(t *testing.T) {
	n := 5
	tmpPath := path.Join(os.TempDir(), "drand")
	os.Mkdir(tmpPath, 0740)
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

	//test not enough keys
	cmd := exec.Command("drand", "--folder", tmpPath, "group", names[0])
	out, err := cmd.CombinedOutput()
	expectedOut := "group command take at least 3 keys as arguments"
	fmt.Println(string(out))
	require.Error(t, err)

	//test valid creation
	groupPath := path.Join(tmpPath, key.GroupFolderName)
	args := []string{"drand", "--folder", tmpPath, "group"}
	args = append(args, names...)
	cmd = exec.Command(args[0], args[1:]...)
	out, err = cmd.CombinedOutput()
	expectedOut = "Copy the following snippet into a new group.toml file " +
		"and distribute it to all the participants:"
	fmt.Println(string(out))
	require.True(t, strings.Contains(string(out), expectedOut))
	require.Nil(t, err)

	//recreates exactly like in main and saves the group
	var threshold = key.DefaultThreshold(n)
	publics := make([]*key.Identity, n)
	for i, str := range names {
		pub := &key.Identity{}
		if err := key.Load(str, pub); err != nil {
			slog.Fatal(err)
		}
		publics[i] = pub
	}
	group := key.NewGroup(publics, threshold)
	group.PublicKey = &key.DistPublic{
		Coefficients: []kyber.Point{publics[0].Key},
	}
	require.Nil(t, key.Save(groupPath, group, false))

	extraName := path.Join(tmpPath, fmt.Sprintf("drand-%d.public", n))
	extraPriv := key.NewKeyPair("127.0.0.1")
	require.NoError(t, key.Save(extraName, extraPriv.Public, false))
	if yes, err := fs.Exists(extraName); !yes || err != nil {
		t.Fatal(err.Error())
	}

	//test valid merge
	cmd = exec.Command("drand", "--folder", tmpPath, "group", "--group", groupPath, extraName)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))

	//expectedOut = "Copy the following snippet into a new_group.toml file and give it to the upgrade command to do the resharing."
	require.True(t, strings.Contains(string(out), expectedOut))

	//test could not load group file
	wrongGroupPath := "not_here"
	cmd = exec.Command("drand", "--folder", tmpPath, "group", "--group", wrongGroupPath, names[0])
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.Error(t, err)

	//test reject empty group file
	emptyGroupPath := path.Join(tmpPath, "empty.toml")
	emptyFile, err := os.Create(emptyGroupPath)
	if err != nil {
		slog.Fatal(err)
	}
	defer emptyFile.Close()
	cmd = exec.Command("drand", "--folder", tmpPath, "group", "--group", emptyGroupPath, names[0])
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.Error(t, err)
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

	cmd := exec.Command("drand", "--folder", tmpPath, "start", "--tls-disable")
	cmd.Env = append(os.Environ(), varEnv+"=1")
	err := cmd.Run()
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

func TestStartWithoutGroup(t *testing.T) {
	tmpPath := path.Join(os.TempDir(), "drand")
	os.Mkdir(tmpPath, 0740)
	defer func() {
		if err := os.RemoveAll(tmpPath); err != nil {
			fmt.Println(err)
		}
	}()

	pubPath := path.Join(tmpPath, "pub.key")
	addr := "127.0.0.1:8083"
	addr2 := "127.0.0.1:8084"
	ctrlPort := "8889"

	priv := key.NewKeyPair(addr)
	require.NoError(t, key.Save(pubPath, priv.Public, false))

	config := core.NewConfig(core.WithConfigFolder(tmpPath))
	fs := key.NewFileStore(config.ConfigFolder())
	fs.SaveKeyPair(priv)

	installCmd := exec.Command("go", "install")
	_, err := installCmd.Output()
	require.NoError(t, err)

	os.Args = []string{"drand", "--verbose", "2", "--folder", tmpPath, "start", "--tls-disable"}
	go main()

	initDKGCmd := exec.Command("drand", "share")
	out, err := initDKGCmd.Output()
	expectedErr := "needs at least one group.toml file argument"
	output := string(out)
	require.Error(t, err)
	require.True(t, strings.Contains(output, expectedErr))

	// fake group
	_, group := test.BatchIdentities(5)
	priv.Public.TLS = false
	group.Nodes[0] = priv.Public
	groupPath := path.Join(tmpPath, fmt.Sprintf("groups/drand_group.toml"))
	require.NoError(t, key.Save(groupPath, group, false))

	//fake share
	pairing := bn256.NewSuite()
	scalarOne := pairing.G2().Scalar().One()
	s := &share.PriShare{I: 2, V: scalarOne}
	share := &key.Share{Share: s}
	fs.SaveShare(share)

	// fake dkg outuput
	keyStr := "0776a00e44dfa3ab8cff6b78b430bf16b9f8d088b54c660722a35f5034abf3ea4deb1a81f6b9241d22185ba07c37f71a67f94070a71493d10cb0c7e929808bd10cf2d72aeb7f4e10a8b0e6ccc27dad489c9a65097d342f01831ed3a9d0a875b770452b9458ec3bca06a5d4b99a5ac7f41ee5a8add2020291eab92b4c7f2d449f"
	fakeKey, _ := key.StringToPoint(key.G2, keyStr)
	distKey := &key.DistPublic{
		Coefficients: []kyber.Point{fakeKey},
	}
	require.NoError(t, fs.SaveDistPublic(distKey))

	// Specify different control and listen ports than TLS example so the two
	// concurrently running drand instances (one secure, one insecure) don't
	// re-use ports.
	os.Args = []string{"drand", "--folder", tmpPath, "start", "--listen", addr2, "--control", ctrlPort, "--tls-disable"}
	go main()

	cmd := exec.Command("drand", "ping", "--control", ctrlPort)
	out, err = cmd.CombinedOutput()
	require.NoError(t, err)

	require.NoError(t, toml.NewEncoder(os.Stdout).Encode(group))

	cmd = exec.Command("drand", "--verbose", "2", "get", "private", "--tls-disable", groupPath)
	out, err = cmd.CombinedOutput()
	require.NoError(t, err)

	cmd = exec.Command("drand", "get", "cokey", "--tls-disable", groupPath)
	out, err = cmd.CombinedOutput()
	require.True(t, strings.Contains(string(out), keyStr))
	require.NoError(t, err)

	cmd = exec.Command("drand", "show", "share", "--control", ctrlPort)
	out, err = cmd.CombinedOutput()
	if err != nil {
		fmt.Println(string(out))
		t.Fatalf("could not run the command : %s", err.Error())
	}
	expectedOutput := "0000000000000000000000000000000000000000000000000000000000000001"
	require.True(t, strings.Contains(string(out), expectedOutput))
	require.NoError(t, err)

	// reset state
	cmd = exec.Command("drand", "--folder", tmpPath, "reset")
	var in bytes.Buffer
	in.WriteString("y\n")
	cmd.Stdin = &in
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(t, err)
	_, err = fs.LoadDistPublic()
	require.Error(t, err)
	_, err = fs.LoadShare()
	require.Error(t, err)
	_, err = fs.LoadGroup()
	require.Error(t, err)

}

func TestClientTLS(t *testing.T) {
	tmpPath := path.Join(os.TempDir(), "drand")
	os.Mkdir(tmpPath, 0740)
	defer os.RemoveAll(tmpPath)

	groupPath := path.Join(tmpPath, "group.toml")
	pubPath := path.Join(tmpPath, "pub.key")
	certPath := path.Join(tmpPath, "server.pem")
	keyPath := path.Join(tmpPath, "key.pem")

	addr := "127.0.0.1:8085"
	ctrlPort := "9091"

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
	group.Nodes[0] = priv.Public
	group.Period = 2 * time.Minute
	groupPath = path.Join(tmpPath, fmt.Sprintf("groups/drand_group.toml"))
	fs.SaveGroup(group)

	// fake dkg outuput
	keyStr := "0776a00e44dfa3ab8cff6b78b430bf16b9f8d088b54c660722a35f5034abf3ea4deb1a81f6b9241d22185ba07c37f71a67f94070a71493d10cb0c7e929808bd10cf2d72aeb7f4e10a8b0e6ccc27dad489c9a65097d342f01831ed3a9d0a875b770452b9458ec3bca06a5d4b99a5ac7f41ee5a8add2020291eab92b4c7f2d449f"

	fakeKey, _ := test.StringToPoint(keyStr)
	distKey := &key.DistPublic{
		Coefficients: []kyber.Point{fakeKey},
	}
	require.NoError(t, fs.SaveDistPublic(distKey))

	//fake share
	pairing := bn256.NewSuite()
	scalarOne := pairing.G2().Scalar().One()
	s := &share.PriShare{I: 2, V: scalarOne}
	share := &key.Share{Share: s}
	fs.SaveShare(share)

	os.Args = []string{"drand", "--folder", tmpPath, "start", "--tls-cert", certPath, "--tls-key", keyPath, "--control", ctrlPort}
	go main()

	installCmd := exec.Command("go", "install")
	_, err := installCmd.Output()
	require.NoError(t, err)

	cmd := exec.Command("drand", "get", "private", "--tls-cert", certPath, groupPath)
	out, err := cmd.CombinedOutput()
	fmt.Println("get private = ", string(out))
	require.NoError(t, err)

	// XXX Commented out test since we can't "fake" anymore in the same way
	// a dist public key. One would need to use the real fs path of the daemon
	// to save the group at the right place
	//
	/*cmd = exec.Command("drand", "fetch", "dist_key", "--tls-cert", certPath, addr)*/
	//out, err = cmd.CombinedOutput()
	//require.True(t, strings.Contains(string(out), keyStr))
	//require.NoError(t, err)

	//cmd = exec.Command("drand", "control", "share")
	//out, err = cmd.CombinedOutput()
	//if err != nil {
	//t.Fatalf("could not run the command : %s", err.Error())
	//}
	//expectedOutput := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAE="
	//require.True(t, strings.Contains(string(out), expectedOutput))
	/*require.NoError(t, err)*/

	// XXX Can't test public randomness without launching an actual DKG / beacon
	// process...
	/*cmd = exec.Command("drand", "get", "public", "--tls-cert", certPath, "--nodes", addr, groupPath)*/
	//out, err = cmd.CombinedOutput()
	//fmt.Println(string(out))
	//require.NoError(t, err)

	cmd = exec.Command("drand", "get", "cokey", "--tls-cert", certPath, "--nodes", addr, groupPath)
	out, err = cmd.CombinedOutput()
	//fmt.Println(string(out))

	expectedOutput := keyStr
	//fmt.Printf("out = %s\n", string(out))
	//fmt.Printf("expected = %s\n", expectedOutput)
	//fmt.Printf("contains ? %v\n", strings.Contains(string(out), expectedOutput))
	require.Contains(t, string(out), expectedOutput)
	require.NoError(t, err)

	cmd = exec.Command("drand", "--verbose", "2", "show", "share", "--control", ctrlPort)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	expectedOutput = "0000000000000000000000000000000000000000000000000000000000000001"

	require.True(t, strings.Contains(string(out), expectedOutput))
	require.NoError(t, err)

	cmd = exec.Command("drand", "show", "public", "--control", ctrlPort)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(t, err)

	cmd = exec.Command("drand", "show", "private", "--control", ctrlPort)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(t, err)

	cmd = exec.Command("drand", "show", "cokey", "--control", ctrlPort)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	expectedOutput = keyStr
	require.True(t, strings.Contains(string(out), expectedOutput))
	require.NoError(t, err)
}
