package core

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	gnet "net"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"

	//"github.com/drand/kyber"
	clock "github.com/jonboulle/clockwork"
	"github.com/kabukky/httpscerts"
	"github.com/stretchr/testify/require"
)

var testBeaconOffset = int((5 * time.Second).Seconds())
var testDkgTimeout = 1 * time.Second

func TestDrandDKGFresh(t *testing.T) {
	n := 4
	beaconPeriod := 1 * time.Second
	//var offsetGenesis = 1 * time.Second
	//genesis := clock.NewFakeClock().Now().Add(offsetGenesis).Unix()
	dt := NewDrandTest2(t, n, key.DefaultThreshold(n), beaconPeriod)
	defer dt.Cleanup()
	finalGroup := dt.RunDKG()
	time.Sleep(getSleepDuration())
	fmt.Println(" --- DKG FINISHED ---")
	// make the last node fail
	lastID := dt.nodes[n-1].addr
	dt.StopDrand(lastID, false)
	//lastOne.Stop()
	fmt.Printf("\n--- lastOne STOPPED --- \n\n")

	// move time to genesis
	//dt.MoveTime(offsetGenesis)
	now := dt.Now().Unix()
	beaconStart := finalGroup.GenesisTime
	diff := beaconStart - now
	dt.MoveTime(time.Duration(diff) * time.Second)
	// two = genesis + 1st round (happens at genesis)
	fmt.Println(" --- Test BEACON LENGTH --- ")
	dt.TestBeaconLength(2, false, dt.Ids(n-1, false)...)
	fmt.Println(" --- START LAST DRAND ---")
	// start last one
	dt.StartDrand(lastID, true, false)
	// leave some room to do the catchup
	time.Sleep(100 * time.Millisecond)
	fmt.Println(" --- STARTED BEACON DRAND ---")
	dt.MoveTime(beaconPeriod)
	dt.TestBeaconLength(3, false, dt.Ids(n, false)...)
	dt.TestPublicBeacon(lastID, false)
}

func TestDrandDKGReshareTimeout(t *testing.T) {
	oldN := 3
	newN := 4
	oldThr := 2
	newThr := 3
	timeout := 1 * time.Second
	beaconPeriod := 2 * time.Second
	offline := 1

	dt := NewDrandTest2(t, oldN, oldThr, beaconPeriod)
	defer dt.Cleanup()
	group1 := dt.RunDKG()
	// make sure all nodes had enough time to run their go routines to start the
	// beacon handler - related to CI problems
	time.Sleep(getSleepDuration())
	dt.MoveToTime(group1.GenesisTime)
	// move to genesis time - so nodes start to make a round
	//dt.MoveTime(offsetGenesis)
	// two = genesis + 1st round (happens at genesis)
	dt.TestBeaconLength(2, false, dt.Ids(oldN, false)...)
	// so nodes think they are going forward with round 2
	dt.MoveTime(1 * time.Second)

	// + offline makes sure t
	toKeep := oldN - offline
	toAdd := newN - toKeep
	dt.SetupNewNodes(toAdd)

	fmt.Println("SETUP RESHARE DONE")
	// run the resharing
	var doneReshare = make(chan *key.Group)
	go func() {
		group := dt.RunReshare(toKeep, toAdd, newThr, timeout)
		doneReshare <- group
	}()
	time.Sleep(3 * time.Second)
	fmt.Printf("\n -- Move to Response phase !! -- \n")
	dt.MoveTime(timeout)
	// at this point in time, nodes should have gotten all deals and send back
	// their responses to all nodes
	time.Sleep(getSleepDuration())
	fmt.Printf("\n -- Move to Justif phase !! -- \n")
	dt.MoveTime(timeout)
	// at this time, all nodes received the responses of each other nodes but
	// there is one node missing so they expect justifications
	time.Sleep(getSleepDuration())
	fmt.Printf("\n -- Move to Finish phase !! -- \n")
	dt.MoveTime(timeout)
	time.Sleep(getSleepDuration())
	// at this time they received no justification from the missing node so he's
	// exlucded of the group and the dkg should finish
	//time.Sleep(10 * time.Second)
	var resharedGroup *key.Group
	select {
	case resharedGroup = <-doneReshare:
	case <-time.After(1 * time.Second):
		require.True(t, false)
	}
	fmt.Println(" RESHARED GROUP:", resharedGroup)
	target := resharedGroup.TransitionTime
	now := dt.Now().Unix()
	// get rounds from first node in the "old" group - since he's the leader for
	// the new group, he's alive
	lastBeacon := dt.TestPublicBeacon(dt.Ids(1, false)[0], false)
	// move to the transition time period by period - do not skip potential
	// periods as to emulate the normal time behavior
	for now < target {
		dt.MoveTime(beaconPeriod)
		lastBeacon = dt.TestPublicBeacon(dt.Ids(1, false)[0], false)
		now = dt.Now().Unix()
	}
	// move to the transition time
	dt.MoveToTime(resharedGroup.TransitionTime)
	time.Sleep(getSleepDuration())
	//fmt.Println(" --- AFTER RESHARED ROUND  SLEEEPING ---")
	dt.TestBeaconLength(int(lastBeacon.Round+1), true, dt.Ids(newN, true)...)
}

