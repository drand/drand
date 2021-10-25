package core

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/drand/drand/common"

	"github.com/stretchr/testify/assert"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/stretchr/testify/require"
)

func setFDLimit() {
	fdOpen := 2000
	_, max, err := unixGetLimit()
	if err != nil {
		panic(err)
	}
	if err := unixSetLimit(uint64(fdOpen), max); err != nil {
		panic(err)
	}
}

// 1 second after end of dkg
var testBeaconOffset = 1
var testDkgTimeout = 2 * time.Second

// Test that the normal dkg process works correctly
func TestRunDKG(t *testing.T) {
	n := 4
	expectedBeaconPeriod := 5 * time.Second
	sch, beaconID := scheme.GetSchemeFromEnv(), common.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, key.DefaultThreshold(n), expectedBeaconPeriod, sch, beaconID)
	defer dt.Cleanup()

	group := dt.RunDKG()

	t.Log(group)

	assert.Equal(t, 3, group.Threshold)
	assert.Equal(t, expectedBeaconPeriod, group.Period)
	assert.Equal(t, time.Duration(0), group.CatchupPeriod)
	assert.Equal(t, n, len(group.Nodes))
	assert.Equal(t, int64(449884810), group.GenesisTime)
}

// Test dkg for a large quantity of nodes (22 nodes)
func TestRunDKGLarge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	setFDLimit()

	n := 22
	expectedBeaconPeriod := 5 * time.Second
	sch, beaconID := scheme.GetSchemeFromEnv(), common.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, key.DefaultThreshold(n), expectedBeaconPeriod, sch, beaconID)
	defer dt.Cleanup()

	group := dt.RunDKG()

	assert.Equal(t, 12, group.Threshold)
	assert.Equal(t, expectedBeaconPeriod, group.Period)
	assert.Equal(t, time.Duration(0), group.CatchupPeriod)
	assert.Equal(t, n, len(group.Nodes))
	assert.Equal(t, int64(449884810), group.GenesisTime)
}

// Test Start/Stop after DKG
// Run DKG
// Stop last node
// Restart last node and wait catch up
// Check beacon still works and length is correct
func TestDrandDKGFresh(t *testing.T) {
	n := 4
	beaconPeriod := 1 * time.Second
	sch, beaconID := scheme.GetSchemeFromEnv(), common.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, key.DefaultThreshold(n), beaconPeriod, sch, beaconID)
	defer dt.Cleanup()

	// Run DKG
	finalGroup := dt.RunDKG()

	// make the last node fail (stop)
	lastNode := dt.nodes[n-1]
	restOfNodes := dt.nodes[:n-1]

	t.Logf("Stop last node %s", lastNode.addr)
	dt.StopMockNode(lastNode.addr, false)

	// move time to genesis
	dt.SetMockClock(t, finalGroup.GenesisTime)
	t.Logf("Time = %d", finalGroup.GenesisTime)

	// two = genesis + 1st round (happens at genesis)
	t.Log("Check Beacon Length")
	dt.CheckBeaconLength(t, restOfNodes, 2)

	t.Logf("Start last node %s", lastNode.addr)
	dt.StartDrand(lastNode.addr, true, false)

	// The catchup process will finish when node gets the previous beacons (1st round)
	dt.WaitUntilRound(t, lastNode, 1)

	dt.AdvanceMockClock(t, beaconPeriod)

	t.Log("Check Beacon Length")
	dt.CheckBeaconLength(t, dt.nodes, 3)

	t.Log("Check Beacon Public")
	response := dt.CheckPublicBeacon(lastNode.addr, false)
	assert.Equal(t, uint64(2), response.Round)
}

