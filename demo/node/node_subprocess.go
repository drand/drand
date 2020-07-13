package node

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"net"
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
	json "github.com/nikkolasg/hexjson"
)

var secretDKG = "dkgsecret_____________________32"
var secretReshare = "sharesecret___________________32"

type NodeProc struct {
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
	privAddr   string
	pubAddr    string
	priv       *key.Pair
	store      key.Store
	cancel     context.CancelFunc
	ctrl       string
	tls        bool
	groupPath  string
	binary     string
}

func NewNode(i int, period string, base string, tls bool, binary string) Node {
	nbase := path.Join(base, fmt.Sprintf("node-%d", i))
	os.MkdirAll(nbase, 0740)
	logPath := path.Join(nbase, "log")
	publicPath := path.Join(nbase, "public.toml")
	groupPath := path.Join(nbase, "group.toml")
	os.Remove(logPath)
	n := &NodeProc{
		tls:        tls,
		base:       nbase,
		i:          i,
		logPath:    logPath,
		publicPath: publicPath,
		groupPath:  groupPath,
		period:     period,
		binary:     binary,
	}
	n.setup()
	return n
}

// UpdateBinary updates the binary this node uses for control, to e.g. simulate an upgrade
func (n *NodeProc) UpdateBinary(binary string) {
	n.binary = binary
}

func (n *NodeProc) setup() {
	var err error
	// find a free port
	freePort := test.FreePort()
	freePortREST := test.FreePort()
	iStr := strconv.Itoa(n.i)
	host := "127.0.0." + iStr
	n.privAddr = host + ":" + freePort
	n.pubAddr = host + ":" + freePortREST
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
	n.priv = key.NewKeyPair(n.privAddr)
	args := []string{"generate-keypair", "--folder", n.base}
	if !n.tls {
		args = append(args, "--tls-disable")
	}
	args = append(args, n.privAddr)
	newKey := exec.Command(n.binary, args...)
	runCommand(newKey)

	// verify it's done
	n.store = key.NewFileStore(n.base)
	n.priv, err = n.store.LoadKeyPair()
	if n.priv.Public.Address() != n.privAddr {
		panic(fmt.Errorf("[-] Private key stored has address %s vs generated %s || base %s", n.priv.Public.Address(), n.privAddr, n.base))
	}
	checkErr(key.Save(n.publicPath, n.priv.Public, false))
	n.ctrl = ctrlPort
	checkErr(err)
}

func (n *NodeProc) Start(certFolder string) error {
	// create log file
	//logFile, err := os.Create(n.logPath)
	flags := os.O_RDWR | os.O_APPEND | os.O_CREATE
	logFile, err := os.OpenFile(n.logPath, flags, 0777)
	logFile.Write([]byte("\n\nNEW LOG\n\n"))
	checkErr(err)
	var args = []string{"start"}
	args = append(args, pair("--folder", n.base)...)
	args = append(args, pair("--control", n.ctrl)...)
	_, privPort, _ := net.SplitHostPort(n.privAddr)
	_, pubPort, _ := net.SplitHostPort(n.pubAddr)
	args = append(args, pair("--private-listen", "0.0.0.0:"+privPort)...)
	args = append(args, pair("--public-listen", "0.0.0.0:"+pubPort)...)
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
	cmd := exec.CommandContext(ctx, n.binary, args...)
	n.startCmd = cmd
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	go func() {
		defer logFile.Close()
		// TODO make the "stop" command returns a graceful error code when
		// stopped
		cmd.Run()
	}()
	return nil
}

func (n *NodeProc) PrivateAddr() string {
	return n.privAddr
}

func (n *NodeProc) PublicAddr() string {
	return n.pubAddr
}

func (n *NodeProc) Index() int {
	return n.i
}

