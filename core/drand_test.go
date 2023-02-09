package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/weaveworks/common/fs"

	"github.com/drand/drand/chain"
	derrors "github.com/drand/drand/chain/errors"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"
	context2 "github.com/drand/drand/test/context"
)

func setFDLimit(t *testing.T) {
	fdOpen := uint64(3000)
	curr, max, err := unixGetLimit()
	if err != nil {
		t.Fatal(err)
	}
	if fdOpen <= curr {
		t.Logf("Current limit is larger (%d) than ours (%d); not changing it.\n", curr, fdOpen)
		return
	} else if err := unixSetLimit(fdOpen, max); err != nil {
		t.Fatal(err)
	}
}

func consumeProgress(t *testing.T, progress chan *drand.SyncProgress, errCh chan error, amount uint64, progressing bool) {
	t.Logf("in consumeProgress(t, progress, errCh, %d, %t)\n", amount, progressing)

	if progressing {
		defer func() {
			for e := range errCh {
				if errors.Is(e, io.EOF) { // means we've reached the end
					t.Logf("\t\t --> Got EOF from daemon.")
					return
				}
				t.Logf("\t\t --> Unexpected error received: %v.", e)
				require.NoError(t, e)
			}
		}()

		for {
			select {
			case p, ok := <-progress:
				if ok && p.Current == amount {
					t.Logf("\t\t --> Successful chain sync progress. Achieved round: %d.", amount)
					return
				}
				if !ok {
					return
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("\t\t --> Timeout during test")
				return
			}
		}
	} else { // we test the special case when we get Current == 0 and Target reports the amount of invalid beacon
		select {
		case p, ok := <-progress:
			require.True(t, ok)
			require.Equal(t, uint64(0), p.Current)
			require.Equal(t, amount, p.Target)
		case e := <-errCh:
			if errors.Is(e, io.EOF) {
				t.Logf("\t\t --> Got EOF from daemon.")
				return
			}
			t.Logf("\t\t -->Unexpected error received: %v.", e)
			require.NoError(t, e)
		case <-time.After(2 * time.Second):
			t.Fatalf("\t\t --> Timeout during test")
			return
		}
	}
}

// 1 second after end of dkg
var testBeaconOffset = 1
var testDkgTimeout = 2 * time.Second

// Test that the normal dkg process works correctly
func TestRunDKG(t *testing.T) {
	n := 4
	expectedBeaconPeriod := 5 * time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, key.DefaultThreshold(n), expectedBeaconPeriod, beaconID)

	group, err := dt.RunDKG()
	require.NoError(t, err)

	t.Log(group)

	require.Equal(t, 3, group.Threshold)
	require.Equal(t, expectedBeaconPeriod, group.Period)
	require.Equal(t, time.Duration(0), group.CatchupPeriod)
	require.Equal(t, n, len(group.Nodes))
	require.Equal(t, int64(449884810), group.GenesisTime)
}

// Test dkg for a large quantity of nodes (22 nodes)
func TestRunDKGLarge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	setFDLimit(t)

	n := 22
	expectedBeaconPeriod := 5 * time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, key.DefaultThreshold(n), expectedBeaconPeriod, beaconID)

	group, err := dt.RunDKG()
	require.NoError(t, err)

	require.Equal(t, 12, group.Threshold)
	require.Equal(t, expectedBeaconPeriod, group.Period)
	require.Equal(t, time.Duration(0), group.CatchupPeriod)
	require.Equal(t, n, len(group.Nodes))
	require.Equal(t, int64(449884810), group.GenesisTime)
}