type Node struct {
	addr  string
	drand *Drand
	clock clock.FakeClock
}

type DrandTest2 struct {
	sync.Mutex
	t *testing.T
	// tmp dir for certificates, keys etc
	dir          string
	newDir       string
	certPaths    []string
	newCertPaths []string
	// global clock on which all drand's clock are synchronized
	clock clock.FakeClock

	n      int
	thr    int
	newN   int
	newThr int
	period time.Duration
	// only set after the DKG
	group *key.Group
	// needed to give the group to new nodes during a resharing - only set after
	// a successfull DKG
	groupPath string
	// only set after the resharing
	newGroup *key.Group
	// nodes that are created for running a first DKG
	nodes []*Node
	// new additional nodes that are created for running a resharing
	newNodes []*Node
	// nodes that actually ran the resharing phase - it's a combination of nodes
	// and new nodes. These are the one that should appear in the newGroup
	resharedNodes []*Node
}

// NewDrandTest creates a drand test scenario with initial n nodes and ready to
// run a DKG for the given threshold that will then launch the beacon with the
// specified period
func NewDrandTest2(t *testing.T, n, thr int, period time.Duration) *DrandTest2 {
	dt := new(DrandTest2)
	drands, _, dir, certPaths := BatchNewDrand(n, false,
		WithCallOption(grpc.FailFast(true)),
	)
	dt.t = t
	dt.dir = dir
	dt.certPaths = certPaths
	dt.n = n
	dt.thr = thr
	dt.period = period
	dt.clock = clock.NewFakeClock()
	dt.nodes = make([]*Node, 0, n)
	for _, drand := range drands {
		node := dt.newNode(drand)
		dt.nodes = append(dt.nodes, node)
	}
	dt.groupPath = path.Join(dt.dir, "group.toml")
	return dt
}

// Ids returns the list of the first n ids given the newGroup parameter (either
// in the original group or the reshared one)
func (d *DrandTest2) Ids(n int, newGroup bool) []string {
	nodes := d.nodes
	if newGroup {
		nodes = d.resharedNodes
	}
	var ids []string
	for _, node := range nodes[:n] {
		ids = append(ids, node.addr)
	}
	return ids
}

