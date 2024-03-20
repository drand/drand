package node

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/kabukky/httpscerts"
	json "github.com/nikkolasg/hexjson"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/core"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/demo/cfg"
	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"
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
	certFolder  string
	startCmd    *exec.Cmd
	logPath     string
	privAddr    string
	pubAddr     string
	priv        *key.Pair
	store       key.Store
	cancel      context.CancelFunc
	ctrl        string
	isCandidate bool
	tls         bool
	groupPath   string
	binary      string
	scheme      *crypto.Scheme
	beaconID    string

	dbEngineType chain.StorageType
	pgDSN        string
	memDBSize    int
}

func NewNode(i int, cfg cfg.Config) *NodeProc {
	nbase := path.Join(cfg.BasePath, fmt.Sprintf("node-%d", i))
	os.MkdirAll(nbase, 0740)
	logPath := path.Join(nbase, "log")
	publicPath := path.Join(nbase, "public.toml")
	groupPath := path.Join(nbase, "group.toml")
	os.Remove(logPath)
	n := &NodeProc{
		tls:          cfg.WithTLS,
		base:         nbase,
		i:            i,
		logPath:      logPath,
		publicPath:   publicPath,
		groupPath:    groupPath,
		period:       cfg.Period,
		scheme:       cfg.Scheme,
		binary:       cfg.Binary,
		beaconID:     cfg.BeaconID,
		isCandidate:  cfg.IsCandidate,
		dbEngineType: cfg.DBEngineType,
		pgDSN:        cfg.PgDSN(),
		memDBSize:    cfg.MemDBSize,
	}
	n.setup()
	return n
}

// UpdateBinary updates the binary this node uses for control, to e.g. simulate an upgrade
func (n *NodeProc) UpdateBinary(binary string, isCandidate bool) {
	n.binary = binary
	n.isCandidate = isCandidate
}

func (n *NodeProc) setup() {
	var err error
	// find a free port
	freePort := test.FreePort()
	freePortREST := test.FreePort()
	host := "127.0.0.1"
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
	n.priv, err = key.NewKeyPair(n.privAddr, nil)
	if err != nil {
		panic(err)
	}

	args := []string{"generate-keypair", "--folder", n.base, "--id", n.beaconID, "--scheme", n.scheme.Name}

	if !n.tls {
		args = append(args, "--tls-disable")
	}
	args = append(args, n.privAddr)
	newKey := exec.Command(n.binary, args...)
	runCommand(newKey)

	config := core.NewConfig(core.WithConfigFolder(n.base))
	n.store = key.NewFileStore(config.ConfigFolderMB(), n.beaconID)

	// verify it's done
	n.priv, err = n.store.LoadKeyPair(nil)
	if n.priv.Public.Address() != n.privAddr {
		panic(fmt.Errorf("[-] Private key stored has address %s vs generated %s || base %s", n.priv.Public.Address(), n.privAddr, n.base))
	}
	checkErr(key.Save(n.publicPath, n.priv.Public, false))
	n.ctrl = ctrlPort
	checkErr(err)
}

func (n *NodeProc) Start(certFolder string, dbEngineType chain.StorageType, pgDSN func() string, memDBSize int) error {
	if dbEngineType != "" {
		n.dbEngineType = dbEngineType
	}
	if pgDSN != nil {
		n.pgDSN = pgDSN()
	}
	if memDBSize != 0 {
		n.memDBSize = memDBSize
	}

	// create log file
	// logFile, err := os.Create(n.logPath)
	flags := os.O_RDWR | os.O_APPEND | os.O_CREATE
	logFile, err := os.OpenFile(n.logPath, flags, 0777)
	checkErr(err)
	_, _ = logFile.Write([]byte("\n\nNEW LOG\n\n"))

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
	args = append(args, pair("--db", string(n.dbEngineType))...)
	args = append(args, pair("--pg-dsn", n.pgDSN)...)
	args = append(args, pair("--memdb-size", fmt.Sprintf("%d", n.memDBSize))...)
	args = append(args, "--verbose")

	fmt.Printf("starting node %s with cmd: %s \n", n.privAddr, args)

	ctx, cancel := context.WithCancel(context.Background())
	n.cancel = cancel
	n.certFolder = certFolder

	cmd := exec.CommandContext(ctx, n.binary, args...)
	n.startCmd = cmd
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	go func() {
		defer func() {
			_ = logFile.Close()
		}()
		// TODO make the "stop" command returns a graceful error code when stopped
		cmd.Run()
	}()
	return nil
}

func (n *NodeProc) PrivateAddr() string {
	return n.privAddr
}

func (n *NodeProc) CtrlAddr() string {
	return n.ctrl
}

func (n *NodeProc) PublicAddr() string {
	return n.pubAddr
}

func (n *NodeProc) Index() int {
	return n.i
}

func (n *NodeProc) RunDKG(nodes, thr int, timeout time.Duration, leader bool, leaderAddr string, beaconOffset int) (*key.Group, error) {
	args := []string{"share", "--control", n.ctrl}
	args = append(args, pair("--out", n.groupPath)...)
	if leader {
		args = append(args, "--leader")
		args = append(args, pair("--nodes", strconv.Itoa(nodes))...)
		args = append(args, pair("--threshold", strconv.Itoa(thr))...)
		args = append(args, pair("--timeout", timeout.String())...)
		args = append(args, pair("--period", n.period)...)
		args = append(args, pair("--scheme", n.scheme.Name)...)

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
	cmd.Env = append(os.Environ(), "DRAND_SHARE_SECRET="+secretDKG)
	out := runCommand(cmd)
	fmt.Println(n.priv.Public.Address(), "FINISHED DKG", string(out))
	group := new(key.Group)
	err := key.Load(n.groupPath, group)
	if err != nil {
		return nil, err
	}
	fmt.Println(n.priv.Public.Address(), "FINISHED LOADING GROUP")
	return group, nil
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
	cmd.Env = append(os.Environ(), "DRAND_SHARE_SECRET="+secretReshare)
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
	if err != nil {
		fmt.Printf(" -- ping output : %s - err %s\n", out, err)
	} else {
		fmt.Printf(" -- ping output : %s\n", out)
	}
	return err == nil
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
	buff, err := os.ReadFile(n.logPath)
	if err != nil {
		fmt.Printf("[-] Can't read logs at %s !\n\n", n.logPath)
		return
	}

	fmt.Printf("%s\n", string(buff))
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
	if err == nil {
		return
	}

	if len(out) > 0 {
		panic(fmt.Errorf("%s: %v", out[0], err))
	}

	panic(err)
}