// Test Start/Stop after DKG
// Run DKG
// Stop last node
// Restart last node and wait catch up
// Check beacon still works and length is correct
func TestDrandDKGFresh(t *testing.T) {
	n := 4
	beaconPeriod := 1 * time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, key.DefaultThreshold(n), beaconPeriod, beaconID)

	// Run DKG
	finalGroup, err := dt.RunDKG()
	require.NoError(t, err)

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
	dt.StartDrand(t, lastNode.addr, true, false)

	// The catchup process will finish when node gets the previous beacons (1st round)
	err = dt.WaitUntilRound(t, lastNode, 1)
	require.NoError(t, err)

	dt.AdvanceMockClock(t, beaconPeriod)

	t.Log("Check Beacon Length")
	dt.CheckBeaconLength(t, dt.nodes, 3)

	t.Log("Check Beacon Public")
	response := dt.CheckPublicBeacon(lastNode.addr, false)
	require.Equal(t, uint64(2), response.Round)
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
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, thr, beaconPeriod, beaconID)

	// close connection between a pair of nodes
	node1 := dt.nodes[1]
	node2 := dt.nodes[2]

	t.Log("Setting node 1 not to broadcast messages to the node 2")
	node1.drand.DenyBroadcastTo(t, node2.addr)

	t.Log("Setting node 2 not to broadcast messages to the node 1")
	node2.drand.DenyBroadcastTo(t, node1.addr)

	group1, err := dt.RunDKG()
	require.NoError(t, err)

	// Advance clock
	dt.SetMockClock(t, group1.GenesisTime)
	dt.AdvanceMockClock(t, 1*time.Second)

	group2, err := dt.RunReshare(t,
		&reshareConfig{
			oldRun:  n,
			newThr:  thr,
			timeout: time.Second,
		})
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
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, oldNodes, oldThreshold, beaconPeriod, beaconID)

	group1, err := dt.RunDKG()
	require.NoError(t, err)

	dt.SetMockClock(t, group1.GenesisTime)

	// wait to get first round
	t.Logf("Getting round %d", 0)
	err = dt.WaitUntilRound(t, dt.nodes[0], 1)
	require.NoError(t, err)

	// run the resharing
	stateCh := make(chan int)
	errFirstTry := make(chan error)
	go func() {
		t.Log("[ReshareForce] Start reshare")
		_, err := dt.RunReshare(t,
			&reshareConfig{
				stateCh:    stateCh,
				oldRun:     oldNodes,
				newThr:     oldThreshold,
				timeout:    timeout,
				onlyLeader: true,
			})
		errFirstTry <- err
	}()

	var resharingRunning bool
	for !resharingRunning {
		select {
		case state := <-stateCh:
			if state == ReshareUnlock {
				resharingRunning = true
			}
		case <-time.After(2 * time.Minute):
			t.Errorf("Timeout waiting reshare process to get unlock phase")
		}
	}
	// first resharing should fail
	select {
	case err := <-errFirstTry:
		require.Error(t, err)
	case <-time.After(2 * time.Minute):
		t.Errorf("timeout of the first resharing output")
	}

	// do a few periods
	for i := 0; i < 2; i++ {
		dt.AdvanceMockClock(t, group1.Period)
		err := dt.WaitUntilRound(t, dt.nodes[0], uint64(2+i))
		require.NoError(t, err)
	}

	// force
	t.Log("[reshare] Start again!")
	group3, err := dt.RunReshare(t,
		&reshareConfig{
			oldRun:  oldNodes,
			newThr:  oldThreshold,
			timeout: timeout,
			force:   true,
		})

	// second resharing should succeed
	require.NoError(t, err, "second resharing failed")

	t.Log("[reshare] Move to response phase!")
	t.Logf("[reshare] Group: %s", group3)
}

// This tests when a node first signal his intention to participate into a
// resharing but is down right after  - he shouldn't be in the final group
func TestRunDKGReshareAbsentNode(t *testing.T) {
	oldNodes, newNodes := 3, 4
	oldThreshold, newThreshold := 2, 3
	timeout, beaconPeriod := 1*time.Second, 2*time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, oldNodes, oldThreshold, beaconPeriod, beaconID)

	group1, err := dt.RunDKG()
	require.NoError(t, err)

	dt.SetMockClock(t, group1.GenesisTime)
	err = dt.WaitUntilChainIsServing(t, dt.nodes[0])
	require.NoError(t, err)

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
	nodeIndexToStop := 1
	nodeToStop := dt.nodes[nodeIndexToStop]
	leader := 0

	dt.nodes[leader].drand.setupCB = func(g *key.Group) {
		t.Logf("Stopping node for test: %s \n", nodeToStop.addr)
		nodeToStop.daemon.Stop(context.Background())
		<-nodeToStop.daemon.WaitExit()
		t.Logf("Node %d stopped \n", nodeIndexToStop)
	}

	t.Log("Setup reshare done. Starting reshare... Ignoring reshare errors")
	newGroup, err := dt.RunReshare(t, &reshareConfig{
		oldRun:    oldNodes,
		newRun:    nodesToAdd,
		newThr:    newThreshold,
		timeout:   timeout,
		ignoreErr: true,
	})
	require.NoError(t, err)
	require.NotNil(t, newGroup)

	// the node that had stopped must not be in the group
	t.Logf("Check node %d is not included in the group \n", nodeIndexToStop)
	missingPublic := nodeToStop.drand.priv.Public
	require.Nil(t, newGroup.Find(missingPublic), "missing public is found", missingPublic)
}

