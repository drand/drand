package main

import (
	"bytes"
	"context"
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
	cmd := exec.Command("drand", "generate-keypair", "--folder", tmp, "127.0.0.1:8081")
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
	cmd = exec.Command("drand", "generate-keypair", "--folder", tmp2)
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
	cmd := exec.Command("drand", "group", "--folder", tmpPath, names[0])
	out, err := cmd.CombinedOutput()
	expectedOut := "group command take at least 3 keys as arguments"
	fmt.Println(string(out))
	require.True(t, strings.Contains(string(out), expectedOut))
	require.Error(t, err)

	// test invalid genesis time
	invalidGenesis := strconv.Itoa(int(time.Now().Unix() - 10))
	args := []string{"drand", "group", "--genesis", invalidGenesis, "--folder", tmpPath}
	args = append(args, names...)
	cmd = exec.Command(args[0], args[1:]...)
	_, err = cmd.CombinedOutput()
	require.Error(t, err)

	//test valid creation
	groupPath := path.Join(tmpPath, "group1.toml")
	genesis := int(time.Now().Unix() + 10)
	args = []string{"drand", "group", "--genesis", strconv.Itoa(genesis), "--folder", tmpPath, "--out", groupPath}
	args = append(args, names...)
	cmd = exec.Command(args[0], args[1:]...)
	out, err = cmd.CombinedOutput()
	// check it read all names given
	for _, name := range names {
		require.True(t, strings.Contains(string(out), name), string(out))
	}
	require.NoError(t, err, string(out))

	loadedGroup := new(key.Group)
	require.NoError(t, key.Load(groupPath, loadedGroup))
	// expect a genesis seed
	require.True(t, len(loadedGroup.GenesisSeed) != 0)
	require.Equal(t, int64(genesis), loadedGroup.GenesisTime)
	for _, priv := range privs {
		_, found := loadedGroup.Index(priv.Public)
		require.True(t, found)
	}

	// test valid creation with `start-in`
	startIn := "1m"
	startInD, err := time.ParseDuration(startIn)
	require.NoError(t, err)
	expGenesis := time.Now().Add(startInD).Unix()
	args = []string{"drand", "group", "--start-in", startIn, "--folder", tmpPath, "--out", groupPath}
	args = append(args, names...)
	cmd = exec.Command(args[0], args[1:]...)
	out, err = cmd.CombinedOutput()
	// check it read all names given
	for _, name := range names {
		require.True(t, strings.Contains(string(out), name), string(out))
	}
	require.NoError(t, err, string(out))

	loadedGroup = new(key.Group)
	require.NoError(t, key.Load(groupPath, loadedGroup))
	// expect a genesis seed
	require.True(t, len(loadedGroup.GenesisSeed) != 0)
	require.True(t, loadedGroup.GenesisTime >= expGenesis)
	for _, priv := range privs {
		_, found := loadedGroup.Index(priv.Public)
		require.True(t, found)
	}

	// test resharing from the previous group
	// create new keys
	newNames := make([]string, n, n)
	newPrivs := make([]*key.Pair, n, n)
	for i := 0; i < n; i++ {
		newNames[i] = path.Join(tmpPath, fmt.Sprintf("drand-%d.public", n+i))
		newPrivs[i] = key.NewKeyPair("127.0.0.1:443")
		require.NoError(t, key.Save(newNames[i], newPrivs[i].Public, false))
		if yes, err := fs.Exists(newNames[i]); !yes || err != nil {
			t.Fatal(err.Error())
		}
	}
	// decide a transition time
	transitionTime := time.Now().Add(100 * time.Second).Unix()
	transitionStr := strconv.Itoa(int(transitionTime))

	newGroupPath := path.Join(tmpPath, key.GroupFolderName)
	newArgs := []string{"drand", "group", "--folder", tmpPath, "--from", groupPath, "--transition", transitionStr, "--out", newGroupPath}
	newArgs = append(newArgs, newNames...)
	newCmd := exec.Command(newArgs[0], newArgs[1:]...)
	out, err = newCmd.CombinedOutput()
	// check it read all names given
	for _, name := range newNames {
		require.True(t, strings.Contains(string(out), name), string(out))
	}
	require.NoError(t, err, string(out))
	fmt.Println(string(out))

	// load and verify new information is correct
	newLoadedGroup := new(key.Group)
	require.NoError(t, key.Load(newGroupPath, newLoadedGroup))
	// expect the same genesis seed, period
	require.Equal(t, loadedGroup.GetGenesisSeed(), newLoadedGroup.GetGenesisSeed())
	require.Equal(t, loadedGroup.Period, newLoadedGroup.Period)
	require.Equal(t, transitionTime, newLoadedGroup.TransitionTime)
	for _, priv := range newPrivs {
		_, found := newLoadedGroup.Index(priv.Public)
		require.True(t, found)
	}

}

