package lib

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	json "github.com/nikkolasg/hexjson"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/demo/node"
	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
)

// 1s after dkg finishes, (new or reshared) beacon starts
var beaconOffset = 1

// how much should we wait before checking if the randomness is present. This is
// mostly due to the fact we run on localhost on cheap machine with CI so we
// need some delays to make sure *all* nodes that we check have gathered the
// randomness.
var afterPeriodWait = 5 * time.Second

// Orchestrator controls a set of nodes
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
	nodes        []node.Node
	paths        []string
	newNodes     []node.Node
	newPaths     []string
	genesis      int64
	transition   int64
	group        *key.Group
	newGroup     *key.Group
	resharePaths []string
	reshareIndex []int
	reshareThr   int
	reshareNodes []node.Node
	tls          bool
	withCurl     bool
	binary       string
}

func NewOrchestrator(n int, thr int, period string, tls bool, binary string, withCurl bool) *Orchestrator {
	basePath := path.Join(os.TempDir(), "drand-full")
	os.RemoveAll(basePath)
	fmt.Printf("[+] Simulation global folder: %s\n", basePath)
	checkErr(os.MkdirAll(basePath, 0740))
	certFolder := path.Join(basePath, "certs")
	checkErr(os.MkdirAll(certFolder, 0740))
	nodes, paths := createNodes(n, 1, period, basePath, certFolder, tls, binary)
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
		withCurl:     withCurl,
		binary:       binary,
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

func (e *Orchestrator) startNodes(nodes []node.Node) {
	fmt.Printf("[+] Starting all nodes\n")
	for _, node := range nodes {
		fmt.Printf("\t- Starting node %s\n", node.PrivateAddr())
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
	panicCh := make(chan interface{}, 1)
	go func() {
		defer func() {
			if err := recover(); err != nil {
				panicCh <- err
			}
			wg.Done()
		}()
		fmt.Printf("\t- Running DKG for leader node %s\n", leader.PrivateAddr())
		leader.RunDKG(e.n, e.thr, timeout, true, "", beaconOffset)
	}()
	time.Sleep(200 * time.Millisecond)
	for _, n := range e.nodes[1:] {
		fmt.Printf("\t- Running DKG for node %s\n", n.PrivateAddr())
		go func(n node.Node) {
			defer func() {
				if err := recover(); err != nil {
					panicCh <- err
				}
				wg.Done()
			}()
			n.RunDKG(e.n, e.thr, timeout, false, leader.PrivateAddr(), beaconOffset)
			fmt.Println("\t FINISHED DKG")
		}(n)
	}
	wg.Wait()
	select {
	case p := <-panicCh:
		panic(p)
	default:
	}

	fmt.Println("[+] Nodes finished running DKG. Checking keys...")
	// we pass the current group path
	g := e.checkDKGNodes(e.nodes, e.groupPath)
	// overwrite group to group path
	e.group = g
	e.genesis = g.GenesisTime
	checkErr(key.Save(e.groupPath, e.group, false))
	fmt.Println("\t- Overwrite group with distributed key to ", e.groupPath)
}

func (e *Orchestrator) checkDKGNodes(nodes []node.Node, groupPath string) *key.Group {
	for {
		fmt.Println("[+] Checking if chain info is present on all nodes...")
		var allFound = true
		for _, node := range nodes {
			if !node.ChainInfo(groupPath) {
				allFound = false
				break
			}
		}
		if !allFound {
			fmt.Println("[+] Chain info not present on all nodes. Sleeping 3s...")
			time.Sleep(3 * time.Second)
		} else {
			fmt.Println("[+] Chain info are present on all nodes. DKG finished.")
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
			lastNode = node.PrivateAddr()
			continue
		}
		if !g.PublicKey.Equal(group.PublicKey) {
			panic(fmt.Errorf("- Node %s has different cokey than %s\n", node.PrivateAddr(), lastNode))
		}
	}
	return g
}

func (e *Orchestrator) WaitGenesis() {
	to := time.Until(time.Unix(e.genesis, 0))
	fmt.Printf("[+] Sleeping %d until genesis happens\n", int(to.Seconds()))
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
	nRound, nTime := chain.NextRound(time.Now().Unix(), e.periodD, e.genesis)
	until := time.Until(time.Unix(nTime, 0).Add(afterPeriodWait))

	fmt.Printf("[+] Sleeping %ds to reach round %d + 3s\n", int(until.Seconds()), nRound)
	time.Sleep(until)
}

func (e *Orchestrator) CheckCurrentBeacon(exclude ...int) {
	filtered := filterNodes(e.nodes, exclude...)
	e.checkBeaconNodes(filtered, e.groupPath, e.withCurl)
}

func (e *Orchestrator) CheckNewBeacon(exclude ...int) {
	filtered := filterNodes(e.reshareNodes, exclude...)
	e.checkBeaconNodes(filtered, e.newGroupPath, e.withCurl)
}

func filterNodes(list []node.Node, exclude ...int) []node.Node {
	var filtered []node.Node
	for _, n := range list {
		var isExcluded = false
		for _, i := range exclude {
			if i == n.Index() {
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

func (e *Orchestrator) checkBeaconNodes(nodes []node.Node, group string, tryCurl bool) {
	nRound, _ := chain.NextRound(time.Now().Unix(), e.periodD, e.genesis)
	currRound := nRound - 1
	fmt.Printf("[+] Checking randomness beacon for round %d via CLI\n", currRound)
	var rand *drand.PublicRandResponse
	var lastIndex int
	for _, node := range nodes {
		randResp, cmd := node.GetBeacon(group, currRound)
		if rand == nil {
			rand = randResp
			lastIndex = node.Index()
			fmt.Printf("\t - Example command is: \"%s\"\n", cmd)
		} else {
			if randResp.GetRound() != rand.GetRound() {
				fmt.Println("last index", lastIndex, " vs current index ", node.Index())
				fmt.Println(rand.String())
				fmt.Println(randResp.String())
				panic("[-] Inconsistent beacon rounds between nodes")

			} else if !bytes.Equal(randResp.GetSignature(), rand.GetSignature()) {
				panic("[-] Inconsistent beacon signature between nodes")
			}
		}
	}
	fmt.Println("[+] Checking randomness via HTTP API using curl")
	var printed bool
	for _, n := range nodes {
		args := []string{"-k", "-s"}
		http := "http"
		if e.tls {
			tmp, _ := ioutil.TempFile("", "cert")
			defer os.Remove(tmp.Name())
			tmp.Close()
			n.WriteCertificate(tmp.Name())
			args = append(args, pair("--cacert", tmp.Name())...)
			http = http + "s"
		}
		args = append(args, pair("-H", "Context-type: application/json")...)
		url := http + "://" + n.PublicAddr() + "/public/"
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
				fmt.Printf("curl output: %s\n", out)
				fmt.Printf("curl output rand: %x\n", r.GetSignature())
				fmt.Printf("cli output: %s\n", rand)
				fmt.Printf("cli output rand: %x\n", rand.GetSignature())
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
	e.newNodes, e.newPaths = createNodes(n, len(e.nodes)+1, e.period, e.basePath, e.certFolder, e.tls, e.binary)
}

// UpdateBinary will either set the 'bianry' to use for the node at 'idx', or on the orchestrator as
// a whole if idx is negative.
func (e *Orchestrator) UpdateBinary(binary string, idx int) {
	if idx < 0 {
		e.binary = binary
	} else {
		n := e.nodes[idx]
		if spn, ok := n.(*node.NodeProc); ok {
			spn.UpdateBinary(binary)
		}
	}
}

func (e *Orchestrator) CreateResharingGroup(oldToRemove, threshold int) {
	fmt.Println("[+] Setting up the nodes for the resharing")
	// create paths that contains old node + new nodes
	for _, node := range e.nodes[oldToRemove:] {
		fmt.Printf("\t- Adding current node %s\n", node.PrivateAddr())
		e.reshareIndex = append(e.reshareIndex, node.Index())
		e.reshareNodes = append(e.reshareNodes, node)
	}
	for _, node := range e.newNodes {
		fmt.Printf("\t- Adding new node %s\n", node.PrivateAddr())
		e.reshareIndex = append(e.reshareIndex, node.Index())
		e.reshareNodes = append(e.reshareNodes, node)
	}
	e.resharePaths = append(e.resharePaths, e.paths[oldToRemove:]...)
	e.resharePaths = append(e.resharePaths, e.newPaths...)
	e.newThr = threshold
	fmt.Printf("[+] Stopping old nodes\n")
	for _, node := range e.nodes {
		var found bool
		for _, idx := range e.reshareIndex {
			if idx == node.Index() {
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("\t- Stopping old node %s\n", node.PrivateAddr())
			node.Stop()
		}
	}
}

func (e *Orchestrator) isNew(n node.Node) bool {
	for _, c := range e.newNodes {
		if c == n {
			return true
		}
	}
	return false
}

func (e *Orchestrator) RunResharing(timeout string) {
	fmt.Println("[+] Running DKG for resharing nodes")
	nodes := len(e.reshareNodes)
	thr := e.newThr
	groupCh := make(chan *key.Group, 1)
	leader := e.reshareNodes[0]
	panicCh := make(chan interface{}, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer func() {
			if err := recover(); err != nil {
				panicCh <- err
			}
		}()
		path := ""
		if e.isNew(leader) {
			path = e.groupPath
		}
		fmt.Printf("\t- Running DKG for leader node %s\n", leader.PrivateAddr())
		group := leader.RunReshare(nodes, thr, path, timeout, true, "", beaconOffset)
		fmt.Printf("\t- Resharing DONE for leader node %s\n", leader.PrivateAddr())
		wg.Done()
		groupCh <- group
	}()
	time.Sleep(100 * time.Millisecond)

	for _, n := range e.reshareNodes[1:] {
		path := ""
		if e.isNew(n) {
			path = e.groupPath
		}
		fmt.Printf("\t- Running DKG for node %s\n", n.PrivateAddr())
		wg.Add(1)
		go func(n node.Node) {
			defer func() {
				if err := recover(); err != nil {
					wg.Done()
					panicCh <- err
				}
			}()
			n.RunReshare(nodes, thr, path, timeout, false, leader.PrivateAddr(), beaconOffset)
			fmt.Printf("\t- Resharing DONE for node %s\n", n.PrivateAddr())
			wg.Done()
		}(n)
	}
	wg.Wait()
	<-groupCh
	select {
	case p := <-panicCh:
		panic(p)
	default:
	}
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

func createNodes(n int, offset int, period, basePath, certFolder string, tls bool, binary string) ([]node.Node, []string) {
	var nodes []node.Node
	for i := 0; i < n; i++ {
		idx := i + offset
		var n node.Node
		if binary != "" {
			n = node.NewNode(idx, period, basePath, tls, binary)
		} else {
			n = node.NewLocalNode(idx, period, basePath, tls, "127.0.0.1")
		}
		n.WriteCertificate(path.Join(certFolder, fmt.Sprintf("cert-%d", idx)))
		nodes = append(nodes, n)
		fmt.Printf("\t- Created node %s at %s\n", n.PrivateAddr(), basePath)
	}
	// write public keys from all nodes
	var paths []string
	for _, node := range nodes {
		path := path.Join(basePath, fmt.Sprintf("public-%d.toml", node.Index()))
		node.WritePublic(path)
		paths = append(paths, path)
	}
	return nodes, paths
}

func (e *Orchestrator) StopNodes(idxs ...int) {
	for _, node := range e.nodes {
		for _, idx := range idxs {
			if node.Index() == idx {
				fmt.Printf("[+] Stopping node %s to simulate a node failure\n", node.PrivateAddr())
				node.Stop()
			}
		}
	}
}

func (e *Orchestrator) StopAllNodes(toExclude ...int) {
	filtered := filterNodes(e.nodes, toExclude...)
	fmt.Printf("[+] Stopping the rest (%d nodes) for a complete failure\n", len(filtered))
	for _, node := range filtered {
		e.StopNodes(node.Index())
	}
}

func (e *Orchestrator) StartNode(idxs ...int) {
	for _, idx := range idxs {
		var foundNode node.Node
		for _, node := range append(e.nodes, e.newNodes...) {
			if node.Index() == idx {
				foundNode = node
			}
		}
		if foundNode == nil {
			panic("node to start doesn't exist")
		}

		fmt.Printf("[+] Attempting to start node %s again ...\n", foundNode.PrivateAddr())
		foundNode.Start(e.certFolder)
		trial := 0
		var started bool
		for trial < 5 {
			if foundNode.Ping() {
				fmt.Printf("\t- Node %s started correctly\n", foundNode.PrivateAddr())
				started = true
				break
			}
			time.Sleep(1 * time.Second)
		}
		if !started {
			panic(fmt.Errorf("[-] Could not start node %s ... \n", foundNode.PrivateAddr()))
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
		fmt.Printf("\t- Stop old node %s\n", node.PrivateAddr())
		node.Stop()
	}
	for _, node := range e.newNodes {
		fmt.Printf("\t- Stop new node %s\n", node.PrivateAddr())
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

func pair(k, v string) []string {
	return []string{k, v}
}