// The test creates the scenario where one node made a complaint during the DKG, at the second phase, so normally,
// there should be a "Justification" at the third phase. In this case, there is not. This scenario
// can happen if there is an offline node right at the beginning of DKG that don't even send any message.
//
//nolint:funlen
func TestRunDKGReshareTimeout(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Fails all the time on CI for some reason")
	}
	oldNodes, newNodes, oldThreshold, newThreshold := 3, 4, 2, 3
	timeout, beaconPeriod := 1*time.Second, 2*time.Second
	offline := 1
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, oldNodes, oldThreshold, beaconPeriod, beaconID)

	group1, err := dt.RunDKG()
	require.NoError(t, err)

	dt.SetMockClock(t, group1.GenesisTime)
	err = dt.WaitUntilChainIsServing(t, dt.nodes[0])
	require.NoError(t, err)

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
	doneReshare := make(chan *key.Group)
	go func() {
		t.Log("[reshare] Start reshare")
		// XXX: notice that the RunReshare is already running AdvanceMockClock on its own after a while!!
		group, err := dt.RunReshare(t,
			&reshareConfig{
				oldRun:  nodesToKeep,
				newRun:  nodesToAdd,
				newThr:  newThreshold,
				timeout: timeout,
			})
		require.NoError(t, err)
		doneReshare <- group
	}()
	time.Sleep(3 * time.Second)

	// TODO: How to remove this sleep? How to check when a node is at this stage
	// at this point in time, nodes should have gotten all deals and send back their responses to all nodes
	time.Sleep(getSleepDuration())

	// TODO: How to remove this sleep? How to check when a node is at this stage
	t.Log("Move to finish phase")
	dt.AdvanceMockClock(t, timeout)

	time.Sleep(getSleepDuration())

	var resharedGroup *key.Group
	select {
	case resharedGroup = <-doneReshare:
	case <-time.After(3 * time.Second):
		require.True(t, false)
	}

	t.Logf("[reshare] Group: %s", resharedGroup)

	require.NotNil(t, resharedGroup)

	transitionTime := resharedGroup.TransitionTime
	now := dt.Now().Unix()

	// move to the transition time period by period - do not skip potential
	// periods as to emulate the normal time behavior
	for now < transitionTime-1 {
		dt.AdvanceMockClock(t, beaconPeriod)
		t.Log("Check Beacon Public on Leader")
		dt.CheckPublicBeacon(dt.Ids(1, false)[0], false)
		now = dt.Now().Unix()
	}

	// move to the transition time
	dt.SetMockClock(t, resharedGroup.TransitionTime)
	time.Sleep(getSleepDuration())

	// test that all nodes in the new group have generated a new beacon
	root := dt.resharedNodes[0].drand
	rootID := root.priv.Public
	cm := root.opts.certmanager
	client := net.NewGrpcClientFromCertManager(cm)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp, err := client.PublicRand(ctx, rootID, new(drand.PublicRandRequest))
	require.NoError(t, err)

	// moving another round to make sure all nodes have time to sync in case one missed a beat
	dt.SetMockClock(t, resharedGroup.TransitionTime)
	time.Sleep(getSleepDuration())
	for _, n := range dt.resharedNodes[1:] {
		// Make sure we pull the same round from the rest of the nodes as we received from the leader
		req := &drand.PublicRandRequest{Round: resp.Round}
		t.Logf("[reshare] Requesting round %d to %s", resp.Round, n.addr)
		resp2, err := client.PublicRand(ctx, n.drand.priv.Public, req)
		if errors.Is(err, derrors.ErrNoBeaconStored) {
			t.Logf("[reshare] ErrNoBeaconStored: retrying request for %s", n.addr)
			time.Sleep(getSleepDuration())
			resp2, err = client.PublicRand(ctx, n.drand.priv.Public, req)
		}
		require.NoError(t, err)
		require.Equal(t, resp, resp2)
	}
}