// Test dkg when two nodes cannot broadcast messages between them. The rest of the nodes
// will be able to broadcast messages, so the process should finish successfully
// Given 4 nodes = [ 0, 1, 2, 3]
// 1. Limit communication between 1 and 2
// 2. Run DKG
// 3. Run reshare
func TestRunDKGBroadcastDeny(t *testing.T) {
	n := 4
	thr := 3
	beaconPeriod := 1 * time.Second
	sch, beaconID := scheme.GetSchemeFromEnv(), common.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, thr, beaconPeriod, sch, beaconID)
	defer dt.Cleanup()

	// close connection between a pair of nodes
	node1 := dt.nodes[1]
	node2 := dt.nodes[2]

	t.Log("Setting node 1 not to broadcast messages to the node 2")
	node1.drand.DenyBroadcastTo(t, node2.addr)

	t.Log("Setting node 2 not to broadcast messages to the node 1")
	node2.drand.DenyBroadcastTo(t, node1.addr)

	group1 := dt.RunDKG()

	// Advance clock
	dt.SetMockClock(t, group1.GenesisTime)
	dt.AdvanceMockClock(t, 1*time.Second)

	group2, err := dt.RunReshare(t, nil, n, 0, thr, 1*time.Second, false, false, false)
	require.NoError(t, err)
	require.NotNil(t, group2)

	t.Log("Resharing complete")
}

// Test the dkg reshare can be forced to restart and finish successfully
// when another dkg reshare was running before
func TestRunDKGReshareForce(t *testing.T) {
	oldNodes := 4
	oldThreshold := 3
	timeout := 1 * time.Second
	beaconPeriod := 2 * time.Second
	sch, beaconID := scheme.GetSchemeFromEnv(), common.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, oldNodes, oldThreshold, beaconPeriod, sch, beaconID)
	defer dt.Cleanup()

	group1 := dt.RunDKG()

	dt.SetMockClock(t, group1.GenesisTime)
	dt.AdvanceMockClock(t, 1*time.Second)

	// run the resharing
	stateCh := make(chan int)
	go func() {
		t.Log("[ReshareForce] Start reshare")
		_, err := dt.RunReshare(t, stateCh, oldNodes, 0, oldThreshold, timeout, false, true, false)
		require.Error(t, err)
	}()

LOOP:
	for {
		select {
		case state := <-stateCh:
			if state == ReshareUnlock {
				break LOOP
			}
		case <-time.After(2 * time.Minute):
			t.Errorf("Timeout waiting reshare process to get unlock phase")
		}
	}

	// force
	t.Log("[reshare] Start again!")
	group3, err := dt.RunReshare(t, nil, oldNodes, 0, oldThreshold, timeout, true, false, false)
	require.NoError(t, err)

	t.Log("[reshare] Move to response phase!")
	t.Logf("[reshare] Group: %s", group3)
}

// This tests when a node first signal his intention to participate into a
// resharing but is down right after  - he shouldn't be in the final group
func TestRunDKGReshareAbsentNode(t *testing.T) {
	oldNodes, newNodes := 3, 4
	oldThreshold, newThreshold := 2, 3
	timeout, beaconPeriod := 1*time.Second, 2*time.Second
	sch, beaconID := scheme.GetSchemeFromEnv(), common.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, oldNodes, oldThreshold, beaconPeriod, sch, beaconID)
	defer dt.Cleanup()

	group1 := dt.RunDKG()

	dt.SetMockClock(t, group1.GenesisTime)
	dt.WaitUntilChainIsServing(t, dt.nodes[0])

	// move to genesis time - so nodes start to make a round
	// dt.AdvanceMockClock(t,offsetGenesis)
	// two = genesis + 1st round (happens at genesis)

	t.Log("Check Beacon Length")
	dt.CheckBeaconLength(t, dt.nodes, 2)

	// so nodes think they are going forward with round 2
	dt.AdvanceMockClock(t, 1*time.Second)

	t.Log("Adding new nodes to the group")
	nodesToAdd := newNodes - oldNodes
	dt.SetupNewNodes(t, nodesToAdd)

	// we want to stop one node right after the group is created
	nodeToStop := 1
	leader := 0

	dt.nodes[leader].drand.setupCB = func(g *key.Group) {
		t.Logf("Stopping node %d \n", nodeToStop)
		dt.nodes[nodeToStop].drand.Stop(context.Background())
		t.Logf("Node %d stopped \n", nodeToStop)
	}

	t.Log("Setup reshare done. Starting reshare... Ignoring reshare errors")
	newGroup, err := dt.RunReshare(t, nil, oldNodes, nodesToAdd, newThreshold, timeout, false, false, true)
	require.NoError(t, err)
	require.NotNil(t, newGroup)

	// the node that had stopped must not be in the group
	t.Logf("Check node %d is not included in the group \n", nodeToStop)
	missingPublic := dt.nodes[nodeToStop].drand.priv.Public
	require.Nil(t, newGroup.Find(missingPublic), "missing public is found", missingPublic)
}

