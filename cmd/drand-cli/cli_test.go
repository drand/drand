package drand

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

	"github.com/stretchr/testify/require"
)

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
	args := []string{"drand", "util", "del-beacon", "--folder", tmp, "3"}
	app := CLI()
	require.NoError(t, app.Run(args))
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
	args := []string{"drand", "generate-keypair", "--folder", tmp, "127.0.0.1:8081"}
	require.NoError(t, CLI().Run(args))

	config := core.NewConfig(core.WithConfigFolder(tmp))
	fs := key.NewFileStore(config.ConfigFolder())
	priv, err := fs.LoadKeyPair()
	require.NoError(t, err)
	require.NotNil(t, priv.Public)

	tmp2 := path.Join(os.TempDir(), "drand2")
	defer os.RemoveAll(tmp2)
	args = []string{"drand", "generate-keypair", "--folder", tmp2}
	require.Error(t, CLI().Run(args))

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

	args := []string{"drand", "generate-keypair", "127.0.0.1:8080", "--tls-disable", "--folder", tmpPath}
	require.NoError(t, CLI().Run(args))
	startCh := make(chan bool)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		startArgs := []string{"start", "--tls-disable", "--folder", tmpPath}
		startCh <- true
		exec.CommandContext(ctx, "drand", startArgs...).Run()
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
	time.Sleep(200 * time.Millisecond)
	stopArgs := []string{"drand", "stop"}
	CLI().Run(stopArgs)
	select {
	case <-startCh:
	case <-time.After(1 * time.Second):
		t.Fatal("drand daemon did not stop")
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

	lctx, lcancel := context.WithCancel(context.Background())
	start1 := []string{"start", "--tls-disable", "--verbose", "2", "--folder", tmpPath, "--control", ctrlPort1, "--metrics", metricsPort}
	go exec.CommandContext(lctx, "drand", start1...).Run()
	time.Sleep(200 * time.Millisecond)

	fmt.Println(" DRAND SHARE ---")
	// this must fail because not enough arguments
	// TODO - test vectors testing on the inputs
	initDKGArgs := []string{"drand", "share", "--control", ctrlPort1}
	require.Error(t, CLI().Run(initDKGArgs))
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

	fmt.Println(" --- DRAND START --- control ", ctrlPort2)

	start2 := []string{"start", "--control", ctrlPort2, "--tls-disable", "--folder", tmpPath, "--verbose", "--private-rand"}
	go exec.CommandContext(ctx, "drand", start2...).Run()
	time.Sleep(300 * time.Millisecond)
	defer CLI().Run([]string{"drand", "stop", "--control", ctrlPort2})

	fmt.Println(" + running PING command with ", ctrlPort2)
	var err error
	for i := 0; i < 3; i++ {
		ping := []string{"drand", "util", "ping", "--control", ctrlPort2}
		err = CLI().Run(ping)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.NoError(t, err)

	require.NoError(t, toml.NewEncoder(os.Stdout).Encode(group))

	fmt.Printf("\n Running GET PRIVATE command with group file at %s\n", groupPath)
	loadedGroup := new(key.Group)
	require.NoError(t, key.Load(groupPath, loadedGroup))
	fmt.Printf("%s", loadedGroup.String())

	getCmd := []string{"drand", "get", "private", "--tls-disable", groupPath}
	require.NoError(t, CLI().Run(getCmd))

	fmt.Printf("\n Running GET COKEY command\n")
	fakeStr := key.PointToString(fakeKey)
	cokeyCmd := []string{"drand", "get", "cokey", "--tls-disable", priv.Public.Address()}
	testCommand(t, cokeyCmd, fakeStr)

	fmt.Println("\nRunning SHOW SHARE command")
	shareCmd := []string{"drand", "show", "share", "--control", ctrlPort2}
	expectedOutput := "0000000000000000000000000000000000000000000000000000000000000001"
	testCommand(t, shareCmd, expectedOutput)

	// reset state
	resetCmd := []string{"drand", "util", "reset", "--folder", tmpPath}
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.Write([]byte("y\n"))
	require.NoError(t, err)
	os.Stdin = r
	require.NoError(t, CLI().Run(resetCmd))
	_, err = fs.LoadDistPublic()
	require.Error(t, err)
	_, err = fs.LoadShare()
	require.Error(t, err)
	_, err = fs.LoadGroup()
	require.Error(t, err)
	fmt.Println("DONE")
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startArgs := []string{"start", "--tls-cert", certPath, "--tls-key", keyPath, "--control", ctrlPort, "--folder", tmpPath, "--metrics", metricsPort, "--private-rand"}
	go exec.CommandContext(ctx, "drand", startArgs...).Run()
	time.Sleep(200 * time.Millisecond)

	var err error
	for i := 0; i < 3; i++ {
		getPrivate := []string{"drand", "get", "private", "--tls-cert", certPath, groupPath}
		err = CLI().Run(getPrivate)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.Nil(t, err)

	getCokey := []string{"drand", "get", "cokey", "--tls-cert", certPath, addr}
	expectedOutput := keyStr
	testCommand(t, getCokey, expectedOutput)

	showCmd := []string{"drand", "show", "share", "--control", ctrlPort}
	expectedOutput = "0000000000000000000000000000000000000000000000000000000000000001"
	testCommand(t, showCmd, expectedOutput)

	showPublic := []string{"drand", "show", "public", "--control", ctrlPort}
	b, _ := priv.Public.Key.MarshalBinary()
	exp := hex.EncodeToString(b)
	testCommand(t, showPublic, exp)

	showPrivate := []string{"drand", "show", "private", "--control", ctrlPort}
	b, _ = priv.Key.MarshalBinary()
	exp = hex.EncodeToString(b)
	testCommand(t, showPrivate, exp)

	showCokey := []string{"drand", "show", "cokey", "--control", ctrlPort}
	expectedOutput = keyStr
	testCommand(t, showCokey, expectedOutput)

	showGroup := []string{"drand", "show", "group", "--control", ctrlPort}
	testCommand(t, showGroup, "")

	showHash := []string{"drand", "show", "group", "--control", ctrlPort, "--hash-only"}
	groupHash := hex.EncodeToString(group.Hash())
	testCommand(t, showHash, groupHash)
}

func testCommand(t *testing.T, args []string, exp string) {
	//capture := newStdoutCapture(t)
	//defer capture.Restore()
	var buff bytes.Buffer
	output = &buff
	defer func() { output = os.Stdout }()
	require.NoError(t, CLI().Run(args))
	if exp == "" {
		return
	}
	require.True(t, strings.Contains(strings.Trim(buff.String(), "\n"), exp))
}