// This test is where a client can stop the resharing in process and start again
//
//nolint:funlen
func TestRunDKGResharePreempt(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping testing in CI environment")
	}

	oldN := 3
	newN := 3
	Thr := 2
	timeout := 1 * time.Second
	beaconPeriod := 2 * time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, oldN, Thr, beaconPeriod, beaconID)

	group1, err := dt.RunDKG()
	require.NoError(t, err)

	dt.SetMockClock(t, group1.GenesisTime)
	err = dt.WaitUntilChainIsServing(t, dt.nodes[0])
	require.NoError(t, err)

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
	doneReshare := make(chan *key.Group, 1)
	go func() {
		g, err := dt.RunReshare(t,
			&reshareConfig{
				oldRun:  oldN,
				newThr:  Thr,
				timeout: timeout,
			})
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
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, thr, p, beaconID)

	group, err := dt.RunDKG()
	require.NoError(t, err)

	chainInfo := chain.NewChainInfo(group)
	certManager := dt.nodes[0].drand.opts.certmanager
	client := NewGrpcClientFromCert(chainInfo.Hash(), certManager)

	for i, node := range dt.nodes {
		d := node.drand
		t.Logf("Getting chain info from node %d \n", i)
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
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, thr, p, beaconID)

	group, err := dt.RunDKG()
	require.NoError(t, err)

	root := dt.nodes[0].drand
	rootID := root.priv.Public

	dt.SetMockClock(t, group.GenesisTime)
	err = dt.WaitUntilChainIsServing(t, dt.nodes[0])
	require.NoError(t, err)

	err = dt.WaitUntilRound(t, dt.nodes[0], 1)
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
//
//nolint:funlen
func TestDrandPublicStream(t *testing.T) {
	n := 4
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, thr, p, beaconID)

	group, err := dt.RunDKG()
	require.NoError(t, err)

	root := dt.nodes[0]
	rootID := root.drand.priv.Public

	dt.SetMockClock(t, group.GenesisTime)
	err = dt.WaitUntilChainIsServing(t, dt.nodes[0])
	require.NoError(t, err)

	// do a few periods
	for i := 0; i < 3; i++ {
		dt.AdvanceMockClock(t, group.Period)

		// +2 because rounds start at 1, and at genesis time, drand generates
		// first round already
		err := dt.WaitUntilRound(t, dt.nodes[0], uint64(i+2))
		require.NoError(t, err)
	}

	cm := root.drand.opts.certmanager
	client := net.NewGrpcClientFromCertManager(cm)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// get last round first
	t.Log("Getting the last round first with PublicRand method")
	resp, err := client.PublicRand(ctx, rootID, new(drand.PublicRandRequest))
	require.NoError(t, err)
	require.Equal(t, uint64(4), resp.Round)

	//  run streaming and expect responses
	req := &drand.PublicRandRequest{Round: resp.GetRound()}
	respCh, err := client.PublicRandStream(ctx, root.drand.priv.Public, req)
	require.NoError(t, err)

	// expect first round now since node already has it
	t.Log("Waiting to receive the first round as the node should have it now...")
	select {
	case beacon := <-respCh:
		t.Logf("First round rcv %d \n", beacon.GetRound())
		require.Equal(t, resp.GetRound(), beacon.GetRound())

	case <-time.After(200 * time.Millisecond):
		t.Logf("First round NOT rcv. Timeout has passed \n")
		require.True(t, false, "too late for the first round, it didn't reply in time")
	}

	nTry := 4
	// we expect the next one now
	initRound := resp.Round + 1
	maxRound := initRound + uint64(nTry)
	t.Logf("Streaming for future rounds starting from %d until round %d", initRound, maxRound)

	for round := initRound; round < maxRound; round++ {
		t.Logf("advancing clock for round %d\n", round)
		// move time to next period
		dt.AdvanceMockClock(t, group.Period)

		select {
		case beacon := <-respCh:
			require.Equal(t, round, beacon.GetRound())
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
		require.False(t, true, "should have gotten a round after time went by")
	}

	t.Logf("Streaming for past rounds starting from %d until %d", 1, maxRound+2)

	respCh, err = client.PublicRandStream(ctx, root.drand.priv.Public, &drand.PublicRandRequest{
		Round: 1,
	})
	require.NoError(t, err)

	for i := uint64(1); i < maxRound+1; i++ {
		select {
		case resp := <-respCh:
			require.Equal(t, i, resp.GetRound())
		case <-time.After(50 * time.Millisecond):
			require.False(t, true, "should have gotten all past rounds")
		}
	}

	dt.AdvanceMockClock(t, group.Period)
	select {
	case resp := <-respCh:
		t.Logf("Round %d rcv \n", maxRound)
		require.Equal(t, maxRound+1, resp.GetRound())

	case <-time.After(50 * time.Millisecond):
		require.False(t, true, "should have gotten a round after time went by")
	}

	select {
	case <-respCh:
		require.False(t, true, "shouldn't get a round if time doesn't go by")

	case <-time.After(50 * time.Millisecond):
		// correct
	}
}