// The test creates the scenario where one node made a complaint during the DKG, at the second phase, so normally,
// there should be a "Justification" at the third phase. In this case, there is not. This scenario
// can happen if there is an offline node right at the beginning of DKG that don't even send any message.
func TestRunDKGReshareTimeout(t *testing.T) {
	oldNodes, newNodes, oldThreshold, newThreshold := 3, 4, 2, 3
	timeout, beaconPeriod := 1*time.Second, 2*time.Second
	offline := 1
	sch, beaconID := scheme.GetSchemeFromEnv(), common.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, oldNodes, oldThreshold, beaconPeriod, sch, beaconID)
	defer dt.Cleanup()

	group1 := dt.RunDKG()

	dt.SetMockClock(t, group1.GenesisTime)
	dt.WaitUntilChainIsServing(t, dt.nodes[0])

	// move to genesis time - so nodes start to make a round
	// dt.AdvanceMockClock(t,offsetGenesis)
	// two = genesis + 1st round (happens at genesis)

	t.Log("Check Beacon Length")
	dt.CheckBeaconLength(t, dt.nodes, 2)

	// so nodes think they are going forward with round 2
	dt.AdvanceMockClock(t, 1*time.Second)

	// + offline makes sure t
	nodesToKeep := oldNodes - offline
	nodesToAdd := newNodes - nodesToKeep
	dt.SetupNewNodes(t, nodesToAdd)

	t.Log("Setup reshare done. Starting reshare.")

	// run the resharing
	var doneReshare = make(chan *key.Group)
	go func() {
		t.Log("[reshare] Start reshare")
		group, err := dt.RunReshare(t, nil, nodesToKeep, nodesToAdd, newThreshold, timeout, false, false, false)
		require.NoError(t, err)
		doneReshare <- group
	}()
	time.Sleep(3 * time.Second)

	t.Log("Move to response phase")
	dt.AdvanceMockClock(t, timeout)

	// TODO: How to remove this sleep? How to check when a node is at this stage
	// at this point in time, nodes should have gotten all deals and send back their responses to all nodes
	time.Sleep(getSleepDuration())

	t.Log("Move to justification phase")
	dt.AdvanceMockClock(t, timeout)

	// TODO: How to remove this sleep? How to check when a node is at this stage
	// at this time, all nodes received the responses of each other nodes but
	// there is one node missing so they expect justifications
	time.Sleep(getSleepDuration())

	// TODO: How to remove this sleep? How to check when a node is at this stage
	t.Log("Move to finish phase")
	dt.AdvanceMockClock(t, timeout)

	time.Sleep(getSleepDuration())
	// at this time they received no justification from the missing node so he's
	// excluded of the group and the dkg should finish
	// time.Sleep(10 * time.Second)

	var resharedGroup *key.Group
	select {
	case resharedGroup = <-doneReshare:
	case <-time.After(1 * time.Second):
		require.True(t, false)
	}

	t.Logf("[reshare] Group: %s", resharedGroup)

	transitionTime := resharedGroup.TransitionTime
	now := dt.Now().Unix()

	// get rounds from first node in the "old" group - since he's the leader for
	// the new group, he's alive
	t.Log("Check Beacon Public on Leader")
	lastBeacon := dt.CheckPublicBeacon(dt.Ids(1, false)[0], false)

	// move to the transition time period by period - do not skip potential
	// periods as to emulate the normal time behavior
	for now < transitionTime-1 {
		dt.AdvanceMockClock(t, beaconPeriod)

		t.Log("Check Beacon Public on Leader")
		lastBeacon = dt.CheckPublicBeacon(dt.Ids(1, false)[0], false)
		now = dt.Now().Unix()
	}

	// move to the transition time
	dt.SetMockClock(t, resharedGroup.TransitionTime)
	time.Sleep(getSleepDuration())

	// test that all nodes in the new group have generated a new beacon
	t.Log("Check Beacon Length")
	dt.CheckBeaconLength(t, dt.resharedNodes, int(lastBeacon.Round+1))
}

