package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	gnet "net"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/drand/drand/beacon"
	"github.com/drand/drand/core"
	"github.com/drand/drand/fs"
	"github.com/drand/drand/key"
	"github.com/drand/drand/test"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/util/random"
	"github.com/kabukky/httpscerts"
	"github.com/nikkolasg/slog"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	cmd := exec.Command("go", "build")
	if err := cmd.Run(); err != nil {
		slog.Fatalf("test failing: %s", err)
	}
	code := m.Run()
	os.Exit(code)
}

func TestDeleteBeacon(t *testing.T) {
	tmp := path.Join(os.TempDir(), "drand")
	defer os.RemoveAll(tmp)

	var opt = core.WithConfigFolder(tmp)
	conf := core.NewConfig(opt)
	fs.CreateSecureFolder(conf.DBFolder())
	store, err := beacon.NewBoltStore(conf.DBFolder(), conf.BoltOptions())
	require.NoError(t, err)
	store.Put(&beacon.Beacon{
		Round:     1,
		Signature: []byte("Hello"),
	})
	store.Put(&beacon.Beacon{
		Round:     2,
		Signature: []byte("Hello"),
	})
	store.Put(&beacon.Beacon{
		Round:     3,
		Signature: []byte("Hello"),
	})
	store.Put(&beacon.Beacon{
		Round:     4,
		Signature: []byte("hello"),
	})
	// try to fetch round 3 and 4
	b, err := store.Get(3)
	require.NoError(t, err)
	require.NotNil(t, b)
	b, err = store.Get(4)
	require.NoError(t, err)
	require.NotNil(t, b)

	store.Close()
	// that commmand should delete round 3 and 4
	cmd := exec.Command("./drand", "util", "del-beacon", "--folder", tmp, "3")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	store, err = beacon.NewBoltStore(conf.DBFolder(), conf.BoltOptions())
	require.NoError(t, err)

	// try to fetch round 3 and 4 - it should now fail
	b, err = store.Get(3)
	require.Error(t, err)
	require.Nil(t, b)
	b, err = store.Get(4)
	require.Error(t, err)
	require.Nil(t, b)

}