// RunDKG runs the DKG with the initial nodes created during NewDrandTest
func (d *DrandTest2) RunDKG() *key.Group {
	// common secret between participants
	secret := "thisisdkg"
	root := d.nodes[0]
	controlClient, err := net.NewControlClient(root.drand.opts.controlPort)
	require.NoError(d.t, err)
	// the root node will return the group over this channel
	var wg sync.WaitGroup
	wg.Add(d.n)
	// first run the leader and then run the other nodes
	go func() {
		_, err := controlClient.InitDKGLeader(d.n, d.thr, d.period, testDkgTimeout, nil, secret, testBeaconOffset)
		require.NoError(d.t, err)
		fmt.Printf("\n\nTEST LEADER FINISHED\n\n")
		wg.Done()
	}()

	// make sure the leader is up and running to start the setup
	time.Sleep(1 * time.Second)
	// all other nodes will send their PK to the leader that will create the
	// group
	for _, node := range d.nodes[1:] {
		go func(n *Node) {
			client, err := net.NewControlClient(n.drand.opts.controlPort)
			require.NoError(d.t, err)
			_, err = client.InitDKG(root.drand.priv.Public, d.n, d.thr, testDkgTimeout, nil, secret)
			fmt.Printf("\n\nTEST NONLEADER FINISHED\n\n")
			require.NoError(d.t, err)
			wg.Done()
			fmt.Printf("\n\n\n TESTDKG NON-ROOT %s FINISHED\n\n\n", n.addr)
		}(node)
	}

	// wait for all to return
	wg.Wait()
	fmt.Printf("\n\n\n TESTDKG ROOT %s FINISHED\n\n\n", root.addr)
	// we check that we can fetch the group using control functionalities on the root node
	groupProto, err := controlClient.GroupFile()
	require.NoError(d.t, err)
	group, err := key.GroupFromProto(groupProto)
	require.NoError(d.t, err)
	// we check all nodes are included in the group
	for _, node := range d.nodes {
		require.NotNil(d.t, group.Find(node.drand.priv.Public))
	}
	// we check the group has the right threshold
	require.Len(d.t, group.PublicKey.Coefficients, d.thr)
	require.Equal(d.t, d.thr, group.Threshold)
	require.NoError(d.t, key.Save(d.groupPath, group, false))
	d.group = group
	fmt.Println("setup dkg group:", d.group.String())
	return group
}

func (d *DrandTest2) Cleanup() {
	os.RemoveAll(d.dir)
	os.RemoveAll(d.newDir)
}

// GetBeacon returns the beacon of the given round for the specified drand id
func (d *DrandTest2) GetBeacon(id string, round int, newGroup bool) (*beacon.Beacon, error) {
	nodes := d.nodes
	if newGroup {
		nodes = d.resharedNodes
	}
	for _, node := range nodes {
		if node.addr != id {
			continue
		}
		return node.drand.beacon.Store().Get(uint64(round))
	}
	return nil, errors.New("that should not happen")
}

// GetDrand returns the node associated with this address, either in the new
// group or the current group.
func (d *DrandTest2) GetDrand(id string, newGroup bool) *Node {
	nodes := d.nodes
	if newGroup {
		nodes = d.resharedNodes
	}
	for _, node := range nodes {
		if node.addr == id {
			return node
		}
	}
	panic("no nodes found at this id")
}

// StopDrand stops a node from the first group
func (d *DrandTest2) StopDrand(id string, newGroup bool) {
	node := d.GetDrand(id, newGroup)
	dr := node.drand
	dr.Stop()
	pinger, err := net.NewControlClient(dr.opts.controlPort)
	require.NoError(d.t, err)
	var counter = 1
	fmt.Println(" DRAND ", dr.priv.Public.Address(), " TRYING TO PING")
	for range time.Tick(100 * time.Millisecond) {
		if err := pinger.Ping(); err != nil {
			fmt.Println(" DRAND ", dr.priv.Public.Address(), " TRYING TO PING DONE")
			break
		}
		counter++
		require.LessOrEqual(d.t, counter, 5)
	}
	fmt.Println(" DRAND ", dr.priv.Public.Address(), " STOPPED")
}

// StartDrand fetches the drand given the id, in the respective group given the
// newGroup parameter and runs the beacon
func (d *DrandTest2) StartDrand(id string, catchup bool, newGroup bool) {
	node := d.GetDrand(id, newGroup)
	dr := node.drand
	// we load it from scratch as if the operator restarted its node
	newDrand, err := LoadDrand(dr.store, dr.opts)
	require.NoError(d.t, err)
	node.drand = newDrand
	newDrand.opts.clock = node.clock
	fmt.Println("--- JUST BEFORE STARTBEACON---")
	newDrand.StartBeacon(catchup)
	fmt.Println("--- JUST AFTER STARTBEACON---")
}

func (d *DrandTest2) Now() time.Time {
	return d.clock.Now()
}

// MoveToTime sets the clock of all drands to the designated unix timestamp in
// seconds
func (d *DrandTest2) MoveToTime(target int64) {
	now := d.Now().Unix()
	if now < target {
		d.MoveTime(time.Duration(target-now) * time.Second)
	}
}