// nolint:funlen
// This test is where a client can stop the resharing in process and start again
func TestRunDKGResharePreempt(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping testing in CI environment")
	}

	oldN := 3
	newN := 3
	Thr := 2
	timeout := 1 * time.Second
	beaconPeriod := 2 * time.Second
	sch, beaconID := scheme.GetSchemeFromEnv(), common.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, oldN, Thr, beaconPeriod, sch, beaconID)
	defer dt.Cleanup()

	group1 := dt.RunDKG()

	dt.SetMockClock(t, group1.GenesisTime)
	dt.WaitUntilChainIsServing(t, dt.nodes[0])

	// move to genesis time - so nodes start to make a round
	t.Log("Check Beacon Length")
	dt.CheckBeaconLength(t, dt.nodes, 2)

	// so nodes think they are going forward with round 2
	dt.AdvanceMockClock(t, 1*time.Second)

	// first, the leader is going to start running a failed reshare:
	oldNode := dt.group.Find(dt.nodes[0].drand.priv.Public)
	if oldNode == nil {
		panic("Leader not found in old group")
	}

	// old root: oldNode.Index leater: leader.addr
	oldDone := make(chan error, 1)
	go func() {
		client, err := net.NewControlClient(dt.nodes[0].drand.opts.controlPort)
		require.NoError(t, err)

		t.Log("Init reshare on leader")
		_, err = client.InitReshareLeader(newN, Thr, timeout, 0, "unused secret", "", testBeaconOffset, beaconID)

		// Done resharing
		if err == nil {
			panic("initial reshare should fail.")
		}
		oldDone <- err
	}()
	time.Sleep(100 * time.Millisecond)

	// run the resharing
	var doneReshare = make(chan *key.Group, 1)
	go func() {
		g, err := dt.RunReshare(t, nil, oldN, 0, Thr, timeout, false, false, false)
		require.NoError(t, err)
		doneReshare <- g
	}()
	time.Sleep(time.Second)

	dt.AdvanceMockClock(t, time.Second)
	time.Sleep(time.Second)

	t.Log("Move to response phase")
	dt.AdvanceMockClock(t, timeout)

	// TODO: How to remove this sleep? How to check when a node is at this stage
	// at this point in time, nodes should have gotten all deals and send back
	// their responses to all nodes
	time.Sleep(getSleepDuration())

	t.Log("Move to justification phase")
	dt.AdvanceMockClock(t, timeout)

	// TODO: How to remove this sleep? How to check when a node is at this stage
	// at this time, all nodes received the responses of each other nodes but
	// there is one node missing so they expect justifications
	time.Sleep(getSleepDuration())

	// TODO: How to remove this sleep? How to check when a node is at this stage
	t.Log("Move to finish phase")
	dt.AdvanceMockClock(t, timeout)
	time.Sleep(getSleepDuration())

	// at this time they received no justification from the missing node so he's
	// exlucded of the group and the dkg should finish
	select {
	case <-doneReshare:
	case <-time.After(1 * time.Second):
		panic("expect dkg to have finished within one second")
	}
	select {
	case <-oldDone:
	case <-time.After(1 * time.Second):
		panic("expect aborted dkg to fail")
	}

	t.Log("Check Beacon Public")
	dt.CheckPublicBeacon(dt.Ids(1, false)[0], false)
}

