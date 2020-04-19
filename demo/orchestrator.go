package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
)

type Orchestrator struct {
	n            int
	thr          int
	newThr       int
	period       string
	periodD      time.Duration
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
	newGroup     *key.Group
	resharePaths []string
	reshareIndex []int
	reshareThr   int
	reshareNodes []*Node
	tls          bool
}

func NewOrchestrator(n int, thr int, period string, tls bool) *Orchestrator {
	basePath := path.Join(os.TempDir(), "drand-full")
	os.RemoveAll(basePath)
	fmt.Printf("[+] Simulation global folder: %s\n", basePath)
	checkErr(os.MkdirAll(basePath, 0740))
	certFolder := path.Join(basePath, "certs")
	checkErr(os.MkdirAll(certFolder, 0740))
	nodes, paths := createNodes(n, 1, period, basePath, certFolder, tls)
	periodD, err := time.ParseDuration(period)
	checkErr(err)
	e := &Orchestrator{
		n:            n,
		thr:          thr,
		basePath:     basePath,
		groupPath:    path.Join(basePath, "group.toml"),
		newGroupPath: path.Join(basePath, "group2.toml"),
		period:       period,
		periodD:      periodD,
		nodes:        nodes,
		paths:        paths,
		certFolder:   certFolder,
		tls:          tls,
	}
	return e
}

func (e *Orchestrator) StartCurrentNodes(toExclude ...int) {
	filtered := filterNodes(e.nodes, toExclude...)
	e.startNodes(filtered)
}

func (e *Orchestrator) StartNewNodes() {
	e.startNodes(e.newNodes)
}

