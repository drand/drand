package drand

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	gnet "net"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/drand/drand/common"

	json "github.com/nikkolasg/hexjson"

	"github.com/BurntSushi/toml"
	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/boltdb"
	"github.com/drand/drand/common/scheme"
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

const expectedShareOutput = "0000000000000000000000000000000000000000000000000000000000000001"

func TestMigrate(t *testing.T) {
	tmp := getSBFolderStructure()
	defer os.RemoveAll(tmp)

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
	beaconID := common.GetBeaconIDFromEnv()

	tmp := getSBFolderStructure()
	defer os.RemoveAll(tmp)

	args := []string{"drand", "util", "reset", "--folder", tmp, "--id", beaconID}
	app := CLI()
	require.Error(t, app.Run(args))
}

func TestDeleteBeaconError(t *testing.T) {
	beaconID := common.GetBeaconIDFromEnv()

	tmp := getSBFolderStructure()
	defer os.RemoveAll(tmp)

	// that command should delete round 3 and 4
	args := []string{"drand", "util", "del-beacon", "--folder", tmp, "--id", beaconID, "3"}
	app := CLI()
	require.Error(t, app.Run(args))
}

func TestDeleteBeacon(t *testing.T) {
	beaconID := common.GetBeaconIDFromEnv()

	tmp := path.Join(os.TempDir(), "drand")
	defer os.RemoveAll(tmp)

	var opt = core.WithConfigFolder(tmp)
	conf := core.NewConfig(opt)
	fs.CreateSecureFolder(conf.DBFolder(beaconID))
	store, err := boltdb.NewBoltStore(conf.DBFolder(beaconID), conf.BoltOptions())
	require.NoError(t, err)
	store.Put(&chain.Beacon{
		Round:     1,
		Signature: []byte("Hello"),
	})
	store.Put(&chain.Beacon{
		Round:     2,
		Signature: []byte("Hello"),
	})
	store.Put(&chain.Beacon{
		Round:     3,
		Signature: []byte("Hello"),
	})
	store.Put(&chain.Beacon{
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

	args := []string{"drand", "util", "del-beacon", "--folder", tmp, "--id", beaconID, "3"}
	app := CLI()
	require.NoError(t, app.Run(args))

	store, err = boltdb.NewBoltStore(conf.DBFolder(beaconID), conf.BoltOptions())
	require.NoError(t, err)

	// try to fetch round 3 and 4 - it should now fail
	b, err = store.Get(3)
	require.Error(t, err)
	require.Nil(t, b)

	b, err = store.Get(4)
	require.Error(t, err)
	require.Nil(t, b)
}

func TestKeySelfSignError(t *testing.T) {
	beaconID := common.GetBeaconIDFromEnv()

	tmp := getSBFolderStructure()
	defer os.RemoveAll(tmp)

	args := []string{"drand", "util", "self-sign", "--folder", tmp, "--id", beaconID}
	app := CLI()
	require.Error(t, app.Run(args))
}

func TestKeySelfSign(t *testing.T) {
	beaconID := common.GetBeaconIDFromEnv()

	tmp := path.Join(os.TempDir(), "drand")
	defer os.RemoveAll(tmp)

	args := []string{"drand", "generate-keypair", "--folder", tmp, "--id", beaconID, "127.0.0.1:8081"}
	require.NoError(t, CLI().Run(args))

	selfSign := []string{"drand", "util", "self-sign", "--folder", tmp, "--id", beaconID}
	// try self sign, it should only print that it's already the case
	expectedOutput := "already self signed"
	testCommand(t, selfSign, expectedOutput)

	// load, remove signature and save
	config := core.NewConfig(core.WithConfigFolder(tmp))
	fileStore := key.NewFileStore(config.ConfigFolderMB(), beaconID)

	pair, err := fileStore.LoadKeyPair()
	require.NoError(t, err)
	pair.Public.Signature = nil
	require.NoError(t, fileStore.SaveKeyPair(pair))

	expectedOutput = "identity self signed"
	testCommand(t, selfSign, expectedOutput)
}

func TestKeyGenError(t *testing.T) {
	beaconID := common.GetBeaconIDFromEnv()

	tmp := getSBFolderStructure()
	defer os.RemoveAll(tmp)

	args := []string{"drand", "generate-keypair", "--folder", tmp, "--id", beaconID, "127.0.0.1:8081"}
	app := CLI()
	require.Error(t, app.Run(args))
}

func TestKeyGen(t *testing.T) {
	beaconID := common.GetBeaconIDFromEnv()

	tmp := path.Join(os.TempDir(), "drand")
	defer os.RemoveAll(tmp)

	args := []string{"drand", "generate-keypair", "--folder", tmp, "--id", beaconID, "127.0.0.1:8081"}
	require.NoError(t, CLI().Run(args))

	config := core.NewConfig(core.WithConfigFolder(tmp))
	fileStore := key.NewFileStore(config.ConfigFolderMB(), beaconID)
	priv, err := fileStore.LoadKeyPair()
	require.NoError(t, err)
	require.NotNil(t, priv.Public)

	tmp2 := path.Join(os.TempDir(), "drand2")
	defer os.RemoveAll(tmp2)

	args = []string{"drand", "generate-keypair", "--folder", tmp2, "--id", beaconID}
	require.Error(t, CLI().Run(args))

	config = core.NewConfig(core.WithConfigFolder(tmp2))
	fileStore = key.NewFileStore(config.ConfigFolderMB(), beaconID)
	priv, err = fileStore.LoadKeyPair()
	require.Error(t, err)
	require.Nil(t, priv)
}

// tests valid commands and then invalid commands
func TestStartAndStop(t *testing.T) {
	tmpPath := path.Join(os.TempDir(), "drand")
	os.Mkdir(tmpPath, 0740)
	defer os.RemoveAll(tmpPath)

	n := 5
	sch := scheme.GetSchemeFromEnv()
	beaconID := common.GetBeaconIDFromEnv()

	_, group := test.BatchIdentities(n, sch, beaconID)
	groupPath := path.Join(tmpPath, "group.toml")
	require.NoError(t, key.Save(groupPath, group, false))

	args := []string{"drand", "generate-keypair", "--tls-disable", "--folder", tmpPath, "--id", beaconID, "127.0.0.1:8080"}
	require.NoError(t, CLI().Run(args))
	startCh := make(chan bool)
	go func() {
		startArgs := []string{"drand", "start", "--tls-disable", "--folder", tmpPath}
		startCh <- true
		CLI().Run(startArgs)
		startCh <- true
		// TODO : figuring out how to not panic in grpc call
		// ERROR: 2020/01/23 21:06:28 grpc: server failed to encode response:
		// rpc error: code = Internal desc = grpc: error while marshaling: proto:
		// Marshal called with nil
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

func TestUtilCheck(t *testing.T) {
	beaconID := common.GetBeaconIDFromEnv()

	tmp, err := os.MkdirTemp("", "drand-cli-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmp)

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

	go CLI().RunContext(ctx, listen)
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
	sch := scheme.GetSchemeFromEnv()
	beaconID := common.GetBeaconIDFromEnv()

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

	ctrlPort1, ctrlPort2, metricsPort := test.FreePort(), test.FreePort(), test.FreePort()

	priv := key.NewKeyPair(addr)
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

	fmt.Println("--- DRAND SHARE --- (expected to fail)")
	// this must fail because not enough arguments
	// TODO - test vectors testing on the inputs

	initDKGArgs := []string{"drand", "share", "--control", ctrlPort1, "--id", beaconID}
	require.Error(t, CLI().Run(initDKGArgs))

	fmt.Println("--- DRAND STOP --- (failing instanace)")
	CLI().Run([]string{"drand", "stop", "--control", ctrlPort1})

	fmt.Println(" --- DRAND GROUP ---")

	// fake group
	_, group := test.BatchIdentities(5, sch, beaconID)

	// fake dkg outuput
	fakeKey := key.KeyGroup.Point().Pick(random.New())
	distKey := &key.DistPublic{
		Coefficients: []kyber.Point{fakeKey,
			key.KeyGroup.Point().Pick(random.New()),
			key.KeyGroup.Point().Pick(random.New()),
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
	scalarOne := key.KeyGroup.Scalar().One()
	s := &share.PriShare{I: 2, V: scalarOne}
	fakeShare := &key.Share{Share: s}
	require.NoError(t, fileStore.SaveShare(fakeShare))

	fmt.Println(" --- DRAND START --- control ", ctrlPort2)

	start2 := []string{
		"drand",
		"start",
		"--control", ctrlPort2,
		"--private-listen", priv.Public.Address(),
		"--tls-disable",
		"--folder", tmpPath,
		"--verbose",
		"--private-rand",
	}

	go func() {
		err := CLI().Run(start2)
		if err != nil {
			t.Errorf(err.Error())
		}
	}()

	defer CLI().Run([]string{"drand", "stop", "--control", ctrlPort2})

	time.Sleep(500 * time.Millisecond)

	testStartedDrandFunctional(t, ctrlPort2, tmpPath, priv.Public.Address(), group, fileStore, beaconID)
}

func testStartedDrandFunctional(t *testing.T, ctrlPort, rootPath, address string, group *key.Group, fileStore key.Store, beaconID string) {
	testPing(t, ctrlPort)
	testStatus(t, ctrlPort, beaconID)
	testListSchemes(t, ctrlPort)

	require.NoError(t, toml.NewEncoder(os.Stdout).Encode(group))

	groupPath := path.Join(rootPath, "drand_group.toml")
	fmt.Printf("\n Running GET PRIVATE command with group file at %s\n", groupPath)
	loadedGroup := new(key.Group)
	require.NoError(t, key.Load(groupPath, loadedGroup))
	fmt.Printf("%s", loadedGroup.String())

	getCmd := []string{"drand", "get", "private", "--tls-disable", groupPath}
	require.NoError(t, CLI().Run(getCmd))

	fmt.Printf("\n Running CHAIN-INFO command\n")
	chainInfo, err := json.MarshalIndent(chain.NewChainInfo(group).ToProto(nil), "", "    ")
	require.NoError(t, err)
	expectedOutput := string(chainInfo)
	chainInfoCmd := []string{"drand", "get", "chain-info", "--tls-disable", address}
	testCommand(t, chainInfoCmd, expectedOutput)

	fmt.Printf("\n Running CHAIN-INFO --HASH command\n")
	chainInfoCmdHash := []string{"drand", "get", "chain-info", "--hash", "--tls-disable", address}
	expectedOutput = fmt.Sprintf("%x", chain.NewChainInfo(group).Hash())
	testCommand(t, chainInfoCmdHash, expectedOutput)

	fmt.Println("\nRunning SHOW SHARE command")
	shareCmd := []string{"drand", "show", "share", "--control", ctrlPort}
	testCommand(t, shareCmd, expectedShareOutput)

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
	_, err = fileStore.LoadShare()
	require.Error(t, err)
	_, err = fileStore.LoadGroup()
	require.Error(t, err)
}

func testPing(t *testing.T, ctrlPort string) {
	var err error

	fmt.Println(" + running PING command with ", ctrlPort)
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
	var err error

	fmt.Println(" + running STATUS command with ", ctrlPort, " on beacon [", beaconID, "]")
	for i := 0; i < 3; i++ {
		status := []string{"drand", "util", "status", "--control", ctrlPort, "--id", beaconID}
		err = CLI().Run(status)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.NoError(t, err)
}

func testListSchemes(t *testing.T, ctrlPort string) {
	var err error

	fmt.Println(" + running list schemes command with ", ctrlPort)
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

func TestClientTLS(t *testing.T) {
	sch := scheme.GetSchemeFromEnv()
	beaconID := common.GetBeaconIDFromEnv()

	tmpPath := path.Join(os.TempDir(), "drand")
	os.Mkdir(tmpPath, 0740)
	defer os.RemoveAll(tmpPath)

	groupPath := path.Join(tmpPath, "group.toml")
	certPath := path.Join(tmpPath, "server.pem")
	keyPath := path.Join(tmpPath, "key.pem")
	pubPath := path.Join(tmpPath, "pub.key")

	freePort := test.FreePort()
	addr := "127.0.0.1:" + freePort
	ctrlPort := test.FreePort()
	metricsPort := test.FreePort()

	priv := key.NewTLSKeyPair(addr)
	require.NoError(t, key.Save(pubPath, priv.Public, false))

	config := core.NewConfig(core.WithConfigFolder(tmpPath))
	fileStore := key.NewFileStore(config.ConfigFolderMB(), beaconID)
	fileStore.SaveKeyPair(priv)

	if httpscerts.Check(certPath, keyPath) != nil {
		fmt.Println("generating on the fly")
		h, _, _ := gnet.SplitHostPort(priv.Public.Address())
		if err := httpscerts.Generate(certPath, keyPath, h); err != nil {
			panic(err)
		}
	}

	// fake group
	_, group := test.BatchTLSIdentities(5, sch, beaconID)
	// fake dkg outuput
	fakeKey := key.KeyGroup.Point().Pick(random.New())
	// need a threshold of coefficients
	distKey := &key.DistPublic{
		Coefficients: []kyber.Point{fakeKey,
			key.KeyGroup.Point().Pick(random.New()),
			key.KeyGroup.Point().Pick(random.New()),
		},
	}
	group.Nodes[0] = &key.Node{Identity: priv.Public, Index: 0}
	group.Period = 2 * time.Minute
	group.GenesisTime = time.Now().Unix()
	group.PublicKey = distKey
	require.NoError(t, fileStore.SaveGroup(group))
	require.NoError(t, key.Save(groupPath, group, false))

	// fake share
	scalarOne := key.KeyGroup.Scalar().One()
	s := &share.PriShare{I: 2, V: scalarOne}
	fakeShare := &key.Share{Share: s}
	fileStore.SaveShare(fakeShare)

	startArgs := []string{
		"drand",
		"start",
		"--private-listen", priv.Public.Address(),
		"--tls-cert", certPath,
		"--tls-key", keyPath,
		"--control", ctrlPort,
		"--folder", tmpPath,
		"--metrics", metricsPort,
		"--private-rand",
	}
	go CLI().Run(startArgs)
	defer CLI().Run([]string{"drand", "stop", "--control", ctrlPort})
	time.Sleep(500 * time.Millisecond)

	testStartedTLSDrandFunctional(t, ctrlPort, certPath, groupPath, group, priv)
}

func testStartedTLSDrandFunctional(t *testing.T, ctrlPort, certPath, groupPath string, group *key.Group, priv *key.Pair) {
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

	showPrivate := []string{"drand", "show", "private", "--control", ctrlPort}
	b, _ = priv.Key.MarshalBinary()
	exp = hex.EncodeToString(b)
	testCommand(t, showPrivate, exp)

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
	var buff bytes.Buffer
	output = &buff
	defer func() { output = os.Stdout }()
	fmt.Println("-------------_")
	require.NoError(t, CLI().Run(args))
	if exp == "" {
		return
	}
	fmt.Println("RUNNING: ", args)
	fmt.Println("EXPECTED: ", exp)
	fmt.Println("GOT: ", strings.Trim(buff.String(), "\n"), " --")
	fmt.Println("CONTAINS: ", strings.Contains(strings.Trim(buff.String(), "\n"), exp))
	require.True(t, strings.Contains(strings.Trim(buff.String(), "\n"), exp))
}

// getSBFolderStructure create a new single-beacon folder structure on a temporal folder
func getSBFolderStructure() string {
	tmp := path.Join(os.TempDir(), "drand")

	fs.CreateSecureFolder(path.Join(tmp, key.GroupFolderName))
	fs.CreateSecureFolder(path.Join(tmp, key.KeyFolderName))
	fs.CreateSecureFolder(path.Join(tmp, core.DefaultDBFolder))

	return tmp
}

func TestDrandListSchemes(t *testing.T) {
	n := 5
	instances, tempPath := launchDrandInstances(t, n)
	defer os.RemoveAll(tempPath)

	for _, instance := range instances {
		remote := []string{"drand", "util", "list-schemes", "--control", instance.ctrlPort}

		err := CLI().Run(remote)
		require.NoError(t, err)
	}
}

func TestDrandReloadBeacon(t *testing.T) {
	sch := scheme.GetSchemeFromEnv()
	beaconID := common.GetBeaconIDFromEnv()

	// beacon id need to have a value in order to stop one beacon process
	// if beacon id is empty, the id will be "default" internally
	if beaconID == "" {
		beaconID = common.DefaultBeaconID
	}

	n := 4
	instances, tempPath := launchDrandInstances(t, n)
	defer os.RemoveAll(tempPath)

	for i, inst := range instances {
		if i == 0 {
			inst.shareLeader(t, n, n, 2, beaconID, sch)
		} else {
			inst.share(t, instances[0].addr, beaconID)
		}
		time.Sleep(500 * time.Millisecond)
	}

	defer func() {
		for _, inst := range instances {
			inst.stopAll()
		}
	}()

	time.Sleep(1 * time.Second)

	// try to reload a beacon which is already loaded
	err := instances[3].load(beaconID)
	require.Error(t, err)

	// wait some time to generate some randomness
	time.Sleep(1 * time.Minute)

	// Stop beacon process... not the entire node
	err = instances[3].stop(beaconID)
	require.NoError(t, err)

	// check the node is still alive
	testPing(t, instances[3].ctrlPort)

	// reload a beacon
	err = instances[3].load(beaconID)
	require.NoError(t, err)

	// test beacon process status
	testStatus(t, instances[3].ctrlPort, beaconID)

	time.Sleep(5 * time.Second)

	// test beacon process status
	testStatus(t, instances[3].ctrlPort, beaconID)
}

func TestDrandStatus(t *testing.T) {
	n := 4
	instances, tempPath := launchDrandInstances(t, n)
	defer os.RemoveAll(tempPath)
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
	insToStop.stopAll()

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

func (d *drandInstance) shareLeader(t *testing.T, nodes, threshold, period int, beaconID string, sch scheme.Scheme) {
	shareArgs := []string{
		"drand",
		"share",
		"--leader",
		"--nodes", strconv.Itoa(nodes),
		"--threshold", strconv.Itoa(threshold),
		"--period", fmt.Sprintf("%ds", period),
		"--control", d.ctrlPort,
		"--scheme", sch.ID,
		"--id", beaconID,
	}

	go func() {
		err := CLI().Run(shareArgs)
		require.NoError(t, err)
	}()
}

func (d *drandInstance) share(t *testing.T, leaderURL, beaconID string) {
	shareArgs := []string{
		"drand",
		"share",
		"--connect", leaderURL,
		"--control", d.ctrlPort,
		"--id", beaconID,
	}

	go func() {
		err := CLI().Run(shareArgs)
		require.NoError(t, err)
	}()
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
	startArgs := []string{
		"drand",
		"start",
		"--tls-cert", d.certPath,
		"--tls-key", d.keyPath,
		"--certs-dir", d.certsDir,
		"--control", d.ctrlPort,
		"--folder", d.path,
		"--metrics", d.metrics,
		"--private-listen", d.addr,
	}

	go func() {
		err := CLI().Run(startArgs)
		require.NoError(t, err)
	}()

	// make sure we run each one sequentially
	testStatus(t, d.ctrlPort, beaconID)
}

//nolint: gocritic
func launchDrandInstances(t *testing.T, n int) ([]*drandInstance, string) {
	beaconID := common.GetBeaconIDFromEnv()

	tmpPath := path.Join(os.TempDir(), "drand")
	os.Mkdir(tmpPath, 0740)
	certsDir, err := ioutil.TempDir(tmpPath, "certs")
	require.NoError(t, err)

	var ins = make([]*drandInstance, 0, n)
	for i := 1; i <= n; i++ {
		nodePath, err := ioutil.TempDir(tmpPath, "node")
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
		priv := key.NewTLSKeyPair(addr)
		require.NoError(t, key.Save(pubPath, priv.Public, false))
		config := core.NewConfig(core.WithConfigFolder(nodePath))
		fileStore := key.NewFileStore(config.ConfigFolderMB(), beaconID)
		fileStore.SaveKeyPair(priv)

		h, _, _ := gnet.SplitHostPort(addr)
		if err := httpscerts.Generate(certPath, keyPath, h); err != nil {
			panic(err)
		}
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

	t.Setenv("DRAND_SHARE_SECRET", "testtesttestesttesttesttestesttesttesttestesttesttesttestest")
	for _, instance := range ins {
		instance.run(t, beaconID)
	}
	return ins, tmpPath
}