func TestStartAndStop(t *testing.T) {
	tmpPath := path.Join(os.TempDir(), "drand")
	os.Mkdir(tmpPath, 0740)
	defer os.RemoveAll(tmpPath)
	n := 5
	_, group := test.BatchIdentities(n)
	groupPath := path.Join(tmpPath, fmt.Sprintf("group.toml"))
	require.NoError(t, key.Save(groupPath, group, false))

	cmd := exec.Command("drand", "generate-keypair", "127.0.0.1:8080", "--tls-disable", "--folder", tmpPath)
	require.NoError(t, cmd.Run())
	startCh := make(chan bool)
	go func() {
		cmd = exec.Command("drand", "start", "--tls-disable", "--folder", tmpPath)
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
	cmd = exec.Command("drand", "stop")
	buff, err := cmd.CombinedOutput()
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

	cmd := exec.Command("drand", "start", "--tls-disable", "--folder", tmpPath)
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

	priv := key.NewKeyPair(addr)
	require.NoError(t, key.Save(pubPath, priv.Public, false))

	config := core.NewConfig(core.WithConfigFolder(tmpPath))
	fs := key.NewFileStore(config.ConfigFolder())
	require.NoError(t, fs.SaveKeyPair(priv))

	installCmd := exec.Command("go", "install")
	_, err := installCmd.Output()
	require.NoError(t, err)

	lctx, lcancel := context.WithCancel(context.Background())
	start1 := exec.CommandContext(lctx, "drand", "start", "--tls-disable", "--verbose", "2", "--folder", tmpPath, "--control", ctrlPort1)
	go start1.Run()

	fmt.Println(" DRAND SHARE ---")
	initDKGCmd := exec.Command("drand", "share", "--control", ctrlPort1)
	out, err := initDKGCmd.Output()
	expectedErr := "needs at least one group.toml file argument"
	output := string(out)
	require.Error(t, err)
	require.True(t, strings.Contains(output, expectedErr))
	lcancel()

	fmt.Println(" --- DRAND GROUP ---")
	// fake group
	_, group := test.BatchIdentities(5)
	priv.Public.TLS = false
	group.Period = 5 * time.Second
	group.GenesisTime = time.Now().Unix() - 10
	group.Nodes[0] = priv.Public
	group.Nodes[1] = priv.Public
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

	start2 := exec.CommandContext(ctx, "drand", "start", "--control", ctrlPort2, "--tls-disable", "--folder", tmpPath, "--verbose")
	start2.Stdout = os.Stdout
	start2.Stderr = os.Stderr
	go start2.Run()
	defer exec.Command("drand", "stop", "--control", ctrlPort2).Run()
	time.Sleep(500 * time.Millisecond)

	fmt.Println(" + running PING command with ", ctrlPort2)
	ping := exec.Command("drand", "ping", "--control", ctrlPort2)
	out, err = ping.CombinedOutput()
	require.NoError(t, err, string(out))

	require.NoError(t, toml.NewEncoder(os.Stdout).Encode(group))

	fmt.Printf("\n Running GET PRIVATE command with group file at %s\n", groupPath)
	loadedGroup := new(key.Group)
	require.NoError(t, key.Load(groupPath, loadedGroup))
	fmt.Printf("%s", loadedGroup.String())

	getCmd := exec.Command("drand", "get", "private", "--tls-disable", groupPath)
	out, err = getCmd.CombinedOutput()
	require.NoError(t, err, string(out))

	fakeStr := key.PointToString(fakeKey)
	cokeyCmd := exec.Command("drand", "get", "cokey", "--tls-disable", groupPath)
	out, err = cokeyCmd.CombinedOutput()
	require.True(t, strings.Contains(string(out), fakeStr))
	require.NoError(t, err)

	shareCmd := exec.Command("drand", "show", "share", "--control", ctrlPort2)
	out, err = shareCmd.CombinedOutput()
	if err != nil {
		fmt.Println(string(out))
		t.Fatalf("could not run the command : %s", err.Error())
	}
	expectedOutput := "0000000000000000000000000000000000000000000000000000000000000001"
	require.True(t, strings.Contains(string(out), expectedOutput))
	require.NoError(t, err)

	// reset state
	resetCmd := exec.Command("drand", "reset", "--folder", tmpPath)
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	startArgs := []string{"drand", "start", "--tls-cert", certPath, "--tls-key", keyPath, "--control", ctrlPort, "--folder", tmpPath}
	startCmd := exec.CommandContext(ctx, startArgs[0], startArgs[1:]...)
	go startCmd.Run()

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

	cmd = exec.Command("drand", "show", "share", "--control", ctrlPort)
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