func (e *Orchestrator) startNodes(nodes []*Node) {
	fmt.Printf("[+] Starting all nodes\n")
	for _, node := range nodes {
		fmt.Printf("\t- Starting node %s\n", node.addr)
		node.Start(e.certFolder)
	}
	time.Sleep(1 * time.Second)
	// ping them all
	for {
		var foundAll = true
		for _, node := range nodes {
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

func (e *Orchestrator) RunDKG(timeout string) {
	fmt.Println("[+] Running DKG for all nodes")
	time.Sleep(100 * time.Millisecond)
	leader := e.nodes[0]
	var wg sync.WaitGroup
	wg.Add(len(e.nodes))
	go func() {
		fmt.Printf("\t- Running DKG for leader node %s\n", leader.addr)
		leader.RunDKG(e.n, e.thr, timeout, true, "")
		wg.Done()
	}()
	time.Sleep(200 * time.Millisecond)
	for _, node := range e.nodes[1:] {
		fmt.Printf("\t- Running DKG for node %s\n", node.addr)
		go func(n *Node) {
			n.RunDKG(e.n, e.thr, timeout, false, leader.addr)
			wg.Done()
		}(node)
	}
	wg.Wait()
	fmt.Println("[+] Nodes finished running DKG. Checking keys...")
	// we pass the current group path
	g := e.checkDKGNodes(e.nodes, e.groupPath)
	// overwrite group to group path
	e.group = g
	e.genesis = g.GenesisTime
	checkErr(key.Save(e.groupPath, e.group, false))
	fmt.Println("\t- Overwrite group with distributed key to ", e.groupPath)
}

func (e *Orchestrator) checkDKGNodes(nodes []*Node, groupPath string) *key.Group {
	for {
		fmt.Println("[+] Checking if distributed key is present on all nodes...")
		var allFound = true
		for _, node := range nodes {
			if !node.GetCokey(groupPath) {
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
	for _, node := range nodes {
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
	return g
}

func (e *Orchestrator) WaitGenesis() {
	to := time.Until(time.Unix(e.genesis, 0))
	fmt.Printf("[+] Sleeping %s until genesis happens\n", to)
	time.Sleep(to)
	relax := 3 * time.Second
	fmt.Printf("[+] Sleeping %s after genesis - leaving some time for rounds \n", relax)
	time.Sleep(relax)
}

func (e *Orchestrator) WaitTransition() {
	to := time.Until(time.Unix(e.transition, 0))
	fmt.Printf("[+] Sleeping %s until transition happens\n", to)
	time.Sleep(to)
	fmt.Printf("[+] Sleeping %s after transition - leaving some time for nodes\n", afterPeriodWait)
	time.Sleep(afterPeriodWait)
}

func (e *Orchestrator) Wait(t time.Duration) {
	fmt.Printf("[+] Sleep %ss to leave some time to sync & start again\n", t)
	time.Sleep(t)
}

func (e *Orchestrator) WaitPeriod() {
	nRound, nTime := beacon.NextRound(time.Now().Unix(), e.periodD, e.genesis)
	until := time.Until(time.Unix(nTime, 0).Add(afterPeriodWait))

	fmt.Printf("[+] Sleeping %ds to reach round %d + 3s\n", int(until.Seconds()), nRound)
	time.Sleep(until)
}

func (e *Orchestrator) CheckCurrentBeacon(exclude ...int) {
	filtered := filterNodes(e.nodes, exclude...)
	e.checkBeaconNodes(filtered, e.groupPath)
}

func (e *Orchestrator) CheckNewBeacon(exclude ...int) {
	filtered := filterNodes(e.reshareNodes, exclude...)
	e.checkBeaconNodes(filtered, e.newGroupPath)
}

func filterNodes(list []*Node, exclude ...int) []*Node {
	var filtered []*Node
	for _, n := range list {
		var isExcluded = false
		for _, i := range exclude {
			if i == n.i {
				isExcluded = true
				break
			}
		}
		if !isExcluded {
			filtered = append(filtered, n)
		}
	}
	rand.Shuffle(len(filtered), func(i, j int) {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	})
	return filtered
}

func (e *Orchestrator) checkBeaconNodes(nodes []*Node, group string) {
	nRound, _ := beacon.NextRound(time.Now().Unix(), e.periodD, e.genesis)
	currRound := nRound - 1
	fmt.Printf("[+] Checking randomness beacon for round %d via CLI\n", currRound)
	var rand *drand.PublicRandResponse
	var lastIndex int
	for _, node := range nodes {
		randResp, cmd := node.GetBeacon(group, currRound)
		if rand == nil {
			rand = randResp
			lastIndex = node.i
			fmt.Printf("\t - Example command is: \"%s\"\n", cmd)
		} else {
			if randResp.GetRound() != rand.GetRound() {
				fmt.Println("last index", lastIndex, " vs current index ", node.i)
				fmt.Println(rand.String())
				fmt.Println(randResp.String())
				panic("[-] Inconsistent beacon rounds between nodes")

			} else if !bytes.Equal(randResp.GetSignature(), rand.GetSignature()) {
				panic("[-] Inconsistent beacon signature between nodes")
			}
		}
	}
	fmt.Println("[+] Checking randomness via HTTP API using curl")
	tryCurl := true
	var printed bool
	for _, node := range nodes {
		args := []string{"-k", "-s"}
		http := "http"
		if e.tls {
			args = append(args, pair("--cacert", node.certPath)...)
			http = http + "s"
		}
		args = append(args, pair("-H", "Context-type: application/json")...)
		url := http + "://" + node.addr + "/api/public/"
		// add the round to make sure we don't ask for a later block if we're
		// behind
		url += strconv.Itoa(int(currRound))
		args = append(args, url)
		cmd := exec.Command("curl", args...)
		if !printed {
			fmt.Printf("\t- Example command: \"%s\"\n", strings.Join(cmd.Args, " "))
			printed = true
		}
		if tryCurl {
			// curl returns weird error code
			out, _ := cmd.CombinedOutput()
			out = append(out, []byte("\n")...)
			var r = new(drand.PublicRandResponse)
			checkErr(json.Unmarshal(out, r), string(out))
			if r.GetRound() != rand.GetRound() {
				panic("[-] Inconsistent round from curl vs CLI")
			} else if !bytes.Equal(r.GetSignature(), rand.GetSignature()) {
				panic("[-] Inconsistent signature from curl vs CLI")
			}
		} else {
			fmt.Printf("\t[-] Issue with curl command at the moment\n")
		}
	}
	out, err := json.MarshalIndent(rand, "", "    ")
	checkErr(err)
	fmt.Printf("%s\n", out)
}

func (e *Orchestrator) SetupNewNodes(n int) {
	fmt.Printf("[+] Setting up %d new nodes for resharing\n", n)
	e.newNodes, e.newPaths = createNodes(n, len(e.nodes)+1, e.period, e.basePath, e.certFolder, e.tls)
	for _, node := range e.newNodes {
		// just specify here since we use the short command for old node and new
		// nodes have a longer command - not necessary but this is the
		// main/simplest way of doing it
		node.reshared = true
	}
}

func (e *Orchestrator) CreateResharingGroup(oldToRemove, threshold int) {
	fmt.Println("[+] Setting up the nodes for the resharing")
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
	e.newThr = threshold
	fmt.Printf("[+] Stopping old nodes\n")
	for _, node := range e.nodes {
		var found bool
		for _, idx := range e.reshareIndex {
			if idx == node.i {
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("\t- Stopping old node %s\n", node.addr)
			node.Stop()
		}
	}
}

func (e *Orchestrator) RunResharing(timeout string) {
	fmt.Println("[+] Running DKG for resharing nodes")
	nodes := len(e.reshareNodes)
	thr := e.newThr
	groupCh := make(chan *key.Group, 1)
	leader := e.reshareNodes[0]
	go func() {
		fmt.Printf("\t- Running DKG for leader node %s\n", leader.addr)
		group := leader.RunReshare(nodes, thr, e.groupPath, timeout, true, "")
		groupCh <- group
	}()
	time.Sleep(100 * time.Millisecond)

	for _, node := range e.reshareNodes[1:] {
		fmt.Printf("\t- Running DKG for node %s\n", node.addr)
		go node.RunReshare(nodes, thr, e.groupPath, timeout, false, leader.addr)
	}
	<-groupCh
	// we pass the new group file
	g := e.checkDKGNodes(e.reshareNodes, e.newGroupPath)
	e.newGroup = g
	e.transition = g.TransitionTime
	checkErr(key.Save(e.newGroupPath, e.newGroup, false))
	fmt.Println("\t- Overwrite reshared group with distributed key to ", e.newGroupPath)
	fmt.Println("[+] Check previous distributed key is the same as the new one")
	oldgroup := new(key.Group)
	newgroup := new(key.Group)
	checkErr(key.Load(e.groupPath, oldgroup))
	checkErr(key.Load(e.newGroupPath, newgroup))
	if !oldgroup.PublicKey.Key().Equal(newgroup.PublicKey.Key()) {
		fmt.Printf("[-] Invalid distributed key !\n")
	}
}

func createNodes(n int, offset int, period, basePath, certFolder string, tls bool) ([]*Node, []string) {
	var nodes []*Node
	for i := 0; i < n; i++ {
		idx := i + offset
		n := NewNode(idx, period, basePath, tls)
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

func (e *Orchestrator) StopNodes(idxs ...int) {
	for _, node := range e.nodes {
		for _, idx := range idxs {
			if node.i == idx {
				fmt.Printf("[+] Stopping node %s to simulate a node failure\n", node.addr)
				node.Stop()
			}
		}
	}
}

func (e *Orchestrator) StopAllNodes(toExclude ...int) {
	filtered := filterNodes(e.nodes, toExclude...)
	fmt.Printf("[+] Stopping the rest (%d nodes) for a complete failure\n", len(filtered))
	for _, node := range filtered {
		e.StopNodes(node.i)
	}
}

func (e *Orchestrator) StartNode(idxs ...int) {
	for _, idx := range idxs {
		var foundNode *Node
		for _, node := range e.nodes {
			if node.i == idx {
				foundNode = node
			}
		}
		if foundNode == nil {
			panic("node to start doesn't exist")
		}

		fmt.Printf("[+] Attempting to start node %s again ...\n", foundNode.addr)
		foundNode.Start(e.certFolder)
		trial := 0
		var started bool
		for trial < 5 {
			if foundNode.Ping() {
				fmt.Printf("\t- Node %s started correctly\n", foundNode.addr)
				started = true
				break
			}
			time.Sleep(1 * time.Second)
		}
		if !started {
			panic(fmt.Errorf("[-] Could not start node %s ... \n", foundNode.addr))
		}
	}
}

func (e *Orchestrator) PrintLogs() {
	fmt.Println("[+] Printing logs for debugging on CI")
	for _, node := range e.nodes {
		node.PrintLog()
	}
	for _, node := range e.newNodes {
		node.PrintLog()
	}
}
func (e *Orchestrator) Shutdown() {
	fmt.Println("[+] Shutdown all nodes")
	for _, node := range e.nodes {
		fmt.Printf("\t- Stop old node %s\n", node.addr)
		node.Stop()
	}
	for _, node := range e.newNodes {
		fmt.Printf("\t- Stop new node %s\n", node.addr)
		node.Stop()
	}
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
