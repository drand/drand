package main

import (
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

	"github.com/dedis/drand/core"
	"github.com/dedis/drand/fs"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/test"
	"github.com/dedis/kyber"
	"github.com/dedis/kyber/pairing/bn256"
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
		fmt.Println("brilo")
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

func TestRunGroupInitBadPath(t *testing.T) {
	tmpPath := path.Join(os.TempDir(), "drand")
	os.Mkdir(tmpPath, 0777)
	defer os.RemoveAll(tmpPath)

	//tests reaction to empty group path
	emptyGroupPath := " "
	cmd := exec.Command("drand", "-c", tmpPath, "run", "--group-init", emptyGroupPath, "--insecure")
	out, err := cmd.CombinedOutput()
	fmt.Println(string(out))
	require.Error(t, err)

	//tests reaction to a bad group path
	wrongGroupPath := "not_here"
	cmd = exec.Command("drand", "-c", tmpPath, "run", "--group-init", wrongGroupPath, "--insecure")
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.Error(t, err)
}

func TestResetBeacon(t *testing.T) {
	tmpPath := path.Join(os.TempDir(), "drand")
	os.Mkdir(tmpPath, 0777)
	defer os.RemoveAll(tmpPath)

	config := core.NewConfig(core.WithConfigFolder(tmpPath))
	os.MkdirAll(config.DBFolder(), 0777)
	fakePath := path.Join(config.DBFolder(), "fake.data")
	require.NoError(t, ioutil.WriteFile(fakePath, []byte("fakyfaky"), 0777))
	tmpStdin, _ := ioutil.TempFile(tmpPath, "stdin")
	name := tmpStdin.Name()
	defer os.RemoveAll(name)
	tmpStdin.WriteString("n\n")
	tmpStdin.Close()
	tmpStdin, err := os.Open(name)
	require.NoError(t, err)
	os.Stdin = tmpStdin

	resetBeaconDB(config)
	// check if we still have the fake file
	if _, err := os.Stat(fakePath); err != nil {
		t.Fatal("database removed")
	}

	tmpStdin2, _ := ioutil.TempFile(tmpPath, "stdin2")
	name = tmpStdin2.Name()
	defer os.RemoveAll(name)
	tmpStdin2.WriteString("y\n")
	tmpStdin2.Close()
	tmpStdin2, err = os.Open(name)
	require.NoError(t, err)
	os.Stdin = tmpStdin2

	resetBeaconDB(config)
	// it should have removed the db
	if _, err := os.Stat(fakePath); err == nil {
		t.Fatal("database not removed")
	}

	// first create one set of key pair
	/*cmd := exec.Command("drand", "--config", config.ConfigFolder(), "keygen", "127.0.0.1:8080")*/
	//require.NoError(t, cmd.Run())
	//// read id
	//store := key.NewFileStore(config.ConfigFolder())
	//kp, err := store.LoadKeyPair()
	//require.NoError(t, err)
	//group := key.NewGroup([]*key.Identity{kp.Public, kp.Public, kp.Public, kp.Public}, 3)

	//// create fake group
	//groupsPath := path.Join(config.ConfigFolder(), "groups")
	//groupPath := path.Join(groupsPath, "group.toml")
	//require.NoError(t, key.Save(groupPath, group, false))

	//// create fake database
	//require.NoError(t, os.MkdirAll(config.DBFolder(), 0777))
	//// create fake file as a way to determine whether it has been deleted or not
	//fakePath := path.Join(config.DBFolder(), "fake.data")
	//require.NoError(t, ioutil.WriteFile(fakePath, []byte("fakyfaky"), 0777))

	//// launch with a group init and we answer by no to the question if we want
	//// to delete the beacon and we expect the fake file to be there
	//args := []string{"--config", tmpPath, "run", "--group-init", groupPath, "--insecure"}
	//cmd = exec.Command("drand", args...)
	//cmd.Stdin = strings.NewReader("y\n")
	//cmdReader, err := cmd.StdoutPipe()
	//if err != nil {
	//log.Fatal(err)
	//}
	//scanner := bufio.NewScanner(cmdReader)

	//go func() {
	//if err := cmd.Start(); err != nil {
	//log.Fatal(err)
	//}
	//if err := cmd.Wait(); err != nil {
	//log.Fatal(err)
	//}

	//}()

	//question := regexp.MustCompile("Accept to delete database")
	//deleted := regexp.MustCompile("Removed existing beacon")
	//for scanner.Scan() {
	//line := scanner.Text()
	////fmt.Println(line)
	//if question.MatchString(line) {
	//// if the question has been asked, then the database must have been
	//// deleted at the next line
	//if !deleted.MatchString(line) {
	//t.Fatal("not deleted")
	//} else {
	////cmd.Process.Kill()
	//return
	//}
	//}

	//}

	//[>err := cmd.CombinedOutput()<]
	////if err != nil {
	////fmt.Println("buffer: ", string(out))
	////t.Fatal()
	////}

	//// check if we still have the fake file
	//if _, err := os.Stat(fakePath); err == nil {
	//t.Fatal("database removed")
	/*}*/
}

func TestRunGroupInit(t *testing.T) {
	tmpPath := path.Join(os.TempDir(), "drand")
	os.Mkdir(tmpPath, 0777)
	defer os.RemoveAll(tmpPath)
	varEnv := "CRASHCRASH"
	n := 5
	_, group := test.BatchIdentities(n)
	groupPath := path.Join(tmpPath, fmt.Sprintf("group.toml"))
	require.NoError(t, key.Save(groupPath, group, false))

	cmd := exec.Command("drand", "-c", tmpPath, "run", "--group-init", groupPath, "--insecure")
	cmd.Env = append(os.Environ(), varEnv+"=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && e.Success() {
		t.Fatal(err)
	}
}

func TestClientTLS(t *testing.T) {
	tmpPath := path.Join(os.TempDir(), "drand")
	os.Mkdir(tmpPath, 0777)
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

	// fake dkg outuput
	pairing := bn256.NewSuite()
	G2 := pairing.G2()
	keyStr := "012067064287f0d81a03e575109478287da0183fcd8f3eda18b85042d1c8903ec8160c56eb6d5884d8c519c30bfa3bf5181f42bcd2efdbf4ba42ab0f31d13c97e9552543be1acf9912476b7da129d7c7e427fbafe69ac5b635773f488b8f46f3fc40c673b93a08a20c0e30fd84de8a89adb6fb95eca61ef2fff66527b3be4912de"
	fakeKey, _ := stringToPoint(G2, keyStr)
	distKey := &key.DistPublic{Key: fakeKey}
	require.NoError(t, fs.SaveDistPublic(distKey))

	os.Args = []string{"drand", "--config", tmpPath, "run", "--tls-cert", certPath, "--tls-key", keyPath, "--group-init", groupPath}
	go main()

	installCmd := exec.Command("go", "install")
	_, err := installCmd.Output()
	require.NoError(t, err)

	cmd := exec.Command("drand", "fetch", "private", "--tls-cert", certPath, pubPath)
	out, err := cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(t, err)

	cmd = exec.Command("drand", "fetch", "dist_key", "--tls-cert", certPath, addr)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.True(t, strings.Contains(string(out), keyStr))
	require.NoError(t, err)
}

func stringToPoint(g kyber.Group, s string) (kyber.Point, error) {
	buff, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	p := g.Point()
	return p, p.UnmarshalBinary(buff)
}
