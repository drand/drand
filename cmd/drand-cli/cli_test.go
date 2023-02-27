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
	"github.com/kabukky/httpscerts"
	json "github.com/nikkolasg/hexjson"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/boltdb"
	"github.com/drand/drand/common"
	"github.com/drand/drand/core"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/fs"
	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	"github.com/drand/drand/test"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/share/dkg"
	"github.com/drand/kyber/util/random"
)

const expectedShareOutput = "0000000000000000000000000000000000000000000000000000000000000001"

func TestMigrate(t *testing.T) {
	tmp := getSBFolderStructure(t)

	args := []string{"drand", "util", "migrate", "--folder", tmp}
	app := CLI()
	require.NoError(t, app.Run(args))

	config := core.NewConfig(core.WithConfigFolder(tmp))
	defaultBeaconPath := path.Join(config.ConfigFolderMB(), common.DefaultBeaconID)

	newGroupFilePath := path.Join(defaultBeaconPath, key.GroupFolderName)
	newKeyFilePath := path.Join(defaultBeaconPath, key.KeyFolderName)
	newDBFilePath := path.Join(defaultBeaconPath, core.DefaultDBFolder)

	if !fs.FolderExists(defaultBeaconPath, newGroupFilePath) {
		t.Errorf("group folder should have been migrated")
	}
	if !fs.FolderExists(defaultBeaconPath, newKeyFilePath) {
		t.Errorf("key folder should have been migrated")
	}
	if !fs.FolderExists(defaultBeaconPath, newDBFilePath) {
		t.Errorf("db folder should have been migrated")
	}
}

func TestResetError(t *testing.T) {
	beaconID := test.GetBeaconIDFromEnv()

	tmp := getSBFolderStructure(t)

	args := []string{"drand", "util", "reset", "--folder", tmp, "--id", beaconID}
	app := CLI()
	require.Error(t, app.Run(args))
}

func TestDeleteBeaconError(t *testing.T) {
	beaconID := test.GetBeaconIDFromEnv()

	tmp := getSBFolderStructure(t)

	// that command should delete round 3 and 4
	args := []string{"drand", "util", "del-beacon", "--folder", tmp, "--id", beaconID, "3"}
	app := CLI()
	require.Error(t, app.Run(args))
}

func TestDeleteBeacon(t *testing.T) {
	beaconID := test.GetBeaconIDFromEnv()
	l := test.Logger(t)
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	if sch.Name == crypto.DefaultSchemeID {
		ctx = chain.SetPreviousRequiredOnContext(ctx)
	}
	tmp := path.Join(t.TempDir(), "drand")

	opt := core.WithConfigFolder(tmp)
	conf := core.NewConfig(opt)
	fs.CreateSecureFolder(conf.DBFolder(beaconID))
	store, err := boltdb.NewBoltStore(ctx, l, conf.DBFolder(beaconID), conf.BoltOptions())
	require.NoError(t, err)
	err = store.Put(ctx, &chain.Beacon{
		Round:     1,
		Signature: []byte("Hello"),
	})
	require.NoError(t, err)
	err = store.Put(ctx, &chain.Beacon{
		Round:     2,
		Signature: []byte("Hello"),
	})
	require.NoError(t, err)
	err = store.Put(ctx, &chain.Beacon{
		Round:     3,
		Signature: []byte("Hello"),
	})
	require.NoError(t, err)
	err = store.Put(ctx, &chain.Beacon{
		Round:     4,
		Signature: []byte("hello"),
	})
	require.NoError(t, err)
	// try to fetch round 3 and 4
	b, err := store.Get(ctx, 3)
	require.NoError(t, err)
	require.NotNil(t, b)
	b, err = store.Get(ctx, 4)
	require.NoError(t, err)
	require.NotNil(t, b)

	err = store.Close(ctx)
	require.NoError(t, err)

	args := []string{"drand", "util", "del-beacon", "--folder", tmp, "--id", beaconID, "3"}
	app := CLI()
	require.NoError(t, app.Run(args))

	store, err = boltdb.NewBoltStore(ctx, l, conf.DBFolder(beaconID), conf.BoltOptions())
	require.NoError(t, err)

	// try to fetch round 3 and 4 - it should now fail
	_, err = store.Get(ctx, 3)
	require.Error(t, err)

	_, err = store.Get(ctx, 4)
	require.Error(t, err)
}

func TestKeySelfSignError(t *testing.T) {
	beaconID := test.GetBeaconIDFromEnv()

	tmp := getSBFolderStructure(t)

	args := []string{"drand", "util", "self-sign", "--folder", tmp, "--id", beaconID}
	app := CLI()
	require.Error(t, app.Run(args))
}