// Check they all have same chain info
func TestDrandPublicChainInfo(t *testing.T) {
	n := 10
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	sch, beaconID := scheme.GetSchemeFromEnv(), common.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, thr, p, sch, beaconID)
	defer dt.Cleanup()

	group := dt.RunDKG()

	chainInfo := chain.NewChainInfo(group)
	certManager := dt.nodes[0].drand.opts.certmanager
	client := NewGrpcClientFromCert(certManager)

	for i, node := range dt.nodes {
		d := node.drand
		t.Logf("Getting chaing info from node %d \n", i)
		received, err := client.ChainInfo(d.priv.Public)

		require.NoError(t, err, fmt.Sprintf("addr %s", node.addr))

		t.Logf("Check Node %d has the same chain info \n", i)
		require.True(t, chainInfo.Equal(received))
	}

	for i, node := range dt.nodes {
		var found bool
		addr := node.addr
		public := node.drand.priv.Public

		// Looking for node i inside the group file
		for _, n := range group.Nodes {
			sameAddr := n.Address() == addr
			sameKey := n.Key.Equal(public.Key)
			sameTLS := n.IsTLS() == public.TLS

			if sameAddr && sameKey && sameTLS {
				found = true
				break
			}
		}

		t.Logf("Check if node %d is present on the group with the correct configuration \n", i)
		require.True(t, found)
	}

	// rest := net.NewRestClientFromCertManager(cm)
	// restGroup, err := rest.Group(context.TODO(), dt.nodes[0].drand.priv.Public, &drand.GroupRequest{})
	// require.NoError(t, err)
	// received, err := key.GroupFromProto(restGroup)
	// require.NoError(t, err)
	// require.True(t, group.Equal(received))
}

// Test if we can correctly fetch the rounds after a DKG using the PublicRand RPC call
func TestDrandPublicRand(t *testing.T) {
	n := 4
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	sch, beaconID := scheme.GetSchemeFromEnv(), common.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, thr, p, sch, beaconID)
	defer dt.Cleanup()

	group := dt.RunDKG()

	root := dt.nodes[0].drand
	rootID := root.priv.Public

	dt.SetMockClock(t, group.GenesisTime)
	dt.WaitUntilChainIsServing(t, dt.nodes[0])

	err := dt.WaitUntilRound(t, dt.nodes[0], 1)
	require.NoError(t, err)

	// do a few periods
	for i := 0; i < 3; i++ {
		dt.AdvanceMockClock(t, group.Period)

		err = dt.WaitUntilRound(t, dt.nodes[0], uint64(i+2))
		require.NoError(t, err)
	}

	cm := root.opts.certmanager
	client := net.NewGrpcClientFromCertManager(cm)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// get last round first
	t.Log("Getting the last round first")
	resp, err := client.PublicRand(ctx, rootID, new(drand.PublicRandRequest))
	require.NoError(t, err)

	// get next rounds
	initRound := resp.Round + 1
	max := initRound + 4
	for i := initRound; i < max; i++ {
		t.Logf("Move clock to generate a new round %d \n", i)
		dt.AdvanceMockClock(t, group.Period)

		err = dt.WaitUntilRound(t, dt.nodes[0], i)
		require.NoError(t, err)

		req := new(drand.PublicRandRequest)
		req.Round = i

		t.Logf("Getting the actual rand %d \n", i)
		resp, err := client.PublicRand(ctx, rootID, req)
		require.NoError(t, err)

		t.Logf("Checking if the round we got (%d) is the expected one (%d) \n", resp.Round, i)
		require.Equal(t, i, resp.Round)
	}
}

