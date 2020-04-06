package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
)

type Orchestrator struct {
	n            int
	thr          int
	period       string
	basePath     string
	groupPath    string
	newGroupPath string
	certFolder   string
	nodes        []*Node
	paths        []string
	newNodes     []*Node
	newPaths     []string
	genesis      int64
	transition   int64
	group        *key.Group
	resharePaths []string
	reshareIndex []int
	reshareThr   int
	reshareNodes []*Node
}

func NewOrchestrator(n int, thr int, period string) *Orchestrator {
	basePath := path.Join(os.TempDir(), "drand-full")
	os.RemoveAll(basePath)
	fmt.Printf("[+] Simulation global folder: %s\n", basePath)
	checkErr(os.MkdirAll(basePath, 0740))
	certFolder := path.Join(basePath, "certs")
	checkErr(os.MkdirAll(certFolder, 0740))
	nodes, paths := createNodes(n, 1, basePath, certFolder)
	e := &Orchestrator{
		n:          n,
		thr:        thr,
		basePath:   basePath,
		groupPath:  path.Join(basePath, "group.toml"),
		period:     period,
		nodes:      nodes,
		paths:      paths,
		certFolder: certFolder,
	}
	return e
}

func (e *Orchestrator) CreateGroup(genesis int64) {
	e.genesis = genesis
	// call drand to create the group file
	args := []string{"group", "--out", e.groupPath}
	args = append(args, "--period", e.period)
	args = append(args, "--genesis", strconv.Itoa(int(e.genesis)))
	args = append(args, e.paths...)
	newGroup := exec.Command("drand", args...)
	runCommand(newGroup)
	// load group
	_, err := ioutil.ReadFile(e.groupPath)
	checkErr(err)
	fmt.Printf("[+] Group file stored at %s\n", e.groupPath)
}

func (e *Orchestrator) StartAll() {
	fmt.Printf("[+] Starting all nodes\n")
	for _, node := range e.nodes {
		fmt.Printf("\t- Starting node %s\n", node.addr)
		node.Start(e.certFolder)
	}
	time.Sleep(1 * time.Second)
	// ping them all
	for {
		var foundAll = true
		for _, node := range e.nodes {
			if !node.Ping() {
				foundAll = false
				break
			}
		}
		if !foundAll {
			fmt.Println("[-] can not ping them all. Sleeping 2s...")
			time.Sleep(2 * time.Second)
		} else {
			break
		}
	}
}

func (e *Orchestrator) CheckGroup() {
	args := []string{"check-group"}
	args = append(args, pair("--certs-dir", e.certFolder)...)
	args = append(args, e.groupPath)
	cmd := exec.Command("drand", args...)
	runCommand(cmd)
}

func (e *Orchestrator) RunDKG(timeout string) {
	fmt.Println("[+] Running DKG for all nodes")
	for _, node := range e.nodes[1:] {
		fmt.Printf("\t- Running DKG for node %s\n", node.addr)
		go node.RunDKG(e.groupPath, timeout, false)
	}
	leader := e.nodes[0]
	fmt.Printf("\t- Running DKG for leader node %s\n", leader.addr)
	leader.RunDKG(e.groupPath, timeout, true)
	e.checkDKGNodes(e.nodes)
}

func (e *Orchestrator) checkDKGNodes(nodes []*Node) {
	for {
		fmt.Println("[+] Checking if distributed key is present on all nodes...")
		var allFound = true
		for _, node := range e.nodes {
			if !node.GetCokey(e.groupPath) {
				allFound = false
				break
			}
		}
		if !allFound {
			fmt.Println("[+] cokey not present on all nodes. Sleeping 3s...")
			time.Sleep(3 * time.Second)
		} else {
			fmt.Println("[+] Distributed key are present on all nodes. DKG finished.")
			break
		}
	}

	var g *key.Group
	var lastNode string
	fmt.Println("[+] Checking all created group file with collective key")
	for _, node := range e.nodes {
		group := node.GetGroup()
		if g == nil {
			g = group
			lastNode = node.addr
			continue
		}
		if !g.PublicKey.Equal(group.PublicKey) {
			panic(fmt.Errorf("- Node %s has different cokey than %s\n", node.addr, lastNode))
		}
	}
	// overwrite group to group path
	e.group = g
	checkErr(key.Save(e.groupPath, e.group, false))
	fmt.Println("\t- Overwritten group with distributed key to ", e.groupPath)
}

func (e *Orchestrator) WaitGenesis() {
	to := time.Until(time.Unix(e.genesis, 0))
	fmt.Printf("[+] Sleeping %s until genesis happens\n", to)
	time.Sleep(to)
	fmt.Printf("[+] Sleeping 2s after genesis\n")
	time.Sleep(2 * time.Second)
}

func (e *Orchestrator) WaitPeriod() {
	fmt.Printf("[+] Sleeping %s - a full period\n", e.period)
	d, err := time.ParseDuration(e.period)
	checkErr(err)
	time.Sleep(d)
}