// MoveTime advance the clock of all drand of the given duration
func (d *DrandTest2) MoveTime(p time.Duration) {
	for _, node := range d.nodes {
		node.clock.Advance(p)
	}
	for _, node := range d.newNodes {
		node.clock.Advance(p)
	}
	d.clock.Advance(p)
	fmt.Printf(" --- MoveTime: new time is %d \n", d.Now().Unix())
	time.Sleep(getSleepDuration())
}

// TestBeaconLength looks if the beacon chain on the given ids is of the
// expected length
func (d *DrandTest2) TestBeaconLength(length int, newGroup bool, ids ...string) {
	nodes := d.nodes
	if newGroup {
		nodes = d.resharedNodes
	}
	for _, id := range ids {
		for _, node := range nodes {
			if node.addr != id {
				continue
			}
			drand := node.drand
			drand.state.Lock()
			defer drand.state.Unlock()
			fmt.Printf("\n\tTest %s (beacon %p)\n", id, drand.beacon)
			howMany := 0
			drand.beacon.Store().Cursor(func(c beacon.Cursor) {
				for b := c.First(); b != nil; b = c.Next() {
					howMany++
					fmt.Printf("\t %d - %s: beacon %s\n", drand.index, drand.priv.Public.Address(), b)
				}
			})
			require.Equal(d.t, length, drand.beacon.Store().Len(), "id %s - howMany is %d vs Len() %d", id, howMany, drand.beacon.Store().Len())

		}
	}
}

// TestPublicBeacon looks if we can get the latest beacon on this node
func (d *DrandTest2) TestPublicBeacon(id string, newGroup bool) *drand.PublicRandResponse {
	node := d.GetDrand(id, newGroup)
	dr := node.drand
	client := net.NewGrpcClientFromCertManager(dr.opts.certmanager, dr.opts.grpcOpts...)
	resp, err := client.PublicRand(context.TODO(), test.NewTLSPeer(dr.priv.Public.Addr), &drand.PublicRandRequest{})
	require.NoError(d.t, err)
	require.NotNil(d.t, resp)
	return resp
}

// SetupNewNodes creates new additional nodes that can participate during the
// resharing
func (d *DrandTest2) SetupNewNodes(newNodes int) {
	newDrands, _, newDir, newCertPaths := BatchNewDrand(newNodes, false,
		WithCallOption(grpc.FailFast(true)), WithLogLevel(log.LogDebug))
	d.newCertPaths = newCertPaths
	d.newDir = newDir
	d.newNodes = make([]*Node, newNodes)
	// add certificates of new nodes to the old nodes
	for _, node := range d.nodes {
		drand := node.drand
		for _, cp := range newCertPaths {
			drand.opts.certmanager.Add(cp)
		}

	}
	// store new part. and add certificate path of current nodes to the new
	d.newNodes = make([]*Node, 0, newNodes)
	for _, drand := range newDrands {
		node := d.newNode(drand)
		d.newNodes = append(d.newNodes, node)
		for _, cp := range d.certPaths {
			drand.opts.certmanager.Add(cp)
		}
	}
}