// Test if the we can correctly fetch the rounds after a DKG using the
// PublicRandStream RPC call
// It also test the follow method call (it avoid redoing an expensive and long
// setup on CI to test both behaviors).
func TestDrandPublicStream(t *testing.T) {
	n := 4
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	sch, beaconID := scheme.GetSchemeFromEnv(), common.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, thr, p, sch, beaconID)
	defer dt.Cleanup()

	group := dt.RunDKG()

	root := dt.nodes[0]
	rootID := root.drand.priv.Public

	dt.SetMockClock(t, group.GenesisTime)
	dt.WaitUntilChainIsServing(t, dt.nodes[0])

	err := dt.WaitUntilRound(t, dt.nodes[0], 1)
	require.NoError(t, err)

	// do a few periods
	for i := 0; i < 3; i++ {
		dt.AdvanceMockClock(t, group.Period)

		err = dt.WaitUntilRound(t, dt.nodes[0], uint64(i+2))
		require.NoError(t, err)
	}

	cm := root.drand.opts.certmanager
	client := net.NewGrpcClientFromCertManager(cm)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// get last round first
	t.Log("Getting the last round first with PublicRand method")
	resp, err := client.PublicRand(ctx, rootID, new(drand.PublicRandRequest))
	require.NoError(t, err)

	//  run streaming and expect responses
	req := &drand.PublicRandRequest{Round: resp.GetRound()}
	respCh, err := client.PublicRandStream(ctx, root.drand.priv.Public, req)
	require.NoError(t, err)

	// expect first round now since node already has it
	t.Log("Waiting to receive the first round as the node should have it now...")
	select {
	case beacon := <-respCh:
		t.Logf("First round rcv %d \n", resp.GetRound())
		require.Equal(t, beacon.GetRound(), resp.GetRound())

	case <-time.After(100 * time.Millisecond):
		t.Logf("First round NOT rcv. Timeout has passed \n")
		require.True(t, false, "too late for the first round, it didn't reply in time")
	}

	nTry := 4
	// we expect the next one now
	initRound := resp.Round + 1
	maxRound := initRound + uint64(nTry)
	t.Logf("Streaming for future rounds starting from %d", initRound)

	for round := initRound; round < maxRound; round++ {
		// move time to next period
		dt.AdvanceMockClock(t, group.Period)

		select {
		case beacon := <-respCh:
			require.Equal(t, beacon.GetRound(), round)
		case <-time.After(1 * time.Second):
			t.Logf("Round %d NOT rcv. Timeout has passed \n", round)
			require.True(t, false, fmt.Sprintf("too late for streaming, round %d didn't reply in time", round))
		}
	}

	// try fetching with round 0 -> get latest
	t.Logf("Streaming for rounds starting from %d to %d", 0, maxRound)

	respCh, err = client.PublicRandStream(ctx, root.drand.priv.Public, new(drand.PublicRandRequest))
	require.NoError(t, err)

	select {
	case <-respCh:
		require.False(t, true, "shouldn't get a round if time doesn't go by")

	case <-time.After(50 * time.Millisecond):
		// correct
	}

	dt.AdvanceMockClock(t, group.Period)
	select {
	case resp := <-respCh:
		t.Logf("Round %d rcv \n", maxRound)
		require.Equal(t, maxRound, resp.GetRound())

	case <-time.After(50 * time.Millisecond):
		// correct
	}
}

