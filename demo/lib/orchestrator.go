package lib

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	json "github.com/nikkolasg/hexjson"

	"github.com/drand/drand/common"
	"github.com/drand/drand/common/key"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/demo/cfg"
	"github.com/drand/drand/demo/node"
	"github.com/drand/drand/internal/chain"
	"github.com/drand/drand/protobuf/drand"
)

// how much should we wait before checking if the randomness is present. This is
// mostly due to the fact we run on localhost on cheap machine with CI so we
// need some delays to make sure *all* nodes that we check have gathered the
// randomness.
var afterPeriodWait = 1 * time.Second

// Orchestrator controls a set of nodes
type Orchestrator struct {
	n                 int
	thr               int
	newThr            int
	beaconID          string
	period            string
	scheme            *crypto.Scheme
	periodD           time.Duration
	basePath          string
	groupPath         string
	newGroupPath      string
	nodes             []node.Node
	paths             []string
	newNodes          []node.Node
	newPaths          []string
	genesis           int64
	transition        int64
	group             *key.Group
	newGroup          *key.Group
	reshareNodes      []node.Node
	withCurl          bool
	isBinaryCandidate bool
	binary            string
	dbEngineType      chain.StorageType
	pgDSN             func() string
	memDBSize         int
}

func NewOrchestrator(c cfg.Config) *Orchestrator {
	c.BasePath = path.Join(os.TempDir(), "drand-full")
	// cleanup the basePath before doing anything
	_ = os.RemoveAll(c.BasePath)

	fmt.Printf("[+] Simulation global folder: %s\n", c.BasePath)
	checkErr(os.MkdirAll(c.BasePath, 0o740))
	c.BeaconID = common.GetCanonicalBeaconID(c.BeaconID)
	nodes, paths := createNodes(c)

	periodD, err := time.ParseDuration(c.Period)
	checkErr(err)
	e := &Orchestrator{
		n:                 c.N,
		thr:               c.Thr,
		scheme:            c.Scheme,
		basePath:          c.BasePath,
		groupPath:         path.Join(c.BasePath, "group.toml"),
		newGroupPath:      path.Join(c.BasePath, "group2.toml"),
		period:            c.Period,
		periodD:           periodD,
		nodes:             nodes,
		paths:             paths,
		withCurl:          c.WithCurl,
		binary:            c.Binary,
		isBinaryCandidate: c.IsCandidate,
		beaconID:          common.GetCanonicalBeaconID(c.BeaconID),
		dbEngineType:      c.DBEngineType,
		pgDSN:             c.PgDSN,
		memDBSize:         c.MemDBSize,
	}
	return e
}

func (e *Orchestrator) StartCurrentNodes(toExclude ...int) error {
	filtered := filterNodes(e.nodes, toExclude...)
	return e.startNodes(filtered)
}

func (e *Orchestrator) StartNewNodes() error {
	return e.startNodes(e.newNodes)
}