// RunReshare runs the resharing procedure with only "oldRun" current nodes
// running, and "newRun" new nodes running (the ones created via SetupNewNodes).
// It sets the given threshold to the group.
// It stops the nodes excluded first.
func (d *DrandTest2) RunReshare(oldRun, newRun, newThr int, timeout time.Duration) *key.Group {
	fmt.Printf(" -- Running RESHARE with %d/%d old, %d/%d new nodes\n", oldRun, len(d.nodes), newRun, len(d.newNodes))
	var clientCounter = new(sync.WaitGroup)
	var secret = "thisistheresharing"
	total := oldRun + newRun
	// stop the exluded nodes
	for _, node := range d.nodes[oldRun:] {
		fmt.Printf("-- Running RESHARE - STOPPING old node %s - %s\n", node.addr, node.drand.priv.Public.Key)
		d.StopDrand(node.addr, false)
	}
	for _, node := range d.newNodes[newRun:] {
		fmt.Printf("-- Running RESHARE - STOPPING new node %s - %s\n", node.addr, node.drand.priv.Public.Key)
		d.StopDrand(node.addr, true)
	}

	d.newN = total
	d.newThr = newThr
	leader := d.nodes[0]
	// function that each non-leader runs to start the resharing
	runreshare := func(n *Node, newNode bool) {
		dr := n.drand
		// instruct to be ready for a reshare
		client, err := net.NewControlClient(dr.opts.controlPort)
		require.NoError(d.t, err)
		_, err = client.InitReshare(leader.drand.priv.Public, total, d.newThr, timeout, secret, d.groupPath)
		require.NoError(d.t, err, "non-leader node (new?%v) error during reshare ", newNode)
		fmt.Printf("\n\nRESHARING TEST: non-leader drand %s DONE RESHARING - %s\n", dr.priv.Public.Address(), dr.priv.Public.Key)
		clientCounter.Done()
	}

	clientCounter.Add(1)
	groupCh := make(chan *key.Group, 1)
	// first run the leader, then the other nodes will send their PK to the
	// leader and then the leader will answer back with the new group
	go func() {
		oldNode := d.group.Find(leader.drand.priv.Public)
		if oldNode == nil {
			panic("leader not found in old group")
		}
		fmt.Printf("\n\nRESHARING TEST: LAUNCH (old) root %d - %s - %s \n", oldNode.Index, leader.addr, leader.drand.priv.Public.Key)
		client, err := net.NewControlClient(leader.drand.opts.controlPort)
		require.NoError(d.t, err)
		finalGroup, err := client.InitReshareLeader(d.newN, d.newThr, timeout, secret, "", testBeaconOffset)
		fmt.Printf("\n\nRESHARING TEST: LEADER root DONE RESHARING %d - %s - %s \n", oldNode.Index, leader.addr, leader.drand.priv.Public.Key)
		if err != nil {
			panic(err)
		}
		clientCounter.Done()
		fg, err := key.GroupFromProto(finalGroup)
		if err != nil {
			panic(err)
		}
		fmt.Printf("\n\nRESHARING TEST: LEADER root DONE RESHARING ###2222 %d - %s - %s \n", oldNode.Index, leader.addr, leader.drand.priv.Public.Key)
		groupCh <- fg
	}()
	d.resharedNodes = append(d.resharedNodes, leader)
	// leave some time to make sure leader is listening
	time.Sleep(1 * time.Second)

	// run the current nodes
	for _, node := range d.nodes[1:oldRun] {
		fmt.Printf("Launching reshare on old %s\n", node.addr)
		d.resharedNodes = append(d.resharedNodes, node)
		clientCounter.Add(1)
		go runreshare(node, false)
	}

	// run the new ones
	for _, node := range d.newNodes[:newRun] {
		fmt.Printf("Launching reshare on new  %s\n", node.addr)
		d.resharedNodes = append(d.resharedNodes, node)
		clientCounter.Add(1)
		go runreshare(node, true)
	}
	// wait for the return of the clients
	fmt.Println("\n\n -- WAITING COUNTER for ", total, " nodes --")
	checkWait(clientCounter)
	fmt.Printf("\n\n\n OUWOUWOUWOUWOUWOUWUWOUW\n\n\n\n")
	fmt.Println("\n\n - WAITING group from leader -- ")
	finalGroup := <-groupCh
	fmt.Printf("\n\n -- TEST FINISHED ALL RESHARE DKG --\n\n")
	d.newGroup = finalGroup
	return finalGroup
}

// newNode creates a node struct from a drand and sets the clock according to
// the drand test clock.
func (d *DrandTest2) newNode(dr *Drand) *Node {
	id := dr.priv.Public.Address()
	now := d.clock.Now()
	clock := clock.NewFakeClockAt(now)
	dr.opts.clock = clock
	return &Node{
		addr:  id,
		drand: dr,
		clock: clock,
	}
}

func checkWait(counter *sync.WaitGroup) {
	var doneCh = make(chan bool, 1)
	go func() {
		counter.Wait()
		doneCh <- true
	}()
	select {
	case <-doneCh:
		break
	case <-time.After(11 * time.Second):
		panic("outdated beacon time")
	}
}