func TestKeySelfSign(t *testing.T) {
	beaconID := test.GetBeaconIDFromEnv()

	tmp := path.Join(t.TempDir(), "drand")

	args := []string{"drand", "generate-keypair", "--folder", tmp, "--id", beaconID, "127.0.0.1:8081"}
	require.NoError(t, CLI().Run(args))

	selfSign := []string{"drand", "util", "self-sign", "--folder", tmp, "--id", beaconID}
	// try self sign, it should only print that it's already the case
	expectedOutput := "already self signed"
	testCommand(t, selfSign, expectedOutput)

	// load, remove signature and save
	config := core.NewConfig(core.WithConfigFolder(tmp))
	fileStore := key.NewFileStore(config.ConfigFolderMB(), beaconID)

	pair, err := fileStore.LoadKeyPair(nil)
	require.NoError(t, err)
	pair.Public.Signature = nil
	require.NoError(t, fileStore.SaveKeyPair(pair))

	expectedOutput = "identity self signed"
	testCommand(t, selfSign, expectedOutput)
}

func TestKeyGenError(t *testing.T) {
	beaconID := test.GetBeaconIDFromEnv()

	tmp := getSBFolderStructure(t)

	args := []string{"drand", "generate-keypair", "--folder", tmp, "--id", beaconID, "127.0.0.1:8081"}
	app := CLI()
	require.Error(t, app.Run(args))
}

func TestKeyGen(t *testing.T) {
	beaconID := test.GetBeaconIDFromEnv()

	tmp := path.Join(t.TempDir(), "drand")
	sch, _ := crypto.GetSchemeFromEnv()
	args := []string{"drand", "generate-keypair", "--folder", tmp, "--id", beaconID, "--scheme", sch.Name, "127.0.0.1:8081"}
	require.NoError(t, CLI().Run(args))

	config := core.NewConfig(core.WithConfigFolder(tmp))
	fileStore := key.NewFileStore(config.ConfigFolderMB(), beaconID)
	priv, err := fileStore.LoadKeyPair(nil)
	require.NoError(t, err)
	require.NotNil(t, priv.Public)

	tmp2 := path.Join(t.TempDir(), "drand2")

	args = []string{"drand", "generate-keypair", "--folder", tmp2, "--id", beaconID, "--scheme", sch.Name}
	require.Error(t, CLI().Run(args))

	config = core.NewConfig(core.WithConfigFolder(tmp2))
	fileStore = key.NewFileStore(config.ConfigFolderMB(), beaconID)
	priv, err = fileStore.LoadKeyPair(nil)
	require.Error(t, err)
	require.Nil(t, priv)
}

// tests valid commands and then invalid commands
func TestStartAndStop(t *testing.T) {
	t.Skipf("test is broken, doesn't check for errors.")
	tmpPath := t.TempDir()

	n := 5
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	beaconID := test.GetBeaconIDFromEnv()

	_, group := test.BatchIdentities(n, sch, beaconID)
	groupPath := path.Join(tmpPath, "group.toml")
	require.NoError(t, key.Save(groupPath, group, false))

	args := []string{"drand", "generate-keypair", "--tls-disable", "--folder", tmpPath, "--id", beaconID, "127.0.0.1:8080"}
	require.NoError(t, CLI().Run(args))

	startCh := make(chan bool)
	go func() {
		startArgs := []string{"drand", "start", "--tls-disable", "--folder", tmpPath}
		// Allow the rest of the test to start
		// Any error will be caught in the error check below
		startCh <- true
		err := CLI().Run(startArgs)
		if err != nil {
			t.Errorf("error starting the node %s\n", err)
			t.Fail()
			return
		}
		// After we finish the execution, flag that we finished.
		// This allows the test to exit cleanly without reaching the
		// timeout at the end.
		startCh <- true
		// TODO : figuring out how to not panic in grpc call
		// ERROR: 2020/01/23 21:06:28 grpc: server failed to encode response:
		// rpc error: code = Internal desc = grpc: error while marshaling: proto:
		// Marshal called with nil
	}()
	<-startCh
	time.Sleep(200 * time.Millisecond)

	stopArgs := []string{"drand", "stop"}
	err = CLI().Run(stopArgs)
	require.NoError(t, err)

	select {
	case <-startCh:
	case <-time.After(1 * time.Second):
		t.Fatal("drand daemon did not stop")
	}
}

func TestUtilCheck(t *testing.T) {
	beaconID := test.GetBeaconIDFromEnv()

	tmp := t.TempDir()

	// try to generate a keypair and make it listen on another address
	keyPort := test.FreePort()
	keyAddr := "127.0.0.1:" + keyPort
	generate := []string{"drand", "generate-keypair", "--tls-disable", "--folder", tmp, "--id", beaconID, keyAddr}
	require.NoError(t, CLI().Run(generate))

	listenPort := test.FreePort()
	listenAddr := "127.0.0.1:" + listenPort
	listen := []string{"drand", "start", "--tls-disable", "--private-listen", listenAddr, "--folder", tmp}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	waitCh := make(chan bool)
	go func() {
		waitCh <- true
		err := CLI().RunContext(ctx, listen)
		if err != nil {
			t.Errorf("error while starting the node %v\n", err)
			t.Fail()
			return
		}
	}()
	<-waitCh
	// XXX can we maybe try to bind continuously to not having to wait
	time.Sleep(200 * time.Millisecond)

	// run the check tool it should fail because key and address are not
	// consistent
	check := []string{"drand", "util", "check", "--tls-disable", "--id", beaconID, listenAddr}
	require.Error(t, CLI().Run(check))

	// cancel the daemon and make it listen on the right address
	cancel()
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	listen = []string{"drand", "start", "--tls-disable", "--folder", tmp, "--control", test.FreePort(), "--private-listen", keyAddr}
	go func() {
		err := CLI().RunContext(ctx, listen)
		if err != nil {
			t.Errorf(err.Error())
		}
	}()

	time.Sleep(200 * time.Millisecond)

	check = []string{"drand", "util", "check", "--verbose", "--tls-disable", keyAddr}
	require.NoError(t, CLI().Run(check))
}