func expectChanFail(t *testing.T, errCh chan error) {
	t.Helper()
	select {
	case e := <-errCh:
		if errors.Is(e, io.EOF) {
			t.Fatal("should have errored but got EOF")
		}
		t.Logf("An error was received as expected: %v", e)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("An error should have been received.")
	}
}

// This test makes sure the "FollowChain" grpc method works fine
//
//nolint:funlen // This is a test function
func TestDrandFollowChain(t *testing.T) {
	n, p := 4, 1*time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, key.DefaultThreshold(n), p, beaconID)

	group, err := dt.RunDKG()
	require.NoError(t, err)
	rootID := dt.nodes[0].drand.priv.Public

	dt.SetMockClock(t, group.GenesisTime)
	err = dt.WaitUntilChainIsServing(t, dt.nodes[0])
	require.NoError(t, err)

	// do a few periods
	for i := 0; i < 6; i++ {
		dt.AdvanceMockClock(t, group.Period)

		// +2 because rounds start at 1, and at genesis time, drand generates
		// first round already
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
	t.Logf(" \t [-] Trying to follow with an invalid hash\n")
	ctx, cancel = context.WithCancel(context.Background())
	_, errCh, _ := newClient.StartFollowChain(ctx, "deadbeef", addrToFollow, tls, 10000, beaconID)
	expectChanFail(t, errCh)
	cancel()

	// testing with a non hex hash
	t.Logf(" \t [-] Trying to follow with a non-hex hash\n")
	ctx, cancel = context.WithCancel(context.Background())
	_, _, err = newClient.StartFollowChain(ctx, "tutu", addrToFollow, tls, 10000, beaconID)
	require.Error(t, err)
	cancel()

	// testing with a invalid beaconID
	t.Logf("T \t [-] rying to follow with an invalid beaconID\n")
	ctx, cancel = context.WithCancel(context.Background())
	_, errCh, _ = newClient.StartFollowChain(ctx, hash, addrToFollow, tls, 10000, "tutu")
	expectChanFail(t, errCh)
	cancel()

	fn := func(upTo, exp uint64) {
		ctx, cancel := context.WithCancel(context.Background())

		t.Logf(" \t [-] Starting to follow chain with a valid hash. %d <= %d \n", upTo, exp)
		t.Logf(" \t\t --> beaconID: %s ; hash-chain: %s", beaconID, hash)
		progress, errCh, err := newClient.StartFollowChain(ctx, hash, addrToFollow, tls, upTo, beaconID)
		require.NoError(t, err)

		for goon := true; goon; {
			select {
			case p, ok := <-progress:
				t.Logf(" \t\t --> Received progress: %d / %d \n", p.Current, p.Target)
				if ok && p.Current == exp {
					t.Logf("\t\t -->Successful beacon rcv. Round: %d.", exp)
					goon = false
				}
			case e := <-errCh:
				if errors.Is(e, io.EOF) { // means we've reached the end
					t.Logf("\t\t -->Got EOF from daemon.")
					goon = false
					break
				}
				t.Logf("\t\t -->Unexpected error received: %v.", e)
				require.NoError(t, e)
			case <-time.After(2 * time.Second):
				t.Fatalf("\t\t --> Timeout during test")
			}
		}

		t.Logf(" \t\t --> Done, proceeding to check store now.\n")

		// cancel the operation
		cancel()

		// (Postgres) Database operations need to have a proper context to work.
		// We create a new one, since we canceled the previous one.
		ctx = context.Background()

		// check if the beacon is in the database
		store := newNode.drand.dbStore
		if newNode.drand.opts.dbStorageEngine == chain.BoltDB {
			store, err = newNode.drand.createDBStore(ctx)
			require.NoError(t, err)
		}
		require.NoError(t, err)
		defer store.Close(ctx)

		lastB, err := store.Last(ctx)
		require.NoError(t, err)
		require.Equal(t, exp, lastB.Round, "found %d vs expected %d", lastB.Round, exp)
	}

	fn(resp.GetRound()-2, resp.GetRound()-2)
	fn(0, resp.GetRound())
}