func (n *NodeProc) RunDKG(nodes, thr int, timeout string, leader bool, leaderAddr string, beaconOffset int) *key.Group {
	args := []string{"share", "--control", n.ctrl}
	args = append(args, pair("--out", n.groupPath)...)
	args = append(args, pair("--secret", secretDKG)...)
	if leader {
		args = append(args, "--leader")
		args = append(args, pair("--nodes", strconv.Itoa(nodes))...)
		args = append(args, pair("--threshold", strconv.Itoa(thr))...)
		args = append(args, pair("--timeout", timeout)...)
		args = append(args, pair("--period", n.period)...)
		// make genesis time offset
		args = append(args, pair("--beacon-delay", strconv.Itoa(beaconOffset))...)
	} else {
		args = append(args, pair("--connect", leaderAddr)...)
		if !n.tls {
			args = append(args, "--tls-disable")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, n.binary, args...)
	out := runCommand(cmd)
	fmt.Println(n.priv.Public.Address(), "FINISHED DKG", string(out))
	group := new(key.Group)
	checkErr(key.Load(n.groupPath, group))
	fmt.Println(n.priv.Public.Address(), "FINISHED LOADING GROUP")
	return group
}

func (n *NodeProc) GetGroup() *key.Group {
	args := []string{"show", "group", "--control", n.ctrl}
	args = append(args, pair("--out", n.groupPath)...)
	cmd := exec.Command(n.binary, args...)
	runCommand(cmd)
	group := new(key.Group)
	checkErr(key.Load(n.groupPath, group))
	return group
}

func (n *NodeProc) RunReshare(nodes, thr int, oldGroup string, timeout string, leader bool, leaderAddr string, beaconOffset int) *key.Group {
	args := []string{"share"}
	args = append(args, pair("--out", n.groupPath)...)
	args = append(args, pair("--control", n.ctrl)...)
	args = append(args, pair("--secret", secretReshare)...)
	if oldGroup != "" {
		// only append if we are a new node
		args = append(args, pair("--from", oldGroup)...)
	} else {
		// previous node only need to say it's a transition/resharing
		args = append(args, "--transition")
	}
	if leader {
		args = append(args, "--leader")
		args = append(args, pair("--timeout", timeout)...)
		args = append(args, pair("--nodes", strconv.Itoa(nodes))...)
		args = append(args, pair("--threshold", strconv.Itoa(thr))...)
		// make transition time offset
		args = append(args, pair("--beacon-delay", strconv.Itoa(beaconOffset))...)
	} else {
		args = append(args, pair("--connect", leaderAddr)...)
		if !n.tls {
			args = append(args, "--tls-disable")
		}
	}
	cmd := exec.Command(n.binary, args...)
	runCommand(cmd, fmt.Sprintf("drand node %s", n.privAddr))
	group := new(key.Group)
	checkErr(key.Load(n.groupPath, group))
	return group
}

func (n *NodeProc) ChainInfo(group string) bool {
	args := []string{"get", "chain-info"}
	if n.tls {
		args = append(args, pair("--tls-cert", n.certPath)...)
	}
	args = append(args, n.privAddr)

	cmd := exec.Command(n.binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("get chain info %s : %s: err: %v:\n\tout:%s\n", n.privAddr, args, err, string(out))
		return false
	}
	var r = new(drand.ChainInfoPacket)
	err = json.Unmarshal(out, r)
	if err != nil {
		fmt.Printf("err json decoding %s\n", out)
	}
	checkErr(err)
	sdist := hex.EncodeToString(r.PublicKey)
	fmt.Printf("\t- Node %s has chain-info %s\n", n.privAddr, sdist[10:14])
	return true
}

func (n *NodeProc) Ping() bool {
	cmd := exec.Command(n.binary, "util", "ping", "--control", n.ctrl)
	out, err := cmd.CombinedOutput()
	fmt.Printf(" -- ping output : %s - err %s\n", out, err)
	if err != nil {
		//fmt.Printf("\t- node %s: ping: %v - \n\tout: %s\n", n.privAddr, err, string(out))
		return false
	}
	return true
}

func (n *NodeProc) GetBeacon(groupPath string, round uint64) (*drand.PublicRandResponse, string) {
	args := []string{"get", "public"}
	if n.tls {
		args = append(args, pair("--tls-cert", n.certPath)...)
	}
	args = append(args, pair("--nodes", n.privAddr)...)
	args = append(args, pair("--round", strconv.Itoa(int(round)))...)
	args = append(args, groupPath)
	cmd := exec.Command(n.binary, args...)
	out := runCommand(cmd)
	s := new(drand.PublicRandResponse)
	err := json.Unmarshal(out, s)
	if err != nil {
		fmt.Printf("failed to unmarshal beacon response: %s\n", out)
	}
	checkErr(err)
	return s, strings.Join(cmd.Args, " ")
}

func (n *NodeProc) WriteCertificate(path string) {
	if n.tls {
		runCommand(exec.Command("cp", n.certPath, path))
	}
}

func (n *NodeProc) WritePublic(path string) {
	checkErr(key.Save(path, n.priv.Public, false))
}

func (n *NodeProc) Stop() {
	if n.cancel != nil {
		n.cancel()
	}
	stopCmd := exec.Command(n.binary, "stop", "--control", n.ctrl)
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

func (n *NodeProc) PrintLog() {
	fmt.Printf("[-] Printing logs of node %s:\n", n.privAddr)
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

func runCommand(c *exec.Cmd, add ...string) []byte {
	out, err := c.CombinedOutput()
	if err != nil {
		if len(add) > 0 {
			fmt.Printf("[-] Msg failed command: %s\n", add[0])
		}
		fmt.Printf("[-] Command \"%s\" gave\n%s\n", strings.Join(c.Args, " "), string(out))
		panic(err)
	}
	return out
}

func checkErr(err error, out ...string) {
	if err != nil {
		if len(out) > 0 {
			panic(fmt.Errorf("%s: %v", out[0], err))
		} else {
			panic(err)
		}
	}
}