//nolint:funlen
func TestStartWithoutGroup(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	beaconID := test.GetBeaconIDFromEnv()

	tmpPath := path.Join(t.TempDir(), "drand")
	require.NoError(t, os.Mkdir(tmpPath, 0o740))

	pubPath := path.Join(tmpPath, "pub.key")
	port1, _ := strconv.Atoi(test.FreePort())
	addr := "127.0.0.1:" + strconv.Itoa(port1)

	ctrlPort1, ctrlPort2, metricsPort := test.FreePort(), test.FreePort(), test.FreePort()

	priv, err := key.NewKeyPair(addr, nil)
	require.NoError(t, err)
	require.NoError(t, key.Save(pubPath, priv.Public, false))

	config := core.NewConfig(core.WithConfigFolder(tmpPath))
	fileStore := key.NewFileStore(config.ConfigFolderMB(), beaconID)
	require.NoError(t, fileStore.SaveKeyPair(priv))

	startArgs := []string{
		"drand",
		"start",
		"--private-listen", priv.Public.Address(),
		"--tls-disable",
		"--verbose",
		"--folder", tmpPath,
		"--control", ctrlPort1,
		"--metrics", "127.0.0.1:" + metricsPort,
	}

	go func() {
		err := CLI().Run(startArgs)
		if err != nil {
			t.Errorf(err.Error())
		}
	}()

	time.Sleep(500 * time.Millisecond)

	t.Log("--- DRAND SHARE --- (expected to fail)")
	// this must fail because not enough arguments
	// TODO - test vectors testing on the inputs

	initDKGArgs := []string{"drand", "share", "--control", ctrlPort1, "--id", beaconID}
	require.Error(t, CLI().Run(initDKGArgs))

	t.Log("--- DRAND STOP --- (failing instance)")
	err = CLI().Run([]string{"drand", "stop", "--control", ctrlPort1})
	require.NoError(t, err)

	t.Log(" --- DRAND GROUP ---")

	// fake group
	_, group := test.BatchIdentities(5, sch, beaconID)

	// fake dkg outuput
	fakeKey := sch.KeyGroup.Point().Pick(random.New())
	distKey := &key.DistPublic{
		Coefficients: []kyber.Point{
			fakeKey,
			sch.KeyGroup.Point().Pick(random.New()),
			sch.KeyGroup.Point().Pick(random.New()),
		},
	}
	priv.Public.TLS = false

	group.Period = 5 * time.Second
	group.GenesisTime = time.Now().Unix() - 10
	group.PublicKey = distKey
	group.Nodes[0] = &key.Node{Identity: priv.Public, Index: 0}
	group.Nodes[1] = &key.Node{Identity: priv.Public, Index: 1}
	groupPath := path.Join(tmpPath, "drand_group.toml")
	require.NoError(t, key.Save(groupPath, group, false))

	// save it also to somewhere drand will find it
	require.NoError(t, fileStore.SaveGroup(group))

	// fake share
	scalarOne := sch.KeyGroup.Scalar().One()
	s := &share.PriShare{I: 2, V: scalarOne}
	fakeShare := &key.Share{DistKeyShare: dkg.DistKeyShare{Share: s}, Scheme: sch}
	require.NoError(t, fileStore.SaveShare(fakeShare))

	t.Logf(" --- DRAND START --- control %s\n", ctrlPort2)

	start2 := []string{
		"drand",
		"start",
		"--control", ctrlPort2,
		"--private-listen", priv.Public.Address(),
		"--tls-disable",
		"--folder", tmpPath,
		"--verbose",
	}

	go func() {
		err := CLI().Run(start2)
		if err != nil {
			t.Errorf("error while starting second node: %v", err)
		}
	}()

	stop2 := []string{"drand", "stop", "--control", ctrlPort2}
	defer func() {
		err := CLI().Run(stop2)
		if err != nil {
			t.Errorf("error while stopping second node: %v", err)
		}
	}()

	time.Sleep(500 * time.Millisecond)

	testStartedDrandFunctional(t, ctrlPort2, tmpPath, priv.Public.Address(), group, fileStore, beaconID)
}