// This test makes sure the "StartCheckChain" grpc method works fine
//
//nolint:funlen
func TestDrandCheckChain(t *testing.T) {
	cfg := Config{}
	WithTestDB(t, "")[0](&cfg)
	if cfg.dbStorageEngine == chain.MemDB {
		t.Skip(`This test does not work with in-memory database. See the "// Skip why: " comment for details.`)
	}

	ctx, _, prevMatters := context2.PrevSignatureMattersOnContext(t, context.Background())

	n, p := 4, 1*time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, key.DefaultThreshold(n), p, beaconID)

	group, err := dt.RunDKG()
	require.NoError(t, err)
	rootID := dt.nodes[0].drand.priv.Public

	dt.SetMockClock(t, group.GenesisTime)
	err = dt.WaitUntilChainIsServing(t, dt.nodes[0])
	require.NoError(t, err)

	// do a few periods
	for i := 0; i < 6; i++ {
		dt.AdvanceMockClock(t, group.Period)

		// +2 because rounds start at 1, and at genesis time, drand generates
		// first round already
		err := dt.WaitUntilRound(t, dt.nodes[0], uint64(i+2))
		require.NoError(t, err)
	}

	client := net.NewGrpcClientFromCertManager(dt.nodes[0].drand.opts.certmanager)
	ctx, cancel := context.WithCancel(ctx)

	// get last round first
	resp, err := client.PublicRand(ctx, rootID, new(drand.PublicRandRequest))
	require.NoError(t, err)
	current := resp.GetRound()

	t.Log(current)

	ctrlClient, err := net.NewControlClient(dt.nodes[0].drand.opts.controlPort)
	require.NoError(t, err)
	tls := true

	// First try with an invalid hash info
	t.Logf("Trying to resync with an invalid address\n")

	_, errCh, _ := ctrlClient.StartCheckChain(context.Background(), "deadbeef", nil, tls, 10000, beaconID)
	expectChanFail(t, errCh)

	// Next trying with a fully valid chain
	cancel()
	ctx, cancel = context.WithCancel(context.Background())
	hash := fmt.Sprintf("%x", chain.NewChainInfo(group).Hash())
	addrToFollow := []string{rootID.Address()}
	upTo := uint64(5)

	t.Logf(" \t [-] Starting resync chain with a valid hash.")
	t.Logf(" \t\t --> beaconID: %s ; hash-chain: %s", beaconID, hash)
	progress, errCh, err := ctrlClient.StartCheckChain(ctx, hash, addrToFollow, tls, upTo, beaconID)
	require.NoError(t, err)
	consumeProgress(t, progress, errCh, upTo, true)
	// check that progress is (0, 0)
	consumeProgress(t, progress, errCh, 0, false)

	t.Logf(" \t\t --> Done, canceling.\n")
	cancel()
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	t.Logf(" \t\t --> Stopping node.s\n")
	dt.StopMockNode(dt.nodes[0].addr, false)

	t.Logf(" \t\t --> Done, proceeding to modify store now.\n")
	store := dt.nodes[0].drand.dbStore
	if dt.nodes[0].drand.opts.dbStorageEngine == chain.BoltDB {
		store, err = dt.nodes[0].drand.createDBStore(ctx)
		require.NoError(t, err)
	}

	t.Logf(" \t\t --> Opened store. Getting 4th beacon\n")
	beac, err := store.Get(ctx, upTo-1)
	require.NoError(t, err)
	require.Equal(t, upTo-1, beac.Round, "found %d vs expected %d", beac.Round, upTo-1)

	t.Logf(" \t\t --> Deleting 4th beacon.\n")
	err = store.Del(ctx, upTo-1)
	require.NoError(t, err)
	err = store.Close(ctx)
	require.NoError(t, err)

	t.Logf(" \t\t --> Re-Starting node.\n")

	// Skip why: This call will create a new database connection.
	//  However, for the MemDB engine type, this means we create a new backing array from scratch
	//  thus removing all previous items from memory. At that point, this invalidates the test.
	dt.StartDrand(t, dt.nodes[0].addr, false, false)

	t.Logf(" \t\t --> Making sure the beacon is now missing.\n")
	_, err = client.PublicRand(ctx, rootID, &drand.PublicRandRequest{Round: upTo - 1})
	require.Error(t, err)

	t.Logf(" \t\t --> Re-Running resync in dry run.\n")
	progress, errCh, err = ctrlClient.StartCheckChain(ctx, hash, addrToFollow, tls, upTo, beaconID)
	require.NoError(t, err)
	consumeProgress(t, progress, errCh, upTo, true)
	// check that progress is (0, 1)

	// The reason why this is 2 is that one missing beacon from the database and will affect
	// the next beacon too. The storage layer cannot compose the chain correctly and a
	// database heal is now required.
	incorrectBeacons := uint64(2)
	if !prevMatters {
		incorrectBeacons = 1
	}

	consumeProgress(t, progress, errCh, incorrectBeacons, false)

	// we wait to make sure everything is done on the sync manager side before testing.
	time.Sleep(time.Second)

	// check dry-run worked and we still get an error
	_, err = client.PublicRand(ctx, rootID, &drand.PublicRandRequest{Round: upTo - 1})
	require.Error(t, err)

	t.Logf(" \t\t --> Re-Running resync and correct the error.\n")
	progress, errCh, err = ctrlClient.StartCheckChain(ctx, hash, nil, tls, upTo, beaconID)
	require.NoError(t, err)
	consumeProgress(t, progress, errCh, upTo, true)
	// check that progress is (0, 1)
	consumeProgress(t, progress, errCh, incorrectBeacons, false)
	// goes on with correcting the chain
	consumeProgress(t, progress, errCh, incorrectBeacons, true)
	// now the progress chan should be closed
	_, ok := <-progress
	require.False(t, ok)

	// we wait to make sure everything is done on the sync manager side before testing.
	time.Sleep(time.Second)

	// check we don't get an error anymore
	resp, err = client.PublicRand(ctx, rootID, &drand.PublicRandRequest{Round: upTo - 1})
	require.NoError(t, err)
	require.Equal(t, upTo-1, resp.Round)
}