// nolint:funlen
// This test makes sure the "FollowChain" grpc method works fine
func TestDrandFollowChain(t *testing.T) {
	n, p := 4, 1*time.Second
	sch, beaconID := scheme.GetSchemeFromEnv(), common.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, key.DefaultThreshold(n), p, sch, beaconID)
	defer dt.Cleanup()

	group := dt.RunDKG()
	rootID := dt.nodes[0].drand.priv.Public

	dt.SetMockClock(t, group.GenesisTime)
	dt.WaitUntilChainIsServing(t, dt.nodes[0])

	err := dt.WaitUntilRound(t, dt.nodes[0], 1)
	require.NoError(t, err)

	// do a few periods
	for i := 0; i < 6; i++ {
		dt.AdvanceMockClock(t, group.Period)

		err := dt.WaitUntilRound(t, dt.nodes[0], uint64(i+2))
		require.NoError(t, err)
	}

	client := net.NewGrpcClientFromCertManager(dt.nodes[0].drand.opts.certmanager)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// get last round first
	resp, err := client.PublicRand(ctx, rootID, new(drand.PublicRandRequest))
	require.NoError(t, err)

	// TEST setup a new node and fetch history
	newNode := dt.SetupNewNodes(t, 1)[0]
	newClient, err := net.NewControlClient(newNode.drand.opts.controlPort)
	require.NoError(t, err)

	addrToFollow := []string{rootID.Address()}
	hash := fmt.Sprintf("%x", chain.NewChainInfo(group).Hash())
	tls := true

	// First try with an invalid hash info
	t.Logf("Trying to follow with an invalid address\n")

	ctx, cancel = context.WithCancel(context.Background())
	_, errCh, _ := newClient.StartFollowChain(ctx, "deadbeef", addrToFollow, tls, 10000, beaconID)

	select {
	case <-errCh:
		t.Logf("An error was received as the address is invalid")
	case <-time.After(100 * time.Millisecond):
		t.Logf("An error should have been received as the address is invalid")
		panic("should have errored")
	}

	_, errCh, _ = newClient.StartFollowChain(ctx, "tutu", addrToFollow, tls, 10000, beaconID)
	select {
	case <-errCh:
		t.Logf("An error was received as the address is invalid")
	case <-time.After(100 * time.Millisecond):
		t.Logf("An error should have been received as the address is invalid")
		panic("should have errored")
	}

	fn := func(upTo, exp uint64) {
		ctx, cancel = context.WithCancel(context.Background())

		t.Logf("Starting to follow chain with a valid address\n")
		t.Logf("%s", hash)
		progress, errCh, err := newClient.StartFollowChain(ctx, hash, addrToFollow, tls, upTo, beaconID)
		require.NoError(t, err)

		var goon = true
		for goon {
			select {
			case p, ok := <-progress:
				if ok && p.Current == exp {
					t.Logf("Successful beacion rcv. Round: %d. Keep following chain.", exp)
					goon = false
					break
				}
			case e := <-errCh:
				if e == io.EOF {
					break
				}
				require.NoError(t, e)
			case <-time.After(1 * time.Second):
				t.FailNow()
			}
		}

		// cancel the operation
		cancel()

		// check if the beacon is in the database
		store, err := newNode.drand.createBoltStore(beaconID)
		require.NoError(t, err)
		defer store.Close()

		lastB, err := store.Last()
		require.NoError(t, err)
		require.Equal(t, exp, lastB.Round, "found %d vs expected %d", lastB.Round, exp)
	}

	fn(resp.GetRound()-2, resp.GetRound()-2)
	fn(0, resp.GetRound())
}

// Test if we can correctly fetch the rounds through the local proxy
func TestDrandPublicStreamProxy(t *testing.T) {
	n := 4
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	sch, beaconID := scheme.GetSchemeFromEnv(), common.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, thr, p, sch, beaconID)
	defer dt.Cleanup()

	group := dt.RunDKG()

	root := dt.nodes[0]
	dt.SetMockClock(t, group.GenesisTime)
	dt.WaitUntilChainIsServing(t, dt.nodes[0])

	// do a few periods
	for i := 0; i < 3; i++ {
		dt.AdvanceMockClock(t, group.Period)
	}

	client := &drandProxy{root.drand}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// get last round first
	t.Logf("Getting round %d", 0)
	resp, err := client.Get(ctx, 0)
	require.NoError(t, err)

	//  run streaming and expect responses
	t.Log("Watching new rounds generated")
	rc := client.Watch(ctx)

	// expect first round now since node already has it
	dt.AdvanceMockClock(t, group.Period)
	beacon, ok := <-rc
	if !ok {
		panic("expected beacon")
	}

	t.Logf("Round received %d", beacon.Round())
	require.Equal(t, beacon.Round(), resp.Round()+1)

	nTry := 4
	// we expect the next one now
	initRound := resp.Round() + 2
	maxRound := initRound + uint64(nTry)
	for round := initRound; round < maxRound; round++ {
		// move time to next period
		dt.AdvanceMockClock(t, group.Period)
		beacon, ok = <-rc

		require.True(t, ok)

		t.Logf("Round received %d", beacon.Round())
		require.Equal(t, round, beacon.Round())
	}
}