// Check they all have same public group file after dkg
func TestDrandPublicGroup(t *testing.T) {
	n := 10
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	//genesisTime := clock.NewFakeClock().Now().Unix()
	dt := NewDrandTest2(t, n, thr, p)
	defer dt.Cleanup()
	group := dt.RunDKG()
	//client := NewGrpcClient()
	cm := dt.nodes[0].drand.opts.certmanager
	client := NewGrpcClientFromCert(cm)
	rest := net.NewRestClientFromCertManager(cm)
	for _, node := range dt.nodes {
		d := node.drand
		groupResp, err := client.Group(d.priv.Public.Address(), d.priv.Public.TLS)
		require.NoError(t, err, fmt.Sprintf("addr %s", node.addr))
		received, err := key.GroupFromProto(groupResp)
		require.NoError(t, err)
		require.True(t, group.Equal(received))
	}
	for _, node := range dt.nodes {
		var found bool
		addr := node.addr
		public := node.drand.priv.Public
		for _, n := range group.Nodes {
			sameAddr := n.Address() == addr
			sameKey := n.Key.Equal(public.Key)
			sameTLS := n.IsTLS() == public.TLS
			if sameAddr && sameKey && sameTLS {
				found = true
				break
			}
		}
		require.True(t, found)
	}

	restGroup, err := rest.Group(context.TODO(), dt.nodes[0].drand.priv.Public, &drand.GroupRequest{})
	require.NoError(t, err)
	received, err := key.GroupFromProto(restGroup)
	require.NoError(t, err)
	require.True(t, group.Equal(received))
}

// Test if the we can correctly fetch the rounds after a DKG using the
// PublicRand RPC call
func TestDrandPublicRand(t *testing.T) {
	n := 4
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	dt := NewDrandTest2(t, n, thr, p)
	defer dt.Cleanup()
	group := dt.RunDKG()
	time.Sleep(getSleepDuration())
	root := dt.nodes[0].drand
	rootID := root.priv.Public

	dt.MoveToTime(group.GenesisTime)
	// do a few periods
	for i := 0; i < 3; i++ {
		dt.MoveTime(group.Period)
	}

	cm := root.opts.certmanager
	client := net.NewGrpcClientFromCertManager(cm)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// get last round first
	resp, err := client.PublicRand(ctx, rootID, new(drand.PublicRandRequest))
	require.NoError(t, err)

	initRound := resp.Round + 1
	max := initRound + 4
	for i := initRound; i < max; i++ {
		dt.MoveTime(group.Period)
		req := new(drand.PublicRandRequest)
		req.Round = i
		resp, err := client.PublicRand(ctx, rootID, req)
		require.NoError(t, err)
		require.Equal(t, i, resp.Round)
		fmt.Println("REQUEST ROUND ", i, " GOT ROUND ", resp.Round)
	}
}

// Test if the we can correctly fetch the rounds after a DKG using the
// PublicRandStream RPC call
func TestDrandPublicStream(t *testing.T) {
	n := 4
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	//genesisTime := clock.NewFakeClock().Now().Unix()
	dt := NewDrandTest2(t, n, thr, p)
	defer dt.Cleanup()
	group := dt.RunDKG()
	time.Sleep(getSleepDuration())
	root := dt.nodes[0]
	rootID := root.drand.priv.Public

	dt.MoveToTime(group.GenesisTime)
	// do a few periods
	for i := 0; i < 3; i++ {
		dt.MoveTime(group.Period)
	}

	cm := root.drand.opts.certmanager
	client := net.NewGrpcClientFromCertManager(cm)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// get last round first
	resp, err := client.PublicRand(ctx, rootID, new(drand.PublicRandRequest))
	require.NoError(t, err)

	//  run streaming and expect responses
	req := &drand.PublicRandRequest{Round: resp.GetRound()}
	respCh, err := client.PublicRandStream(ctx, root.drand.priv.Public, req)
	require.NoError(t, err)
	// expect first round now since node already has it
	select {
	case beacon := <-respCh:
		require.Equal(t, beacon.GetRound(), resp.GetRound())
	case <-time.After(100 * time.Millisecond):
		require.True(t, false, "too late")
	}
	nTry := 4
	// we expect the next one now
	initRound := resp.Round + 1
	maxRound := initRound + uint64(nTry)
	fmt.Println("Streaming for future rounds starting from", initRound)
	for round := initRound; round < maxRound; round++ {
		// move time to next period
		dt.MoveTime(group.Period)
		select {
		case beacon := <-respCh:
			require.Equal(t, beacon.GetRound(), round)
		case <-time.After(1 * time.Second):
			require.True(t, false, "too late for streaming, round %d didn't reply in time", round)
		}
	}
	// try fetching with round 0 -> get latest
	respCh, err = client.PublicRandStream(ctx, root.drand.priv.Public, new(drand.PublicRandRequest))
	require.NoError(t, err)
	select {
	case <-respCh:
		require.False(t, true, "shouldn't get an entry if time doesn't go by")
	case <-time.After(50 * time.Millisecond):
		// correct
	}

	dt.MoveTime(group.Period)
	select {
	case resp := <-respCh:
		require.Equal(t, maxRound, resp.GetRound())
	case <-time.After(50 * time.Millisecond):
		// correct
	}
}