// Test if we can correctly fetch the rounds through the local proxy
func TestDrandPublicStreamProxy(t *testing.T) {
	n := 4
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, thr, p, beaconID)

	group, err := dt.RunDKG()
	require.NoError(t, err)

	root := dt.nodes[0]
	dt.SetMockClock(t, group.GenesisTime)
	err = dt.WaitUntilChainIsServing(t, dt.nodes[0])
	if err != nil {
		t.Log("Error waiting until chain is serving:", err)
		t.Fail()
	}

	// do a few periods
	for i := 0; i < 3; i++ {
		dt.AdvanceMockClock(t, group.Period)
		// +2 because rounds start at 1, and at genesis time, drand generates
		// first round already
		err := dt.WaitUntilRound(t, dt.nodes[0], uint64(i+2))
		require.NoError(t, err)
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
		err := dt.WaitUntilRound(t, dt.nodes[0], round)
		require.NoError(t, err)

		beacon, ok = <-rc

		require.True(t, ok)

		t.Logf("Round received %d", beacon.Round())
		require.Equal(t, round, beacon.Round())
	}
}

func TestReshareWithInvalidBeaconIdInMetadataFailsButNoSegfault(t *testing.T) {
	n := 3
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, thr, p, beaconID)
	_, err := dt.RunDKG()
	require.NoError(t, err)

	nonsenseBeaconID := "completely made up"
	resharePacket := drand.InitResharePacket{
		Old: &drand.GroupInfo{
			Location: &drand.GroupInfo_Path{
				Path: "http://localhost:8080",
			},
		},
		Info: &drand.SetupInfoPacket{
			Leader:        false,
			LeaderAddress: "whatever",
			LeaderTls:     false,
			Nodes:         uint32(n),
			Threshold:     uint32(thr),
			Timeout:       100,
			BeaconOffset:  100,
			DkgOffset:     1,
			Secret:        []byte("doesntmatter"),
			Force:         false,
		},
		Metadata: &common.Metadata{
			BeaconID: nonsenseBeaconID,
		},
	}
	_, err = dt.nodes[1].daemon.InitReshare(context.Background(), &resharePacket)
	require.EqualError(
		t,
		err,
		"beacon with ID "+nonsenseBeaconID+" could not be found - make sure you have passed the id flag or have a default beacon",
	)
}

