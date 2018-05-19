package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"testing"

	"github.com/dedis/drand/core"
	"github.com/dedis/drand/fs"
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

func TestDKG(t *testing.T) {
	/* n := 5*/
	//thr := 4
	//tmpPath := path.Join(os.TempDir(), "drand")
	//os.Mkdir(tmpPath, 0777)
	//defer os.RemoveAll(tmpPath)

	//exec.Command("pkill", "beacon").Run()
	//exec.Command("pkill", "dkg").Run()

	//folders := make([]string, n, n)
	//configs := make([]*core.Config, n, n)
	//privs := make([]string, n, n)
	//addrs := make([]string, n, n)
	//for i := 0; i < n; i++ {
	//// create folder and private key
	//folders[i] = path.Join(tmpPath, fmt.Sprintf("p%d", i))
	//defer os.RemoveAll(folders[i])
	//os.Mkdir(folders[i], 0777)
	//configs[i] = core.NewConfig(core.WithConfigFolder(folders[i]))
	//addrs[i] = "127.0.0.1:" + fmt.Sprintf("%d", 8000+i)
	//priv := key.NewKeyPair(addrs[i])
	//require.Nil(t, key.NewFileStore(configs[i].ConfigFolder()).SavePrivate(priv))
	//privs[i] = path.Join(tmpPath, fmt.Sprintf("public-%d", i))
	//require.Nil(t, key.Save(privs[i], priv.Public, false))
	//}

	//// create the group file
	//os.Args = []string{"drand", "group", "--threshold", strconv.Itoa(thr)}
	//os.Args = append(os.Args, privs...)
	//main()
	//defer os.Remove(gname)

	//cmd := exec.Command("go", "build")
	//require.NoError(t, cmd.Run())

	//doneCh := make(chan bool)
	//// launch all nodes in DKG mode
	//for i := 0; i < n; i++ {
	//go func(idx int) {
	//args := []string{"--config", folders[idx], "-d", "dkg", gname}
	//if idx == n-1 {
	//args = append(args, "--leader")
	//}
	//cmd := exec.Command("./drand", args...)
	//cmd.Stdout = os.Stdout
	//cmd.Stderr = os.Stderr
	//err := cmd.Run()
	//require.Nil(t, err)
	//doneCh <- true
	//}(i)
	//}

	//for count := 0; count < n; count++ {
	//<-doneCh
	//}

	//// check if dist public key and share are there
	//for i := 0; i < n; i++ {
	//store := key.NewFileStore(configs[i].ConfigFolder())
	//_, err := store.LoadShare()
	//require.NoError(t, err)
	//_, err = store.LoadDistPublic()
	//require.NoError(t, err)
	//}
	//publicPath := path.Join(fs.Pwd(), dpublic)
	//if ok, _ := fs.Exists(publicPath); !ok {
	//t.Fatal("dkg public does not exists")
	//}
	//defer os.Remove(publicPath)
	//// launch beacon
	////period := 1 * time.Second
	//periodStr := "1s"
	//quits := make([]chan bool, n)
	//processes := make(chan *os.Process, n)
	//fmt.Println("starting beacon...")
	//for i := 0; i < n; i++ {
	//quits[i] = make(chan bool, 1)
	//go func(idx int) {
	//var buff bytes.Buffer
	//args := []string{"--config", folders[idx], "beacon", "--period", periodStr}
	//cmd := exec.Command("./drand", args...)
	//require.Nil(t, cmd.Start())
	//cmd.Stdout = &buff
	//cmd.Stderr = &buff
	//time.Sleep(500 * time.Millisecond)
	//fmt.Println(string(buff.Bytes()))
	//processes <- cmd.Process
	//cmd.Wait()
	//fmt.Println(string(buff.Bytes()))
	//<-quits[idx]
	//cmd.Process.Kill()
	//}(i)
	//}

	//toKill := make([]*os.Process, 0)
	//for i := 0; i < n; i++ {
	//toKill = append(toKill, <-processes)
	//}
	//defer func() {
	//for i := 0; i < n; i++ {
	//toKill[i].Kill()
	//}
	//}()

	//fmt.Println("beacon all ran!")
	//time.Sleep(2 * time.Second)
	//fmt.Println("==> trying to contact with client")
	//// try to fetch a beacon
	//args := []string{"fetch", "public", "--public", publicPath, addrs[0]}
	//cmd = exec.Command("./drand", args...)
	//cmd.Env = append(cmd.Env, "GRPC_GO_LOG_SEVERITY_LEVEL=info")
	//out, err := cmd.CombinedOutput()
	//fmt.Println(string(out))
	//require.Nil(t, err)
	/*time.Sleep(1 * time.Second)*/
}
