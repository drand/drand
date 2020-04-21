package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"
	"github.com/kabukky/httpscerts"
)

var secretDKG = "dkgsecret"
var secretReshare = "sharesecret"

type Node struct {
	base       string
	i          int
	period     string
	publicPath string
	certPath   string
	// certificate key
	keyPath string
	// where all public certs are stored
	certFolder string
	startCmd   *exec.Cmd
	logPath    string
	addr       string
	priv       *key.Pair
	store      key.Store
	cancel     context.CancelFunc
	ctrl       string
	reshared   bool
	tls        bool
	groupPath  string
}

func NewNode(i int, period string, base string, tls bool) *Node {
	nbase := path.Join(base, fmt.Sprintf("node-%d", i))
	os.MkdirAll(nbase, 0740)
	logPath := path.Join(nbase, "log")
	publicPath := path.Join(nbase, "public.toml")
	groupPath := path.Join(nbase, "group.toml")
	os.Remove(logPath)
	n := &Node{
		tls:        tls,
		base:       nbase,
		i:          i,
		logPath:    logPath,
		publicPath: publicPath,
		groupPath:  groupPath,
		period:     period,
	}
	n.setup()
	return n
}

func (n *Node) setup() {
	var err error
	// find a free port
	freePort := test.FreePort()
	iStr := strconv.Itoa(n.i)
	host := "127.0.0." + iStr
	fullAddr := host + ":" + freePort
	n.addr = fullAddr
	ctrlPort := test.FreePort()
	if n.tls {
		// generate certificate
		n.certPath = path.Join(n.base, fmt.Sprintf("server-%d.crt", n.i))
		n.keyPath = path.Join(n.base, fmt.Sprintf("server-%d.key", n.i))
		func() {
			log.SetOutput(new(bytes.Buffer))
			// XXX how to get rid of that annoying creating cert..
			err = httpscerts.Generate(n.certPath, n.keyPath, host)
			if err != nil {
				panic(err)
			}
		}()

	}

	// call drand binary
	n.priv = key.NewKeyPair(fullAddr)
	args := []string{"generate-keypair", "--folder", n.base}
	if !n.tls {
		args = append(args, "--tls-disable")
	}
	args = append(args, fullAddr)
	newKey := exec.Command("drand", args...)
	runCommand(newKey)

	// verify it's done
	n.store = key.NewFileStore(n.base)
	n.priv, err = n.store.LoadKeyPair()
	if n.priv.Public.Address() != fullAddr {
		panic(fmt.Errorf("[-] Private key stored has address %s vs generated %s || base %s", n.priv.Public.Address(), fullAddr, n.base))
	}
	checkErr(key.Save(n.publicPath, n.priv.Public, false))
	n.ctrl = ctrlPort
	checkErr(err)
}

func (n *Node) Start(certFolder string) {
	// create log file
	//logFile, err := os.Create(n.logPath)
	flags := os.O_RDWR | os.O_APPEND | os.O_CREATE
	logFile, err := os.OpenFile(n.logPath, flags, 0777)
	logFile.Write([]byte("\n\nNEW LOG\n\n"))
	checkErr(err)
	var args = []string{"start"}
	args = append(args, pair("--folder", n.base)...)
	args = append(args, pair("--control", n.ctrl)...)
	if n.tls {
		args = append(args, pair("--tls-cert", n.certPath)...)
		args = append(args, pair("--tls-key", n.keyPath)...)
		args = append(args, []string{"--certs-dir", certFolder}...)
	} else {
		args = append(args, "--tls-disable")
	}
	args = append(args, "--verbose")
	ctx, cancel := context.WithCancel(context.Background())
	n.cancel = cancel
	n.certFolder = certFolder
	cmd := exec.CommandContext(ctx, "drand", args...)
	n.startCmd = cmd
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	go func() {
		defer logFile.Close()
		// TODO make the "stop" command returns a graceful error code when
		// stopped
		cmd.Run()
	}()
}

func (n *Node) RunDKG(nodes, thr int, timeout string, leader bool, leaderAddr string) *key.Group {
	args := []string{"share", "--control", n.ctrl}
	args = append(args, pair("--nodes", strconv.Itoa(nodes))...)
	args = append(args, pair("--threshold", strconv.Itoa(thr))...)
	args = append(args, pair("--timeout", timeout)...)
	args = append(args, pair("--period", n.period)...)
	args = append(args, pair("--out", n.groupPath)...)
	args = append(args, pair("--secret", secretDKG)...)
	if leader {
		args = append(args, "--leader")
		// make genesis time offset
		args = append(args, pair("--beacon-delay", strconv.Itoa(beaconOffset))...)
	} else {
		args = append(args, pair("--connect", leaderAddr)...)
		if !n.tls {
			args = append(args, "--tls-disable")
		}
	}
	cmd := exec.Command("drand", args...)
	runCommand(cmd)
	group := new(key.Group)
	checkErr(key.Load(n.groupPath, group))
	return group
}