func testStartedDrandFunctional(t *testing.T, ctrlPort, rootPath, address string, group *key.Group, fileStore key.Store, beaconID string) {
	t.Helper()

	testPing(t, ctrlPort)
	testStatus(t, ctrlPort, beaconID)
	testListSchemes(t, ctrlPort)

	require.NoError(t, toml.NewEncoder(os.Stdout).Encode(group.TOML()))

	t.Log("Running CHAIN-INFO command")
	chainInfo, err := json.MarshalIndent(chain.NewChainInfo(group).ToProto(nil), "", "    ")
	require.NoError(t, err)
	expectedOutput := string(chainInfo)
	chainInfoCmd := []string{"drand", "get", "chain-info", "--tls-disable", address}
	testCommand(t, chainInfoCmd, expectedOutput)

	t.Log("Running CHAIN-INFO --HASH command")
	chainInfoCmdHash := []string{"drand", "get", "chain-info", "--hash", "--tls-disable", address}
	expectedOutput = fmt.Sprintf("%x", chain.NewChainInfo(group).Hash())
	testCommand(t, chainInfoCmdHash, expectedOutput)

	showChainInfo := []string{"drand", "show", "chain-info", "--control", ctrlPort}
	buffCi, err := json.MarshalIndent(chain.NewChainInfo(group).ToProto(nil), "", "    ")
	require.NoError(t, err)
	testCommand(t, showChainInfo, string(buffCi))

	showChainInfo = []string{"drand", "show", "chain-info", "--hash", "--control", ctrlPort}
	expectedOutput = fmt.Sprintf("%x", chain.NewChainInfo(group).Hash())
	testCommand(t, showChainInfo, expectedOutput)

	// reset state
	resetCmd := []string{"drand", "util", "reset", "--folder", rootPath, "--id", beaconID}
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString("y\n")
	require.NoError(t, err)
	os.Stdin = r
	require.NoError(t, CLI().Run(resetCmd))
	_, err = fileStore.LoadShare(nil)
	require.Error(t, err)
	_, err = fileStore.LoadGroup()
	require.Error(t, err)
}