func TestKeyGen(t *testing.T) {
	tmp := path.Join(os.TempDir(), "drand")
	defer os.RemoveAll(tmp)
	cmd := exec.Command("./drand", "generate-keypair", "--folder", tmp, "127.0.0.1:8081")
	out, err := cmd.Output()
	require.Nil(t, err)
	fmt.Println(string(out))
	config := core.NewConfig(core.WithConfigFolder(tmp))
	fs := key.NewFileStore(config.ConfigFolder())
	priv, err := fs.LoadKeyPair()
	require.NoError(t, err)
	require.NotNil(t, priv.Public)

	tmp2 := path.Join(os.TempDir(), "drand2")
	defer os.RemoveAll(tmp2)
	cmd = exec.Command("./drand", "generate-keypair", "--folder", tmp2)
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
func TestStartAndStop(t *testing.T) {
	tmpPath := path.Join(os.TempDir(), "drand")
	os.Mkdir(tmpPath, 0740)
	defer os.RemoveAll(tmpPath)
	n := 5
	_, group := test.BatchIdentities(n)
	groupPath := path.Join(tmpPath, fmt.Sprintf("group.toml"))
	require.NoError(t, key.Save(groupPath, group, false))

	cmd := exec.Command("./drand", "generate-keypair", "127.0.0.1:8080", "--tls-disable", "--folder", tmpPath)
	require.NoError(t, cmd.Run())
	startCh := make(chan bool)
	go func() {
		cmd = exec.Command("./drand", "start", "--tls-disable", "--folder", tmpPath)
		startCh <- true
		cmd.Run()
		startCh <- true
		// TODO : figuring out how to not panic in grpc call
		// ERROR: 2020/01/23 21:06:28 grpc: server failed to encode response:
		// rpc error: code = Internal desc = grpc: error while marshaling: proto:
		// Marshal called with nil

		//if e, ok := err.(*exec.ExitError); !ok || e.ExitCode() != 0 {
		//t.Fatal(err)
		//}
	}()
	<-startCh
	time.Sleep(50 * time.Millisecond)
	stopCmd := exec.Command("./drand", "stop")
	buff, err := stopCmd.CombinedOutput()
	require.NoError(t, err, string(buff))
	select {
	case <-startCh:
	case <-time.After(1 * time.Second):
		t.Fatal("drand daemon did not stop")
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

	cmd := exec.Command("./drand", "start", "--tls-disable", "--folder", tmpPath)
	cmd.Env = append(os.Environ(), varEnv+"=1")
	out, err := cmd.Output()
	fmt.Print(string(out))
	if e, ok := err.(*exec.ExitError); ok && e.Success() {
		t.Fatal(err)
	}
}

func TestStartWithoutGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tmpPath := path.Join(os.TempDir(), "drand")
	os.Mkdir(tmpPath, 0740)
	defer func() {
		if err := os.RemoveAll(tmpPath); err != nil {
			fmt.Println(err)
		}
	}()

	pubPath := path.Join(tmpPath, "pub.key")
	port1, _ := strconv.Atoi(test.FreePort())
	addr := "127.0.0.1:" + strconv.Itoa(port1)
	ctrlPort1 := test.FreePort()
	ctrlPort2 := test.FreePort()
	metricsPort := test.FreePort()

	priv := key.NewKeyPair(addr)
	require.NoError(t, key.Save(pubPath, priv.Public, false))

	config := core.NewConfig(core.WithConfigFolder(tmpPath))
	fs := key.NewFileStore(config.ConfigFolder())
	require.NoError(t, fs.SaveKeyPair(priv))

	installCmd := exec.Command("go", "build")
	_, err := installCmd.Output()
	require.NoError(t, err)

	lctx, lcancel := context.WithCancel(context.Background())
	start1 := exec.CommandContext(lctx, "./drand", "start", "--tls-disable", "--verbose", "2", "--folder", tmpPath, "--control", ctrlPort1, "--metrics", metricsPort)
	go start1.Run()

	fmt.Println(" DRAND SHARE ---")
	initDKGCmd := exec.Command("./drand", "share", "--control", ctrlPort1)
	out, err := initDKGCmd.Output()
	require.Error(t, err)
	lcancel()

	fmt.Println(" --- DRAND GROUP ---")
	// fake group
	_, group := test.BatchIdentities(5)
	priv.Public.TLS = false
	group.Period = 5 * time.Second
	group.GenesisTime = time.Now().Unix() - 10
	group.Nodes[0] = &key.Node{Identity: priv.Public, Index: 0}
	group.Nodes[1] = &key.Node{Identity: priv.Public, Index: 1}
	groupPath := path.Join(tmpPath, "drand_group.toml")
	require.NoError(t, key.Save(groupPath, group, false))
	// save it also to somewhere drand will find it
	require.NoError(t, fs.SaveGroup(group))

	//fake share
	scalarOne := key.KeyGroup.Scalar().One()
	s := &share.PriShare{I: 2, V: scalarOne}
	share := &key.Share{Share: s}
	require.NoError(t, fs.SaveShare(share))

	// fake dkg outuput
	fakeKey := key.KeyGroup.Point().Pick(random.New())
	distKey := &key.DistPublic{
		Coefficients: []kyber.Point{fakeKey},
	}
	require.NoError(t, fs.SaveDistPublic(distKey))

	fmt.Println(" --- DRAND START --- control ", ctrlPort1)

	start2 := exec.CommandContext(ctx, "./drand", "start", "--control", ctrlPort2, "--tls-disable", "--folder", tmpPath, "--verbose", "--private-rand")
	start2.Stdout = os.Stdout
	start2.Stderr = os.Stderr
	go start2.Run()
	defer exec.Command("./drand", "stop", "--control", ctrlPort2).Run()
	time.Sleep(500 * time.Millisecond)

	fmt.Println(" + running PING command with ", ctrlPort2)
	ping := exec.Command("./drand", "util", "ping", "--control", ctrlPort2)
	out, err = ping.CombinedOutput()
	require.NoError(t, err, string(out))

	require.NoError(t, toml.NewEncoder(os.Stdout).Encode(group))

	fmt.Printf("\n Running GET PRIVATE command with group file at %s\n", groupPath)
	loadedGroup := new(key.Group)
	require.NoError(t, key.Load(groupPath, loadedGroup))
	fmt.Printf("%s", loadedGroup.String())

	getCmd := exec.Command("./drand", "get", "private", "--tls-disable", groupPath)
	out, err = getCmd.CombinedOutput()
	require.NoError(t, err, string(out))

	fakeStr := key.PointToString(fakeKey)
	cokeyCmd := exec.Command("./drand", "get", "cokey", "--tls-disable", priv.Public.Address())
	out, err = cokeyCmd.CombinedOutput()
	require.NoError(t, err, string(out))
	require.True(t, strings.Contains(string(out), fakeStr))

	shareCmd := exec.Command("./drand", "show", "share", "--control", ctrlPort2)
	out, err = shareCmd.CombinedOutput()
	if err != nil {
		fmt.Println(string(out))
		t.Fatalf("could not run the command : %s", err.Error())
	}
	expectedOutput := "0000000000000000000000000000000000000000000000000000000000000001"
	require.True(t, strings.Contains(string(out), expectedOutput))
	require.NoError(t, err)

	// reset state
	resetCmd := exec.Command("./drand", "util", "reset", "--folder", tmpPath)
	var in bytes.Buffer
	in.WriteString("y\n")
	resetCmd.Stdin = &in
	out, err = resetCmd.CombinedOutput()
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

	//ctx, cancel := context.WithCancel(context.Background())
	//defer cancel()

	groupPath := path.Join(tmpPath, "group.toml")
	pubPath := path.Join(tmpPath, "pub.key")
	certPath := path.Join(tmpPath, "server.pem")
	keyPath := path.Join(tmpPath, "key.pem")

	freePort := test.FreePort()
	addr := "127.0.0.1:" + freePort
	ctrlPort := test.FreePort()
	metricsPort := test.FreePort()

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
	group.Nodes[0] = &key.Node{Identity: priv.Public, Index: 0}
	group.Period = 2 * time.Minute
	groupPath = path.Join(tmpPath, fmt.Sprintf("groups/drand_group.toml"))
	fs.SaveGroup(group)

	// fake dkg outuput
	fakeKey := key.KeyGroup.Point().Pick(random.New())
	keyStr := key.PointToString(fakeKey)

	distKey := &key.DistPublic{
		Coefficients: []kyber.Point{fakeKey},
	}
	require.NoError(t, fs.SaveDistPublic(distKey))

	//fake share
	scalarOne := key.KeyGroup.Scalar().One()
	s := &share.PriShare{I: 2, V: scalarOne}
	share := &key.Share{Share: s}
	fs.SaveShare(share)

	startArgs := []string{"./drand", "start", "--tls-cert", certPath, "--tls-key", keyPath, "--control", ctrlPort, "--folder", tmpPath, "--metrics", metricsPort, "--private-rand"}
	os.Args = startArgs
	//startCmd := exec.CommandContext(ctx, startArgs[0], startArgs[1:]...)
	//startCmd.Stdout = os.Stdout
	//startCmd.Stderr = os.Stderr
	//go startCmd.Run()
	go main()

	installCmd := exec.Command("go", "build")
	_, err := installCmd.Output()
	require.NoError(t, err)

	cmd := exec.Command("./drand", "get", "private", "--tls-cert", certPath, groupPath)
	out, err := cmd.CombinedOutput()
	fmt.Println("get private = ", string(out))
	require.NoError(t, err, string(out))

	cmd = exec.Command("./drand", "get", "cokey", "--tls-cert", certPath, addr)
	out, err = cmd.CombinedOutput()
	//fmt.Println(string(out))

	expectedOutput := keyStr
	//fmt.Printf("out = %s\n", string(out))
	//fmt.Printf("expected = %s\n", expectedOutput)
	//fmt.Printf("contains ? %v\n", strings.Contains(string(out), expectedOutput))
	require.Contains(t, string(out), expectedOutput)
	require.NoError(t, err)

	cmd = exec.Command("./drand", "show", "share", "--control", ctrlPort)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	expectedOutput = "0000000000000000000000000000000000000000000000000000000000000001"

	require.True(t, strings.Contains(string(out), expectedOutput))
	require.NoError(t, err)

	cmd = exec.Command("./drand", "show", "public", "--control", ctrlPort)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(t, err)

	cmd = exec.Command("./drand", "show", "private", "--control", ctrlPort)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(t, err)

	cmd = exec.Command("./drand", "show", "cokey", "--control", ctrlPort)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	expectedOutput = keyStr
	require.True(t, strings.Contains(string(out), expectedOutput))
	require.NoError(t, err)

	cmd = exec.Command("./drand", "show", "group", "--control", ctrlPort)
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	pubBuff, _ := priv.Public.Key.MarshalBinary()
	pubStr := hex.EncodeToString(pubBuff)
	require.True(t, strings.Contains(string(out), pubStr), "key: %s, group: %s", pubStr, string(out))

	cmd = exec.Command("./drand", "show", "group", "--control", ctrlPort, "--hash-only")
	out, err = cmd.CombinedOutput()
	require.NoError(t, err)
	groupHash := hex.EncodeToString(group.Hash())
	require.Equal(t, strings.Trim(string(out), "\n"), groupHash, string(out))

}