// BatchNewDrand returns n drands, using TLS or not, with the given
// options. It returns the list of Drand structures, the group created,
// the folder where db, certificates, etc are stored. It is the folder
// to delete at the end of the test. As well, it returns a public grpc
// client that can reach any drand node.
func BatchNewDrand(n int, insecure bool, opts ...ConfigOption) ([]*Drand, *key.Group, string, []string) {
	var privs []*key.Pair
	var group *key.Group
	if insecure {
		privs, group = test.BatchIdentities(n)
	} else {
		privs, group = test.BatchTLSIdentities(n)
	}
	ports := test.Ports(n)
	var err error
	drands := make([]*Drand, n, n)
	tmp := os.TempDir()
	dir, err := ioutil.TempDir(tmp, "drand")
	if err != nil {
		panic(err)
	}

	certPaths := make([]string, n)
	keyPaths := make([]string, n)
	if !insecure {
		for i := 0; i < n; i++ {
			certPath := path.Join(dir, fmt.Sprintf("server-%d.crt", i))
			keyPath := path.Join(dir, fmt.Sprintf("server-%d.key", i))
			if httpscerts.Check(certPath, keyPath) != nil {

				h, _, err := gnet.SplitHostPort(privs[i].Public.Address())
				if err != nil {
					panic(err)
				}
				if err := httpscerts.Generate(certPath, keyPath, h); err != nil {
					panic(err)
				}
			}
			certPaths[i] = certPath
			keyPaths[i] = keyPath
		}
	}

	for i := 0; i < n; i++ {
		s := test.NewKeyStore()
		s.SaveKeyPair(privs[i])
		// give each one their own private folder
		dbFolder := path.Join(dir, fmt.Sprintf("db-%d", i))
		confOptions := []ConfigOption{WithDbFolder(dbFolder)}
		if !insecure {
			confOptions = append(confOptions, WithTLS(certPaths[i], keyPaths[i]))
			confOptions = append(confOptions, WithTrustedCerts(certPaths...))
		} else {
			confOptions = append(confOptions, WithInsecure())
		}
		confOptions = append(confOptions, WithControlPort(ports[i]))
		confOptions = append(confOptions, WithLogLevel(log.LogDebug))
		// add options in last so it overwrites the default
		confOptions = append(confOptions, opts...)
		drands[i], err = NewDrand(s, NewConfig(confOptions...))
		if err != nil {
			panic(err)
		}
	}
	return drands, group, dir, certPaths
}

// CloseAllDrands closes all drands
func CloseAllDrands(drands []*Drand) {
	for i := 0; i < len(drands); i++ {
		drands[i].Stop()
		//os.RemoveAll(drands[i].opts.dbFolder)
	}
}

func getSleepDuration() time.Duration {
	if os.Getenv("CIRCLE_CI") != "" {
		fmt.Println("--- Sleeping on CIRCLECI")
		return time.Duration(1000) * time.Millisecond
	}
	return time.Duration(500) * time.Millisecond
}