func (e *Orchestrator) startNodes(nodes []node.Node) error {
	fmt.Printf("[+] Starting all nodes\n")
	for _, n := range nodes {
		fmt.Printf("\t- Starting node %s\n", n.PrivateAddr())
		err := n.Start(e.dbEngineType, e.pgDSN, e.memDBSize)
		if err != nil {
			return err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// ping them all
	for {
		select {
		case <-ticker.C:
			var foundAll = true
			for _, n := range nodes {
				if !n.Ping() {
					foundAll = false
					break
				}
			}

			if !foundAll {
				fmt.Println("[-] can not ping them all. Sleeping 2s...")
				break
			}
			return nil
		case <-ctx.Done():
			fmt.Println("[-] can not ping all nodes in 30 seconds. Shutting down.")
			panic("failed to ping nodes in 30 seconds")
		}
	}
}
func (e *Orchestrator) RunDKG(timeout time.Duration) error {
	fmt.Println("[+] Running DKG for all nodes")
	leader := e.nodes[0]

	fmt.Printf("\t- Running DKG for leader node %s\n", leader.PrivateAddr())
	joiners := make([]*drand.Participant, len(e.nodes))
	for i, n := range e.nodes {
		identity, err := n.Identity()
		if err != nil {
			return fmt.Errorf("n.Identity: %w for %s", err, n.PrivateAddr())
		}
		joiners[i] = identity
	}

	catchupPeriod := 0
	err := leader.StartLeaderDKG(e.thr, catchupPeriod, joiners)
	if err != nil {
		return fmt.Errorf("leader.StartLeaderDKG: %w", err)
	}

	for _, n := range e.nodes[1:] {
		n := n
		fmt.Printf("\t- Joining DKG for node %s\n", n.PrivateAddr())
		err = n.JoinDKG()
		if err != nil {
			return fmt.Errorf("n.JoinDKG: %w for %s", err, n.PrivateAddr())
		}
	}

	err = leader.ExecuteLeaderDKG()
	if err != nil {
		return fmt.Errorf("leader.ExecuteLeaderDKG: %w", err)
	}

	fmt.Println("[+] Waiting for DKG completion")
	_, err = leader.WaitDKGComplete(1, timeout)
	if err != nil {
		return fmt.Errorf("leader.WaitDKGComplete: %w", err)
	}

	fmt.Println("[+] Nodes finished running DKG. Checking keys...")
	// we pass the current group path
	g := e.checkDKGNodes(e.nodes, e.groupPath)
	// overwrite group to group path
	e.group = g
	e.genesis = g.GenesisTime
	checkErr(key.Save(e.groupPath, e.group, false))
	fmt.Println("\t- Overwrite group with distributed key to ", e.groupPath)
	return nil
}

func (e *Orchestrator) checkDKGNodes(nodes []node.Node, groupPath string) *key.Group {
	for {
		fmt.Println("[+] Checking if chain info is present on all nodes...")
		var allFound = true
		for _, n := range nodes {
			if !n.ChainInfo(groupPath) {
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
	for _, n := range nodes {
		group := n.GetGroup()
		if g == nil {
			g = group
			lastNode = n.PrivateAddr()
			continue
		}
		if !g.PublicKey.Equal(group.PublicKey) {
			panic(fmt.Errorf("[-] Node %s has different cokey than %s", n.PrivateAddr(), lastNode))
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
	currentRound := common.CurrentRound(e.transition, e.periodD, e.genesis)

	fmt.Printf("[+] Sleeping %s until transition happens (transition time: %d) currentRound: %d current time: %d\n", to, e.transition, currentRound, time.Now().Unix())
	time.Sleep(to)
	fmt.Printf("[+] Sleeping %s after transition - leaving some time for nodes\n", afterPeriodWait)
	time.Sleep(afterPeriodWait)
}

func (e *Orchestrator) Wait(t time.Duration) {
	fmt.Printf("[+] Sleep %ss to leave some time to sync & start again\n", t)
	time.Sleep(t)
}

func (e *Orchestrator) WaitPeriod() {
	nRound, nTime := common.NextRound(time.Now().Unix(), e.periodD, e.genesis)
	until := time.Until(time.Unix(nTime, 0).Add(afterPeriodWait))

	fmt.Printf("[+] Sleeping %ds to reach round %d [period %f, current %d]\n", int(until.Seconds()), nRound, e.periodD.Seconds(), nRound)
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
	nRound, _ := common.NextRound(time.Now().Unix(), e.periodD, e.genesis)
	currRound := nRound - 1
	fmt.Printf("[+] Checking randomness beacon for round %d via CLI\n", currRound)
	var pubRand *drand.PublicRandResponse
	var lastIndex int
	for _, n := range nodes {
		fmt.Println("\t[-] Trying node", n.PrivateAddr())
		const maxTrials = 3
		for i := 0; i < maxTrials; i++ {
			fmt.Println("\t\t[-] attempt", i+1)

			randResp, cmd := n.GetBeacon(group, currRound)
			if pubRand == nil {
				pubRand = randResp
				lastIndex = n.Index()
				fmt.Printf("\t - Example command is: %q\n", cmd)
				break
			}

			// we first check both are at the same round
			if randResp.GetRound() != pubRand.GetRound() {
				fmt.Println("\t\t[-] Mismatch between last index", lastIndex, " vs current index ", n.Index(), " - trying again in some time...")
				time.Sleep(100 * time.Millisecond)
				// we try again
				continue
			}
			// then we check if the signatures match
			if !bytes.Equal(randResp.GetSignature(), pubRand.GetSignature()) {
				panic("\t\t[-] Inconsistent beacon signature between nodes")
			}
			// everything is good
			fmt.Println("\t\t[-] attempt", i+1, "SUCCESS")
			break
		}
	}

	fmt.Println("[+] Checking randomness via HTTP API using curl")
	var printed bool
	for _, n := range nodes {
		fmt.Println("\t[-] Trying node", n.PrivateAddr())
		args := []string{"-k", "-s"}
		args = append(args, pair("-H", "Context-type: application/json")...)
		url := "http://" + n.PublicAddr() + "/public/"
		// add the round to make sure we don't ask for a later block if we're
		// behind
		url += strconv.Itoa(int(currRound))
		args = append(args, url)

		const maxCurlRetries = 10
		for i := 0; i < maxCurlRetries; i++ {
			cmd := exec.Command("curl", args...)
			if !printed {
				fmt.Printf("\t\t- Example command: %q\n", strings.Join(cmd.Args, " "))
				printed = true
			}
			if tryCurl {
				// curl returns weird error code
				out, _ := cmd.CombinedOutput()
				if len(out) == 0 {
					fmt.Println("received empty response from curl. Retrying ...")
					time.Sleep(afterPeriodWait)
					continue
				}

				out = append(out, []byte("\n")...)
				var r = new(drand.PublicRandResponse)
				checkErr(json.Unmarshal(out, r), string(out))
				if r.GetRound() != pubRand.GetRound() {
					panic("[-] Inconsistent round from curl vs CLI")
				} else if !bytes.Equal(r.GetSignature(), pubRand.GetSignature()) {
					fmt.Printf("curl output: %s\n", out)
					if !strings.Contains(string(out), "round") ||
						!strings.Contains(string(out), "randomness") ||
						!strings.Contains(string(out), "signature") {
						panic("curl output is incorrect!")
					}
					fmt.Printf("curl output rand: %x\n", r.GetSignature())
					fmt.Printf("cli output: %s\n", pubRand)
					fmt.Printf("cli output rand: %x\n", pubRand.GetSignature())
					panic("\t[-] Inconsistent signature from curl vs CLI")
				}
			} else {
				fmt.Printf("\t[-] Issue with curl command at the moment\n")
			}
			break
		}
	}
	out, err := json.MarshalIndent(pubRand, "", "    ")
	checkErr(err)
	fmt.Printf("%s\n", out)
}

func (e *Orchestrator) SetupNewNodes(n int) {
	fmt.Printf("[+] Setting up %d new nodes for resharing\n", n)
	c := cfg.Config{
		N:            n,
		Offset:       len(e.nodes) + 1,
		Period:       e.period,
		BasePath:     e.basePath,
		Binary:       e.binary,
		Scheme:       e.scheme,
		BeaconID:     e.beaconID,
		IsCandidate:  e.isBinaryCandidate,
		DBEngineType: e.dbEngineType,
		PgDSN:        e.pgDSN,
		MemDBSize:    e.memDBSize,
	}
	//  offset int, period, basePath, certFolder string, tls bool, binary string, sch scheme.Scheme, beaconID string, isCandidate bool
	e.newNodes, e.newPaths = createNodes(c)
}

// UpdateBinary will set the 'binary' to use for the node at 'idx'
func (e *Orchestrator) UpdateBinary(binary string, idx uint, isCandidate bool) {
	n := e.nodes[idx]
	if spn, ok := n.(*node.NodeProc); ok {
		spn.UpdateBinary(binary, isCandidate)
	}
}

// UpdateGlobalBinary will set the 'binary' to use on the orchestrator as a whole
func (e *Orchestrator) UpdateGlobalBinary(binary string, isCandidate bool) {
	e.binary = binary
	e.isBinaryCandidate = isCandidate
}

type ResharingGroup struct {
	leaving   []*drand.Participant
	joining   []*drand.Participant
	remaining []*drand.Participant
}

func (e *Orchestrator) CreateResharingGroup(oldToRemove, threshold int) (*ResharingGroup, error) {
	resharingGroup := ResharingGroup{}
	fmt.Println("[+] Setting up the nodes for the resharing")
	// create paths that contains old node + new nodes
	for _, n := range e.nodes[oldToRemove:] {
		fmt.Printf("\t- Adding current node %s\n", n.PrivateAddr())
		p, err := n.Identity()
		if err != nil {
			return nil, err
		}

		resharingGroup.remaining = append(resharingGroup.remaining, p)
		e.reshareNodes = append(e.reshareNodes, n)
	}

	for _, n := range e.newNodes {
		p, err := n.Identity()
		if err != nil {
			return nil, err
		}
		resharingGroup.joining = append(resharingGroup.joining, p)
		e.reshareNodes = append(e.reshareNodes, n)
	}

	e.newThr = threshold
	fmt.Printf("[+] Stopping old nodes\n")
	for _, n := range e.nodes {
		var found bool
		for _, resharer := range append(e.nodes[oldToRemove:], e.newNodes...) {
			if resharer == n {
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("\t- Stopping old node %s\n", n.PrivateAddr())
			p, err := n.Identity()
			if err != nil {
				return nil, err
			}
			resharingGroup.leaving = append(resharingGroup.leaving, p)
			n.Stop()
		}
	}

	return &resharingGroup, nil
}

func (e *Orchestrator) isNew(n node.Node) bool {
	for _, c := range e.newNodes {
		if c == n {
			return true
		}
	}
	return false
}

func (e *Orchestrator) RunResharing(resharingGroup *ResharingGroup, timeout time.Duration) {
	fmt.Println("[+] Running DKG for resharing nodes")
	leader := e.reshareNodes[0]

	// if the transition time is in the past, the DKG will fail, so it needs to be long enough to complete the DKG
	roundInOneMinute := common.CurrentRound(time.Now().Add(1*time.Minute).Unix(), e.periodD, e.genesis)
	transitionTime := common.TimeOfRound(e.periodD, e.genesis, roundInOneMinute)
	catchupPeriod := 0
	err := leader.StartLeaderReshare(e.newThr, time.Unix(transitionTime, 0), catchupPeriod, resharingGroup.joining, resharingGroup.remaining, resharingGroup.leaving)
	if err != nil {
		panic(err)
	}

	oldGroup := *leader.GetGroup()
	for _, n := range e.newNodes {
		n := n
		fmt.Printf("\t- Joining DKG for node %s\n", n.PrivateAddr())
		err = n.JoinReshare(oldGroup)
		if err != nil {
			panic(err)
		}
		fmt.Printf("\t- Joined DKG for node %s\n", n.PrivateAddr())
	}

	for _, n := range except(e.reshareNodes[1:], e.newNodes) {
		n := n
		fmt.Printf("\t- Accepting DKG for node %s\n", n.PrivateAddr())
		err = n.AcceptReshare()
		if err != nil {
			panic(err)
		}
		fmt.Printf("\t- Accepted DKG for node %s\n", n.PrivateAddr())
	}

	err = leader.ExecuteLeaderReshare()
	if err != nil {
		panic(err)
	}

	_, err = leader.WaitDKGComplete(2, timeout)
	if err != nil {
		panic(err)
	}
	fmt.Printf("\t- Resharing DONE for leader node %s\n", leader.PrivateAddr())

	// let's give the other nodes a little time to settle and finish their DKGs
	time.Sleep(2 * time.Second)

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

func createNodes(cfg cfg.Config) ([]node.Node, []string) {
	var nodes []node.Node
	for i := 0; i < cfg.N; i++ {
		idx := i + cfg.Offset
		var n node.Node
		if cfg.Binary != "" {
			n = node.NewNode(idx, cfg)
		} else {
			n = node.NewLocalNode(idx, "127.0.0.1", cfg)
		}
		nodes = append(nodes, n)
		fmt.Printf("\t- Created node %s at %s --> ctrl port: %s\n", n.PrivateAddr(), cfg.BasePath, n.CtrlAddr())
	}
	// write public keys from all nodes
	var paths []string
	for _, nd := range nodes {
		p := path.Join(cfg.BasePath, fmt.Sprintf("public-%d.toml", nd.Index()))
		nd.WritePublic(p)
		paths = append(paths, p)
	}
	return nodes, paths
}

func (e *Orchestrator) StopNodes(idxs ...int) {
	for _, n := range e.nodes {
		for _, idx := range idxs {
			if n.Index() == idx {
				fmt.Printf("[+] Stopping node %s to simulate a node failure\n", n.PrivateAddr())
				n.Stop()
			}
		}
	}
}

func (e *Orchestrator) StopAllNodes(toExclude ...int) {
	filtered := filterNodes(e.nodes, toExclude...)
	fmt.Printf("[+] Stopping the rest (%d nodes) for a complete failure\n", len(filtered))
	for _, n := range filtered {
		e.StopNodes(n.Index())
	}
}

func (e *Orchestrator) StartNode(idxs ...int) {
	for _, idx := range idxs {
		var foundNode node.Node
		for _, n := range append(e.nodes, e.newNodes...) {
			if n.Index() == idx {
				foundNode = n
			}
		}
		if foundNode == nil {
			panic("node to start doesn't exist")
		}

		fmt.Printf("[+] Attempting to start node %s again ...\n", foundNode.PrivateAddr())
		// Here we send the nil values to the start method to allow the node to reconnect to the same database
		err := foundNode.Start("", nil, e.memDBSize)
		if err != nil {
			panic(fmt.Errorf("[-] Could not start node %s error: %v", foundNode.PrivateAddr(), err))
		}
		var started bool
		for trial := 1; trial < 10; trial++ {
			if foundNode.Ping() {
				fmt.Printf("\t- Node %s started correctly\n", foundNode.PrivateAddr())
				started = true
				break
			}
			time.Sleep(time.Duration(trial*trial) * time.Second)
		}
		if !started {
			panic(fmt.Errorf("[-] Could not start node %s", foundNode.PrivateAddr()))
		}
	}
}

func (e *Orchestrator) PrintLogs() {
	fmt.Println("[+] Printing logs for debugging on CI")
	for _, n := range e.nodes {
		n.PrintLog()
	}
	for _, n := range e.newNodes {
		n.PrintLog()
	}
}

func (e *Orchestrator) Shutdown() {
	fmt.Println("[+] Shutdown all nodes")
	for _, no := range e.nodes {
		fmt.Printf("\t- Stopping old node %s\n", no.PrivateAddr())
		go no.Stop()
	}
	for _, no := range e.newNodes {
		fmt.Printf("\t- Stopping new node %s\n", no.PrivateAddr())
		go no.Stop()
		fmt.Println("\t- Successfully stopped Node", no.Index(), "(", no.PrivateAddr(), ")")
	}
	fmt.Println("\t- Successfully sent Stop command to all node")
	time.Sleep(20 * time.Second)
	fmt.Println("\t- Wait done. Exiting.")
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

func pair(k, v string) []string {
	return []string{k, v}
}

// returns an array containing all the nodes in the first array except if they appear in the second
func except(arr []node.Node, arr2 []node.Node) []node.Node {
	var out []node.Node

	for _, n := range arr {
		found := false
		for _, n2 := range arr2 {
			if n == n2 {
				found = true
				break
			}
		}
		if !found {
			out = append(out, n)
		}
	}
	return out
}