func testPing(t *testing.T, ctrlPort string) {
	t.Helper()

	var err error

	t.Logf(" + running PING command with %s\n", ctrlPort)
	for i := 0; i < 3; i++ {
		ping := []string{"drand", "util", "ping", "--control", ctrlPort}
		err = CLI().Run(ping)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.NoError(t, err)
}

func testStatus(t *testing.T, ctrlPort, beaconID string) {
	t.Helper()

	var err error

	t.Logf(" + running STATUS command with %s on beacon [%s]", ctrlPort, beaconID)
	for i := 0; i < 3; i++ {
		status := []string{"drand", "util", "status", "--control", ctrlPort, "--id", beaconID}
		err = CLI().Run(status)
		if err == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.NoError(t, err)
}

func testFailStatus(t *testing.T, ctrlPort, beaconID string) {
	t.Helper()

	var err error

	t.Logf(" + running STATUS command with %s on beacon [%s]", ctrlPort, beaconID)
	for i := 0; i < 3; i++ {
		status := []string{"drand", "util", "status", "--control", ctrlPort, "--id", beaconID}
		err = CLI().Run(status)
		require.Error(t, err)
		time.Sleep(500 * time.Millisecond)
	}
}

func testListSchemes(t *testing.T, ctrlPort string) {
	t.Helper()

	var err error

	t.Logf(" + running list schemes command with %s\n", ctrlPort)
	for i := 0; i < 3; i++ {
		schemes := []string{"drand", "util", "list-schemes", "--control", ctrlPort}
		err = CLI().Run(schemes)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.NoError(t, err)
}

//nolint:funlen //This is a test
func TestClientTLS(t *testing.T) {
	t.Skipf("test fails when error checking commands")
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	beaconID := test.GetBeaconIDFromEnv()

	tmpPath := path.Join(t.TempDir(), "drand")
	os.Mkdir(tmpPath, 0o740)

	groupPath := path.Join(tmpPath, "group.toml")
	certPath := path.Join(tmpPath, "server.pem")
	keyPath := path.Join(tmpPath, "key.pem")
	pubPath := path.Join(tmpPath, "pub.key")

	freePort := test.FreePort()
	addr := "127.0.0.1:" + freePort
	ctrlPort := test.FreePort()
	metricsPort := test.FreePort()

	priv, err := key.NewTLSKeyPair(addr, nil)
	require.NoError(t, err)
	require.NoError(t, key.Save(pubPath, priv.Public, false))

	config := core.NewConfig(core.WithConfigFolder(tmpPath))
	fileStore := key.NewFileStore(config.ConfigFolderMB(), beaconID)
	err = fileStore.SaveKeyPair(priv)
	require.NoError(t, err)

	if httpscerts.Check(certPath, keyPath) != nil {
		t.Log("generating on the fly")
		h, _, err := gnet.SplitHostPort(priv.Public.Address())
		require.NoError(t, err)
		err = httpscerts.Generate(certPath, keyPath, h)
		require.NoError(t, err)
	}

	// fake group
	_, group := test.BatchTLSIdentities(5, sch, beaconID)
	// fake dkg outuput
	fakeKey := sch.KeyGroup.Point().Pick(random.New())
	// need a threshold of coefficients
	distKey := &key.DistPublic{
		Coefficients: []kyber.Point{
			fakeKey,
			sch.KeyGroup.Point().Pick(random.New()),
			sch.KeyGroup.Point().Pick(random.New()),
		},
	}
	group.Nodes[0] = &key.Node{Identity: priv.Public, Index: 0}
	group.Period = 2 * time.Minute
	group.GenesisTime = time.Now().Unix()
	group.PublicKey = distKey
	require.NoError(t, fileStore.SaveGroup(group))
	require.NoError(t, key.Save(groupPath, group, false))

	// fake share
	scalarOne := sch.KeyGroup.Scalar().One()
	s := &share.PriShare{I: 2, V: scalarOne}
	// TODO: check DistKeyShare if it needs a scheme
	fakeShare := &key.Share{DistKeyShare: dkg.DistKeyShare{Share: s}, Scheme: sch}
	err = fileStore.SaveShare(fakeShare)
	require.NoError(t, err)

	startArgs := []string{
		"drand",
		"start",
		"--private-listen", priv.Public.Address(),
		"--tls-cert", certPath,
		"--tls-key", keyPath,
		"--control", ctrlPort,
		"--folder", tmpPath,
		"--metrics", metricsPort,
	}
	go func() {
		err := CLI().Run(startArgs)
		if err != nil {
			t.Errorf("error while starting node: %v", err)
		}
	}()

	stopArgs := []string{"drand", "stop", "--control", ctrlPort}
	defer func() {
		err := CLI().Run(stopArgs)
		if err != nil {
			t.Errorf("error while stopping the node: %v", err)
		}
	}()

	time.Sleep(500 * time.Millisecond)

	testStartedTLSDrandFunctional(t, ctrlPort, certPath, group, priv)
}

//nolint:unused // We want to provide convenience functions
func testStartedTLSDrandFunctional(t *testing.T, ctrlPort, certPath string, group *key.Group, priv *key.Pair) {
	t.Helper()

	var err error

	chainInfoCmd := []string{"drand", "get", "chain-info", "--tls-cert", certPath, priv.Public.Address()}
	chainInfoBuff, err := json.MarshalIndent(chain.NewChainInfo(group).ToProto(nil), "", "    ")
	require.NoError(t, err)
	expectedOutput := string(chainInfoBuff)
	testCommand(t, chainInfoCmd, expectedOutput)

	showCmd := []string{"drand", "show", "share", "--control", ctrlPort}
	testCommand(t, showCmd, expectedShareOutput)

	showPublic := []string{"drand", "show", "public", "--control", ctrlPort}
	b, _ := priv.Public.Key.MarshalBinary()
	exp := hex.EncodeToString(b)
	testCommand(t, showPublic, exp)

	showCokey := []string{"drand", "show", "chain-info", "--control", ctrlPort}
	expectedOutput = string(chainInfoBuff)
	testCommand(t, showCokey, expectedOutput)

	showGroup := []string{"drand", "show", "group", "--control", ctrlPort}
	testCommand(t, showGroup, "")

	showHash := []string{"drand", "show", "group", "--control", ctrlPort, "--hash"}
	groupHash := hex.EncodeToString(group.Hash())
	testCommand(t, showHash, groupHash)
}

func testCommand(t *testing.T, args []string, exp string) {
	t.Helper()

	var buff bytes.Buffer
	output = &buff
	defer func() { output = os.Stdout }()
	t.Log("--------------")
	require.NoError(t, CLI().Run(args))
	if exp == "" {
		return
	}
	t.Logf("RUNNING: %v\n", args)
	require.Contains(t, strings.Trim(buff.String(), "\n"), exp)
}

// getSBFolderStructure create a new single-beacon folder structure in a temporary folder
func getSBFolderStructure(t *testing.T) string {
	t.Helper()

	tmp := path.Join(t.TempDir(), "drand")

	fs.CreateSecureFolder(path.Join(tmp, key.GroupFolderName))
	fs.CreateSecureFolder(path.Join(tmp, key.KeyFolderName))
	fs.CreateSecureFolder(path.Join(tmp, core.DefaultDBFolder))

	return tmp
}

func TestDrandListSchemes(t *testing.T) {
	n := 2
	instances := genAndLaunchDrandInstances(t, n)

	for _, instance := range instances {
		remote := []string{"drand", "util", "list-schemes", "--control", instance.ctrlPort}

		err := CLI().Run(remote)
		require.NoError(t, err)
	}
}

func TestDrandReloadBeacon(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	beaconID := test.GetBeaconIDFromEnv()

	n := 4
	instances := genAndLaunchDrandInstances(t, n)

	done := make(chan error, n)
	for i, inst := range instances {
		if i == 0 {
			go inst.shareLeader(t, n, n, 1, beaconID, sch, done)
			// Wait a bit after launching the leader to launch the other nodes too.
			time.Sleep(500 * time.Millisecond)
		} else {
			go inst.share(t, instances[0].addr, beaconID, done)
		}
	}

	t.Log("waiting for initial set up to settle on all nodes")
	for i := 0; i < n; i++ {
		err := <-done
		require.NoError(t, err)
	}

	defer func() {
		for _, inst := range instances {
			// We want to ignore this error, at least until the stop command won't return an error
			// when correctly running the stop command.
			t.Logf("stopping instance %v\n", inst.addr)
			err := inst.stopAll()
			require.NoError(t, err)
			t.Logf("stopped instance %v\n", inst.addr)
		}
	}()

	t.Log("waiting for initial setup to finish")
	time.Sleep(5 * time.Second)

	// try to reload a beacon which is already loaded
	err = instances[3].load(beaconID)
	require.Error(t, err)

	// Stop beacon process... not the entire node
	err = instances[3].stop(beaconID)
	require.NoError(t, err)

	// check the node is still alive
	testPing(t, instances[3].ctrlPort)

	t.Log("waiting for beacons to be generated while a beacon process is stopped on a node")
	time.Sleep(10 * time.Second)

	// reload a beacon
	err = instances[3].load(beaconID)
	require.NoError(t, err)

	// test beacon process status
	testStatus(t, instances[3].ctrlPort, beaconID)

	time.Sleep(3 * time.Second)

	// test beacon process status
	testStatus(t, instances[3].ctrlPort, beaconID)
}

func TestDrandLoadNotPresentBeacon(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	beaconID := test.GetBeaconIDFromEnv()

	n := 4
	instances := genAndLaunchDrandInstances(t, n)

	done := make(chan error, n)
	for i, inst := range instances {
		if i == 0 {
			go inst.shareLeader(t, n, n, 1, beaconID, sch, done)
			// Wait a bit after launching the leader to launch the other nodes too.
			time.Sleep(500 * time.Millisecond)
		} else {
			go inst.share(t, instances[0].addr, beaconID, done)
		}
	}

	t.Log("waiting for initial set up to settle on all nodes")
	for i := 0; i < n; i++ {
		err := <-done
		require.NoError(t, err)
	}

	defer func() {
		for _, inst := range instances {
			_ = inst.stopAll()
		}
	}()

	t.Log("waiting for initial setup to finish")
	time.Sleep(5 * time.Second)

	// Stop beacon process... not the entire node
	err = instances[3].stop(beaconID)
	require.NoError(t, err)

	t.Log("waiting for beacons to be generated while a beacon process is stopped on a node")
	time.Sleep(10 * time.Second)

	// reload a different beacon
	err = instances[3].load("not-a-valid-beacon-name-here")
	require.Error(t, err)

	// test original beacon process status and hope it's still off
	testFailStatus(t, instances[3].ctrlPort, beaconID)
}

func TestDrandStatus(t *testing.T) {
	t.Skipf("test fails when error checking commands")
	n := 4
	instances := genAndLaunchDrandInstances(t, n)
	allAddresses := make([]string, 0, n)
	for _, instance := range instances {
		allAddresses = append(allAddresses, instance.addr)
	}

	defer func() { output = os.Stdout }()

	// check that each node can reach each other
	for i, instance := range instances {
		remote := []string{"drand", "util", "remote-status", "--control", instance.ctrlPort}
		remote = append(remote, allAddresses...)
		var buff bytes.Buffer
		output = &buff

		err := CLI().Run(remote)
		require.NoError(t, err)
		for j, instance := range instances {
			if i == j {
				continue
			}
			require.True(t, strings.Contains(buff.String(), instance.addr+" -> OK"))
		}
	}
	// stop one and check that all nodes report this node down
	toStop := 2
	insToStop := instances[toStop]
	err := insToStop.stopAll()
	require.NoError(t, err)

	for i, instance := range instances {
		if i == toStop {
			continue
		}
		remote := []string{"drand", "util", "remote-status", "--control", instance.ctrlPort}
		remote = append(remote, allAddresses...)
		var buff bytes.Buffer
		output = &buff

		err := CLI().Run(remote)
		require.NoError(t, err)
		for j, instance := range instances {
			if i == j {
				continue
			}
			if j != toStop {
				require.True(t, strings.Contains(buff.String(), instance.addr+" -> OK"))
			} else {
				require.True(t, strings.Contains(buff.String(), instance.addr+" -> X"))
			}
		}
	}
}

func TestEmptyPortSelectionUsesDefaultDuringKeygen(t *testing.T) {
	beaconID := test.GetBeaconIDFromEnv()

	tmp := t.TempDir()
	app := CLI()

	// args are missing a port for the node address
	args := []string{"drand", "generate-keypair", "--folder", tmp, "--id", beaconID, "127.0.0.1"}
	// after being prompted for a port, the 'user' hits enter to select the default
	app.Reader = strings.NewReader("\n")

	require.NoError(t, app.Run(args))
}

func TestValidPortSelectionSucceedsDuringKeygen(t *testing.T) {
	beaconID := test.GetBeaconIDFromEnv()

	tmp := t.TempDir()
	app := CLI()

	args := []string{"drand", "generate-keypair", "--folder", tmp, "--id", beaconID, "127.0.0.1"}
	app.Reader = strings.NewReader("8080\n")

	require.NoError(t, app.Run(args))
}

type drandInstance struct {
	path     string
	ctrlPort string
	addr     string
	metrics  string
	certPath string
	keyPath  string
	certsDir string
}

func (d *drandInstance) stopAll() error {
	return CLI().Run([]string{"drand", "stop", "--control", d.ctrlPort})
}

func (d *drandInstance) stop(beaconID string) error {
	return CLI().Run([]string{"drand", "stop", "--control", d.ctrlPort, "--id", beaconID})
}

func (d *drandInstance) shareLeader(t *testing.T,
	nodes, threshold, periodSeconds int,
	beaconID string,
	sch *crypto.Scheme,
	done chan error) {
	t.Helper()

	shareArgs := []string{
		"drand",
		"share",
		"--leader",
		"--nodes", strconv.Itoa(nodes),
		"--threshold", strconv.Itoa(threshold),
		"--period", fmt.Sprintf("%ds", periodSeconds),
		"--control", d.ctrlPort,
		"--scheme", sch.Name,
		"--id", beaconID,
	}

	done <- CLI().Run(shareArgs)
}

func (d *drandInstance) share(t *testing.T, leaderURL, beaconID string, done chan error) {
	t.Helper()

	shareArgs := []string{

		"drand",
		"share",
		"--connect", leaderURL,
		"--control", d.ctrlPort,
		"--id", beaconID,
	}

	done <- CLI().Run(shareArgs)
}

func (d *drandInstance) load(beaconID string) error {
	reloadArgs := []string{
		"drand",
		"load",
		"--control", d.ctrlPort,
		"--id", beaconID,
	}

	return CLI().Run(reloadArgs)
}

func (d *drandInstance) run(t *testing.T, beaconID string) {
	t.Helper()

	d.runWithStartArgs(t, beaconID, nil)
}

func (d *drandInstance) runWithStartArgs(t *testing.T, beaconID string, startArgs []string) {
	t.Helper()

	require.Equal(t, 0, len(startArgs)%2, "start args must be in pairs of option/value")

	baseArgs := []string{
		"drand",
		"start",
		"--verbose",
		"--tls-cert", d.certPath,
		"--tls-key", d.keyPath,
		"--certs-dir", d.certsDir,
		"--control", d.ctrlPort,
		"--folder", d.path,
		"--metrics", d.metrics,
		"--private-listen", d.addr,
	}

	args := append(baseArgs, startArgs...)

	go func() {
		err := CLI().Run(args)
		require.NoError(t, err)
	}()

	// make sure we run each one sequentially
	testStatus(t, d.ctrlPort, beaconID)
}

func genAndLaunchDrandInstances(t *testing.T, n int) []*drandInstance {
	t.Helper()

	beaconID := test.GetBeaconIDFromEnv()

	ins := genDrandInstances(t, beaconID, n)
	return launchDrandInstances(t, beaconID, ins)
}

func genDrandInstances(t *testing.T, beaconID string, n int) []*drandInstance {
	t.Helper()

	tmpPath := t.TempDir()

	certsDir := path.Join(tmpPath, "certs")
	require.NoError(t, os.Mkdir(certsDir, 0o740))

	ins := make([]*drandInstance, 0, n)
	for i := 1; i <= n; i++ {
		nodePath, err := os.MkdirTemp(tmpPath, "node")
		require.NoError(t, err)

		certPath := path.Join(nodePath, "cert")
		keyPath := path.Join(nodePath, "tls.key")
		pubPath := path.Join(tmpPath, "pub.key")

		freePort := test.FreePort()
		addr := "127.0.0.1:" + freePort
		ctrlPort := test.FreePort()
		metricsPort := test.FreePort()

		// generate key so it loads
		// XXX let's remove this requirement - no need for longterm keys
		priv, err := key.NewTLSKeyPair(addr, nil)
		require.NoError(t, err)
		require.NoError(t, key.Save(pubPath, priv.Public, false))
		config := core.NewConfig(core.WithConfigFolder(nodePath))
		fileStore := key.NewFileStore(config.ConfigFolderMB(), beaconID)
		err = fileStore.SaveKeyPair(priv)
		require.NoError(t, err)

		h, _, err := gnet.SplitHostPort(addr)
		require.NoError(t, err)

		err = httpscerts.Generate(certPath, keyPath, h)
		require.NoError(t, err)

		// copy into one folder for giving a common CERT folder
		_, err = exec.Command("cp", certPath, path.Join(certsDir, fmt.Sprintf("cert-%d", i))).Output()
		require.NoError(t, err)

		ins = append(ins, &drandInstance{
			addr:     addr,
			ctrlPort: ctrlPort,
			path:     nodePath,
			keyPath:  keyPath,
			metrics:  metricsPort,
			certPath: certPath,
			certsDir: certsDir,
		})
	}

	return ins
}

func launchDrandInstances(t *testing.T, beaconID string, ins []*drandInstance) []*drandInstance {
	t.Helper()

	t.Setenv("DRAND_SHARE_SECRET", "testtesttestesttesttesttestesttesttesttestesttesttesttestest")
	for _, instance := range ins {
		instance.run(t, beaconID)
	}
	return ins
}

func TestSharingWithInvalidFlagCombos(t *testing.T) {
	beaconID := test.GetBeaconIDFromEnv()

	// leader and connect flags can't be used together
	share1 := []string{
		"drand", "share", "--tls-disable", "--id", beaconID, "--leader", "--connect", "127.0.0.1:9090",
		"--threshold", "2", "--nodes", "3", "--period", "5s",
	}

	require.EqualError(t, CLI().Run(share1), "you can't use the leader and connect flags together")

	// transition and from flags can't be used together
	share3 := []string{
		"drand", "share", "--tls-disable", "--id", beaconID, "--connect", "127.0.0.1:9090", "--transition", "--from", "somepath.txt",
	}

	require.EqualError(
		t,
		CLI().Run(share3),
		"--from flag invalid with --reshare - nodes resharing should already have a secret share and group ready to use",
	)
}

func TestMemDBBeaconReJoinsNetworkAfterLongStop(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	beaconID := test.GetBeaconIDFromEnv()

	// How many rounds to generate while the node is stopped.
	roundsWhileMissing := 80
	// If we are in short mode, let's run less rounds.
	if testing.Short() {
		roundsWhileMissing  = 20
	}

	period := 1
	n := 4
	instances := genDrandInstances(t, beaconID, n)
	memDBNodeID := len(instances) - 1

	t.Setenv("DRAND_SHARE_SECRET", "testtesttestesttesttesttestesttesttesttestesttesttesttestest")
	for i := 0; i < memDBNodeID; i++ {
		inst := instances[i]
		inst.run(t, beaconID)
	}

	instances[memDBNodeID].runWithStartArgs(t, beaconID, []string{"--db", "memdb"})
	memDBNode := instances[memDBNodeID]

	done := make(chan error, n)
	for i, inst := range instances {
		inst := inst
		if i == 0 {
			go inst.shareLeader(t, n, 3, period, beaconID, sch, done)
			// Wait a bit after launching the leader to launch the other nodes too.
			time.Sleep(500 * time.Millisecond)
		} else {
			go inst.share(t, instances[0].addr, beaconID, done)
		}
	}

	t.Log("waiting for initial set up to settle on all nodes")
	for i := 0; i < n; i++ {
		err := <-done
		require.NoError(t, err)
	}

	defer func() {
		for _, inst := range instances {
			// We want to ignore this error, at least until the stop command won't return an error
			// when correctly running the stop command.
			t.Logf("stopping instance %v\n", inst.addr)
			err := inst.stopAll()
			require.NoError(t, err)
			t.Logf("stopped instance %v\n", inst.addr)
		}
	}()

	memDBClient, err := net.NewControlClient(memDBNode.ctrlPort)
	require.NoError(t, err)

	chainInfo, err := memDBClient.ChainInfo(beaconID)
	require.NoError(t, err)

	// Wait until DKG finishes
	secondsToGenesisTime := chainInfo.GenesisTime - time.Now().Unix()
	t.Logf("waiting %ds until DKG finishes\n", secondsToGenesisTime)
	time.Sleep(time.Duration(secondsToGenesisTime) * time.Second)

	// Wait for some rounds to be generated
	t.Log("wait for some rounds to be generated")
	time.Sleep(time.Duration(roundsWhileMissing*period) * time.Second)

	// Get the status before stopping the node
	status, err := memDBClient.Status(beaconID)
	require.NoError(t, err)

	require.False(t, status.ChainStore.IsEmpty)
	require.NotZero(t, status.ChainStore.LastRound)

	lastRoundBeforeShutdown := status.ChainStore.LastRound

	// Stop beacon process... not the entire node
	err = instances[memDBNodeID].stop(beaconID)
	require.NoError(t, err)

	t.Log("waiting for beacons to be generated while a beacon process is stopped on a node")
	time.Sleep(time.Duration(roundsWhileMissing*period) * time.Second)

	// reload a beacon
	err = instances[memDBNodeID].load(beaconID)
	require.NoError(t, err)

	// Wait a bit to allow the node to startup and load correctly
	time.Sleep(2 * time.Second)

	status, err = memDBClient.Status(beaconID)
	require.NoError(t, err)

	require.False(t, status.ChainStore.IsEmpty)
	require.NotZero(t, status.ChainStore.LastRound)
	expectedRound := lastRoundBeforeShutdown + uint64(roundsWhileMissing)
	t.Logf("comparing lastRound %d with lastRoundBeforeShutdown %d\n", status.ChainStore.LastRound, expectedRound)
	require.GreaterOrEqual(t, status.ChainStore.LastRound, expectedRound)
}
