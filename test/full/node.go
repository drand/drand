package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"
	"github.com/kabukky/httpscerts"
)

type Node struct {
	base     string
	i        int
	certPath string
	// certificate key
	keyPath string
	// where all public certs are stored
	certFolder string

	logPath  string
	addr     string
	priv     *key.Pair
	store    key.Store
	cancel   context.CancelFunc
	ctrl     string
	reshared bool
}

func NewNode(i int, base string) *Node {
	nbase := path.Join(base, fmt.Sprintf("node-%d", i))
	os.MkdirAll(nbase, 0740)
	logPath := path.Join(nbase, "log")
	n := &Node{
		base:    nbase,
		i:       i,
		logPath: logPath,
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

	// call drand binary
	n.priv = key.NewKeyPair(fullAddr)
	newKey := exec.Command("drand", "generate-keypair", "--folder", n.base, fullAddr)
	runCommand(newKey)

	// verify it's done
	n.store = key.NewFileStore(n.base)
	n.priv, err = n.store.LoadKeyPair()
	if n.priv.Public.Address() != fullAddr {
		panic(fmt.Errorf("[-] Private key stored has address %s vs generated %s || base %s", n.priv.Public.Address(), fullAddr, n.base))
	}
	n.ctrl = ctrlPort
	checkErr(err)
}

func (n *Node) Start(certFolder string) {
	// create log file
	logFile, err := os.Create(n.logPath)
	checkErr(err)
	var args = []string{"start"}
	args = append(args, pair("--folder", n.base)...)
	args = append(args, pair("--control", n.ctrl)...)
	args = append(args, pair("--tls-cert", n.certPath)...)
	args = append(args, pair("--tls-key", n.keyPath)...)
	args = append(args, []string{"--certs-dir", certFolder}...)
	args = append(args, "--verbose")
	ctx, cancel := context.WithCancel(context.Background())
	n.cancel = cancel
	n.certFolder = certFolder
	cmd := exec.CommandContext(ctx, "drand", args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	go func() {
		defer logFile.Close()
		// TODO make the "stop" command returns a graceful error code when
		// stopped
		cmd.Run()
	}()
}

func (n *Node) RunDKG(group string, timeout string, leader bool) {
	args := []string{"share", "--control", n.ctrl, "--timeout", timeout}
	args = append(args, pair("--folder", n.base)...)
	if leader {
		args = append(args, "--leader")
	}
	args = append(args, group)
	cmd := exec.Command("drand", args...)
	runCommand(cmd)
}

func (n *Node) RunReshare(oldGroup string, newGroup string, timeout string, leader bool) {
	args := []string{"share"}
	args = append(args, pair("--folder", n.base)...)
	args = append(args, pair("--control", n.ctrl)...)
	args = append(args, pair("--timeout", timeout)...)
	if n.reshared {
		args = append(args, pair("--from", oldGroup)...)
	}
	if leader {
		args = append(args, "--leader")
	}
	args = append(args, newGroup)
	cmd := exec.Command("drand", args...)
	runCommand(cmd, fmt.Sprintf("drand node %s", n.addr))
}

func (n *Node) GetCokey(group string) bool {
	args := []string{"get", "cokey"}
	args = append(args, pair("--tls-cert", n.certPath)...)
	args = append(args, pair("--nodes", n.addr)...)
	args = append(args, group)
	cmd := exec.Command("drand", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("get cokey%s err: %v:\n\tout:%s\n", n.addr, err, string(out))
		return false
	}
	var r = new(drand.DistKeyResponse)
	err = json.Unmarshal(out, r)
	checkErr(err)
	sdist := hex.EncodeToString(r.Key)
	fmt.Printf("\t- Node %s has cokey %s\n", n.addr, sdist[10:14])
	return true
}

func (n *Node) GetGroup() *key.Group {
	group, err := n.store.LoadGroup()
	checkErr(err)
	return group
}

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
	args := []string{"get", "public", "--tls-cert", n.certPath}
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
	runCommand(exec.Command("cp", n.certPath, path))
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
}

func pair(k, v string) []string {
	return []string{k, v}
}