func TestReshareWithoutOldGroupFailsButNoSegfault(t *testing.T) {
	n := 3
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, thr, p, beaconID)
	_, err := dt.RunDKG()
	require.NoError(t, err)

	resharePacket := drand.InitResharePacket{
		Old: nil,
		Info: &drand.SetupInfoPacket{
			Leader:        false,
			LeaderAddress: "whatever",
			LeaderTls:     false,
			Nodes:         uint32(n),
			Threshold:     uint32(thr),
			Timeout:       100,
			BeaconOffset:  100,
			DkgOffset:     1,
			Secret:        []byte("doesntmatter"),
			Force:         false,
		},
	}

	_, err = dt.nodes[1].daemon.InitReshare(context.Background(), &resharePacket)
	require.EqualError(t, err, "cannot reshare without an old group")
}

func TestModifyingGroupFileManuallyDoesNotSegfault(t *testing.T) {
	// set up 3 nodes for a test
	n := 3
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, thr, p, beaconID)

	node := dt.nodes[0]
	dir := dt.dir
	priv := node.drand.priv

	// set a persistent keystore, as the normal test ones are ephemeral
	store := key.NewFileStore(dir, beaconID)
	node.drand.store = store

	// save the key pair, as this was done ephemerally inside of `NewDrandTestScenario` >.>
	err := store.SaveKeyPair(priv)
	require.NoError(t, err)

	// run a DKG so that every node gets a group file and key share
	_, err = dt.RunDKG()
	require.NoError(t, err)

	// stop the node and wait for it
	node.daemon.Stop(context.Background())
	<-node.daemon.exitCh

	// modify your entry (well, all of them!) in the group file to change the TLS status
	groupPath := path.Join(dir, beaconID, key.GroupFolderName, "drand_group.toml")

	// read
	groupFileReader, err := fs.Open(groupPath)
	require.NoError(t, err)
	groupFile, err := io.ReadAll(groupFileReader)
	require.NoError(t, err)
	// write
	err = os.WriteFile(groupPath, []byte(strings.ReplaceAll(string(groupFile), "true", "false")), 0o740)
	require.NoError(t, err)

	// try and reload the beacon from the store
	// the updated TLS status will fail verification
	_, err = node.daemon.LoadBeaconFromStore(beaconID, store)

	require.EqualError(t, err, "could not restore beacon info for the given identity - this can happen if you updated the group file manually")
}

func TestDKGWithMismatchedSchemes(t *testing.T) {
	t.Setenv("DRAND_TEST_LOGS", "DEBUG")
	beaconID := "blah"
	scenario := NewDrandTestScenario(t, 2, 2, 1*time.Second, beaconID)

	// to dedupe it when we're running the tests with different default schemes
	if os.Getenv("SCHEME_ID") == crypto.ShortSigSchemeID {
		scenario.scheme = crypto.NewPedersenBLSChained()
	} else {
		scenario.scheme = crypto.NewPedersenBLSUnchainedSwapped()
	}

	t.Setenv("SCHEME_ID", scenario.scheme.Name)
	scenario.AddNodesWithOptions(t, 1, beaconID)
	t.Setenv("SCHEME_ID", "")

	_, err := scenario.RunDKG()
	require.ErrorContainsf(t, err, key.ErrInvalidKeyScheme.Error(), "expected node to fail DKG due to mismatch of schemes")
}