func (e *Orchestrator) CheckBeacon() {
	fmt.Println("[+] Checking randomness beacon for all nodes via CLI")
	var rand *drand.PublicRandResponse
	for _, node := range e.nodes {
		randResp, cmd := node.GetLastBeacon(e.groupPath)
		if rand == nil {
			rand = randResp
			fmt.Printf("\t - Example command is: \"%s\"\n", cmd)
		} else {
			if randResp.GetRound() != rand.GetRound() {
				panic("[-] Inconsistent beacon rounds between nodes")

			} else if !bytes.Equal(randResp.GetSignature(), rand.GetSignature()) {
				panic("[-] Inconsistent beacon signature between nodes")
			}
		}
	}
	fmt.Println("[+] Checking randomness via HTTP API using curl")
	if true {
		fmt.Println("\t- Avoiding REST api with gRPC - JSON API - issue")
	} else {
		var printed bool
		for _, node := range e.nodes {
			args := []string{"-k", "-s"}
			args = append(args, pair("--cacert", node.certPath)...)
			args = append(args, pair("-H", "\"Context-type: application/json\"")...)
			args = append(args, "https://"+node.addr+"/api/public")
			cmd := exec.Command("curl", args...)
			if !printed {
				fmt.Printf("\t- Example command: \"%s\"\n", strings.Join(cmd.Args, " "))
			}
			// curl returns weird error code
			out, err := cmd.CombinedOutput()
			var r = new(drand.PublicRandResponse)
			fmt.Println(" YOUHOU", string(out), err)
			checkErr(json.Unmarshal(out, r))
			if r.GetRound() != rand.GetRound() {
				panic("[-] Inconsistent round from curl vs CLI")
			} else if !bytes.Equal(r.GetSignature(), rand.GetSignature()) {
				panic("[-] Inconsistent signature from curl vs CLI")
			}
		}
	}
	out, err := json.MarshalIndent(rand, "", "    ")
	checkErr(err)
	fmt.Printf("%s\n", out)
}

func (e *Orchestrator) SetupNewNodes(n int) {
	fmt.Printf("[+] Setting up %d new nodes for resharing\n", n)
	e.newNodes, e.newPaths = createNodes(n, len(e.nodes)+1, e.basePath, e.certFolder)
	for _, node := range e.newNodes {
		// just specify here since we use the short command for old node and new
		// nodes have a longer command - not necessary but this is the
		// main/simplest way of doing it
		node.reshared = true
	}
}

func (e *Orchestrator) CreateResharingGroup(oldToRemove, threshold int, transitionTime int64) {
	fmt.Println("[+] Creating new resharing group")
	// create paths that contains old node + new nodes
	for _, node := range e.nodes[oldToRemove:] {
		fmt.Printf("\t- Adding current node %s\n", node.addr)
		e.reshareIndex = append(e.reshareIndex, node.i)
		e.reshareNodes = append(e.reshareNodes, node)
	}
	for _, node := range e.newNodes {
		fmt.Printf("\t- Adding new node %s\n", node.addr)
		e.reshareIndex = append(e.reshareIndex, node.i)
		e.reshareNodes = append(e.reshareNodes, node)
	}
	e.resharePaths = append(e.resharePaths, e.paths[oldToRemove:]...)
	e.resharePaths = append(e.resharePaths, e.newPaths...)

	e.transition = transitionTime
	e.reshareThr = threshold
	e.newGroupPath = path.Join(e.basePath, "new_group.toml")
	args := []string{"group", "--out", e.newGroupPath}
	// specifiy the previous group file
	args = append(args, pair("--from", e.groupPath)...)
	args = append(args, pair("--threshold", strconv.Itoa(e.reshareThr))...)
	args = append(args, pair("--transition", strconv.Itoa(int(e.transition)))...)
	args = append(args, e.resharePaths...)
	newGroup := exec.Command("drand", args...)
	runCommand(newGroup)
	// load group
	_, err := ioutil.ReadFile(e.newGroupPath)
	checkErr(err)
	fmt.Printf("[+] Group file stored at %s\n", e.newGroupPath)
}

func (e *Orchestrator) RunResharing(timeout string) {
	fmt.Println("[+] Running DKG for resharing nodes")
	for _, node := range e.reshareNodes[1:] {
		fmt.Printf("\t- Running DKG for node %s\n", node.addr)
		go node.RunDKG(e.newGroupPath, timeout, false)
	}
	leader := e.reshareNodes[0]
	fmt.Printf("\t- Running DKG for leader node %s\n", leader.addr)
	leader.RunDKG(e.newGroupPath, timeout, true)
	e.checkDKGNodes(e.nodes)

}

func createNodes(n int, offset int, basePath, certFolder string) ([]*Node, []string) {
	var nodes []*Node
	for i := 0; i < n; i++ {
		idx := i + offset
		n := NewNode(idx, basePath)
		n.WriteCertificate(path.Join(certFolder, fmt.Sprintf("cert-%d", idx)))
		nodes = append(nodes, n)
		fmt.Printf("\t- Created node %s at %s\n", n.addr, n.base)
	}
	// write public keys from all nodes
	var paths []string
	for _, node := range nodes {
		path := path.Join(basePath, fmt.Sprintf("public-%d.toml", node.i))
		node.WritePublic(path)
		paths = append(paths, path)
	}
	return nodes, paths
}

func runCommand(c *exec.Cmd) []byte {
	out, err := c.CombinedOutput()
	if err != nil {
		fmt.Printf("[-] Command \"%s\" gave\n%s\n", strings.Join(c.Args, " "), string(out))
		panic(err)
	}
	return out
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}