func (n *Node) GetGroup() *key.Group {
	args := []string{"show", "group", "--control", n.ctrl}
	args = append(args, pair("--out", n.groupPath)...)
	cmd := exec.Command("drand", args...)
	runCommand(cmd)
	group := new(key.Group)
	checkErr(key.Load(n.groupPath, group))
	return group
}

func (n *Node) RunReshare(nodes, thr int, oldGroup string, timeout string, leader bool, leaderAddr string) *key.Group {
	args := []string{"share"}
	args = append(args, pair("--out", n.groupPath)...)
	args = append(args, pair("--control", n.ctrl)...)
	args = append(args, pair("--timeout", timeout)...)
	args = append(args, pair("--nodes", strconv.Itoa(nodes))...)
	args = append(args, pair("--threshold", strconv.Itoa(thr))...)
	args = append(args, pair("--secret", secretReshare)...)
	if n.reshared {
		// only append if we are a new node
		args = append(args, pair("--from", oldGroup)...)
	} else {
		// previous node only need to say it's a transition/resharing
		args = append(args, "--transition")
	}
	if leader {
		args = append(args, "--leader")
		// make transition time offset
		args = append(args, pair("--beacon-delay", strconv.Itoa(beaconOffset))...)
	} else {
		args = append(args, pair("--connect", leaderAddr)...)
		if !n.tls {
			args = append(args, "--tls-disable")
		}
	}
	cmd := exec.Command("drand", args...)
	runCommand(cmd, fmt.Sprintf("drand node %s", n.addr))
	group := new(key.Group)
	checkErr(key.Load(n.groupPath, group))
	return group
}

func (n *Node) GetCokey(group string) bool {
	args := []string{"get", "cokey"}
	if n.tls {
		args = append(args, pair("--tls-cert", n.certPath)...)
	}
	args = append(args, n.addr)

	cmd := exec.Command("drand", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("get cokey %s : %s: err: %v:\n\tout:%s\n", n.addr, args, err, string(out))
		return false
	}
	var r = new(drand.DistKeyResponse)
	err = json.Unmarshal(out, r)
	checkErr(err)
	sdist := hex.EncodeToString(r.Key)
	fmt.Printf("\t- Node %s has cokey %s\n", n.addr, sdist[10:14])
	return true
}

/*func (n *Node) GetGroup() *key.Group {*/
//group, err := n.store.LoadGroup()
//checkErr(err)
//return group
/*}*/

func (n *Node) Ping() bool {
	cmd := exec.Command("drand", "ping", "--control", n.ctrl)
	_, err := cmd.CombinedOutput()
	if err != nil {
		//fmt.Printf("\t- node %s: ping: %v - \n\tout: %s\n", n.addr, err, string(out))
		return false
	}
	return true
}

func (n *Node) GetBeacon(groupPath string, round uint64) (*drand.PublicRandResponse, string) {
	args := []string{"get", "public"}
	if n.tls {
		args = append(args, pair("--tls-cert", n.certPath)...)
	}
	args = append(args, pair("--nodes", n.addr)...)
	args = append(args, pair("--round", strconv.Itoa(int(round)))...)
	args = append(args, groupPath)
	cmd := exec.Command("drand", args...)
	out := runCommand(cmd)
	s := new(drand.PublicRandResponse)
	checkErr(json.Unmarshal(out, s))
	return s, strings.Join(cmd.Args, " ")
}

func (n *Node) WriteCertificate(path string) {
	if n.tls {
		runCommand(exec.Command("cp", n.certPath, path))
	}
}

func (n *Node) WritePublic(path string) {
	checkErr(key.Save(path, n.priv.Public, false))
}

func (n *Node) Stop() {
	if n.cancel != nil {
		n.cancel()
	}
	stopCmd := exec.Command("drand", "stop", "--control", n.ctrl)
	stopCmd.Run()
	if n.startCmd != nil {
		killPid := exec.Command("kill", "-9", strconv.Itoa(n.startCmd.Process.Pid))
		killPid.Run()
	}
	for i := 0; i < 3; i++ {
		if n.Ping() {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		return
	}
	panic("node should have stopped but is still running")
}

func (n *Node) PrintLog() {
	fmt.Printf("[-] Printing logs of node %s:\n", n.addr)
	buff, err := ioutil.ReadFile(n.logPath)
	if err != nil {
		fmt.Printf("[-] Can't read logs !\n\n")
		return
	}
	os.Stdout.Write([]byte(buff))
	fmt.Println()
}

func pair(k, v string) []string {
	return []string{k, v}
}
