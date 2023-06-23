package core

import (
	"context"
	"errors"
	"fmt"
	"github.com/drand/drand/common"
	dkg3 "github.com/drand/drand/protobuf/crypto/dkg"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	"io"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	chain2 "github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/key"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/internal/chain"
	derrors "github.com/drand/drand/internal/chain/errors"
	dkg2 "github.com/drand/drand/internal/dkg"
	"github.com/drand/drand/internal/net"
	"github.com/drand/drand/internal/test"
	context2 "github.com/drand/drand/internal/test/context"
	"github.com/drand/drand/internal/test/testlogger"
	"github.com/drand/drand/protobuf/drand"
	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setFDLimit(t testing.TB) {
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
					t.Logf("\t\t --> Got EOF from daemon.\n")
					return
				}
				t.Logf("\t\t --> Unexpected error received: %v.\n", e)
				require.NoError(t, e)
			}
		}()

		for {
			select {
			case p, ok := <-progress:
				if ok && p.Current == amount {
					t.Logf("\t\t --> Successful chain sync progress. Achieved round: %d.\n", amount)
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
				t.Logf("\t\t --> Got EOF from daemon.\n")
				return
			}
			t.Logf("\t\t -->Unexpected error received: %v.\n", e)
			require.NoError(t, e)
		case <-time.After(2 * time.Second):
			t.Fatalf("\t\t --> Timeout during test")
			return
		}
	}
}

// Test that the normal dkg process works correctly
func TestRunDKG(t *testing.T) {
	n := 4
	expectedBeaconPeriod := 5 * time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, key.DefaultThreshold(n), expectedBeaconPeriod, beaconID, clockwork.NewFakeClock())

	group, err := dt.RunDKG(t)
	require.NoError(t, err)

	t.Log(group)

	assert.Equal(t, 3, group.Threshold)
	assert.Equal(t, expectedBeaconPeriod, group.Period)
	assert.Equal(t, time.Duration(0), group.CatchupPeriod)
	assert.Equal(t, n, len(group.Nodes))
}

// Test dkg for a large quantity of nodes (22 nodes)
func TestRunDKGLarge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	setFDLimit(t)

	n := 22
	thr := 12
	expectedBeaconPeriod := 5 * time.Second
	beaconID := test.GetBeaconIDFromEnv()

	// we need to increase some DKG timings for bigger DKGs!
	dt := NewDrandTestScenario(
		t,
		n,
		key.DefaultThreshold(n),
		expectedBeaconPeriod,
		beaconID,
		clockwork.NewFakeClock(),
		WithDkgKickoffGracePeriod(15*time.Second),
		WithDkgPhaseTimeout(45*time.Second),
		WithDkgTimeout(3*time.Minute),
	)

	// let's wait for the last node to be started
	for {
		_, err := dt.nodes[n-1].drand.PingPong(context.Background(), &drand.Ping{})
		if err == nil {
			break
		}
	}

	group, err := dt.RunDKG(t)
	require.NoError(t, err)

	assert.Equal(t, thr, group.Threshold)
	assert.Equal(t, expectedBeaconPeriod, group.Period)
	assert.Equal(t, time.Duration(0), group.CatchupPeriod)
	assert.Equal(t, n, len(group.Nodes))
}

// Test Start/Stop after DKG
// Run DKG
// Stop last node
// Restart last node and wait catch up
// Check beacon still works and length is correct
func TestDrandDKGFresh(t *testing.T) {
	ctx := context.Background()
	n := 4
	beaconPeriod := 1 * time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, key.DefaultThreshold(n), beaconPeriod, beaconID, clockwork.NewFakeClockAt(time.Now()))

	// Run DKG
	finalGroup, err := dt.RunDKG(t)
	require.NoError(t, err)

	// make the last node fail (stop)
	lastNode := dt.nodes[n-1]
	restOfNodes := dt.nodes[:n-1]

	t.Logf("Stop last node %s\n", lastNode.addr)
	dt.StopMockNode(lastNode.addr, false)

	// move time to genesis
	dt.SetMockClock(t, finalGroup.GenesisTime)
	t.Logf("Time = %d\n", finalGroup.GenesisTime)

	// two = genesis + 1st round (happens at genesis)
	t.Log("Check Beacon Length")
	dt.CheckBeaconLength(t, restOfNodes, 2)

	t.Logf("Start last node %s\n", lastNode.addr)
	dt.StartDrand(ctx, t, lastNode.addr, true, false)

	// The catchup process will finish when node gets the previous beacons (1st round)
	err = dt.WaitUntilRound(t, lastNode, 1)
	require.NoError(t, err)

	dt.AdvanceMockClock(t, beaconPeriod)

	t.Log("Check Beacon Length")
	dt.CheckBeaconLength(t, dt.nodes, 3)

	t.Log("Check Beacon Public")
	response := dt.CheckPublicBeacon(ctx, lastNode.addr, false)
	require.Equal(t, uint64(2), response.Round)
}

// Test dkg when two nodes cannot broadcast messages between them. The rest of the nodes
// will be able to broadcast messages, so the process should finish successfully
// Given 4 nodes = [0, 1, 2, 3]
// 1. Limit communication between 1 and 2
// 2. Run DKG
// 3. Run reshare
func TestRunDKGBroadcastDeny(t *testing.T) {
	n := 4
	thr := 3
	beaconPeriod := 1 * time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, thr, beaconPeriod, beaconID, clockwork.NewFakeClockAt(time.Now()))

	// close connection between a pair of nodes
	node1 := dt.nodes[1]
	node2 := dt.nodes[2]

	t.Log("Setting node 1 not to broadcast messages to the node 2")
	node1.drand.DenyBroadcastTo(t, node2.addr)

	t.Log("Setting node 2 not to broadcast messages to the node 1")
	node2.drand.DenyBroadcastTo(t, node1.addr)

	group1, err := dt.RunDKG(t)
	require.NoError(t, err)

	// Advance clock
	dt.SetMockClock(t, group1.GenesisTime)
	dt.AdvanceMockClock(t, 1*time.Second)

	group2, err := dt.RunReshare(t, dt.nodes, nil)
	require.NoError(t, err)
	require.NotNil(t, group2)

	t.Log("Resharing complete")
}

// This tests when a node first signal his intention to participate in a resharing
// and received the execution kickoff but does not participate in the DKG execution
// the node should be left out of the group file
func TestRunDKGReshareAbsentNodeDuringExecution(t *testing.T) {
	oldNodeCount := 3
	newNodeCount := 4
	oldThreshold := 2
	beaconPeriod := 2 * time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, oldNodeCount, oldThreshold, beaconPeriod, beaconID, clockwork.NewFakeClockAt(time.Now()))

	group1, err := dt.RunDKG(t)
	require.NoError(t, err)

	dt.SetMockClock(t, group1.GenesisTime)
	err = dt.WaitUntilChainIsServing(t, dt.nodes[0])
	require.NoError(t, err)

	t.Log("Check Beacon Length")
	dt.CheckBeaconLength(t, dt.nodes, 2)

	// so nodes think they are going forward with round 2
	dt.AdvanceMockClock(t, 1*time.Second)

	t.Log("Adding new nodes to the group")
	nodesToAdd := newNodeCount - oldNodeCount
	currentNodes := dt.nodes[0:]
	newNodes := dt.SetupNewNodes(t, nodesToAdd)

	// we want to stop one node right after the execution has been started, so it doesn't participate in the DKG
	// messaging phase
	nodeIndexToStop := 1
	nodeToStop := currentNodes[nodeIndexToStop]

	t.Log("Setup reshare done. Starting reshare... Ignoring reshare errors")
	hooks := lifecycleHooks{
		postExecutionStart: func() {
			t.Logf("Stopping node %d for test: %s \n", nodeIndexToStop, nodeToStop.addr)
			nodeToStop.daemon.Stop(context.Background())
			<-nodeToStop.daemon.WaitExit()
			t.Logf("Node %d stopped \n", nodeIndexToStop)
		},
	}
	newGroup, err := dt.RunReshareWithHooks(t, currentNodes, newNodes, hooks)
	require.NoError(t, err)
	require.NotNil(t, newGroup)

	// the node that had stopped must not be in the group
	t.Logf("Check node %d is not included in the group \n", nodeIndexToStop)
	missingPublic := nodeToStop.drand.priv.Public
	require.Nil(t, newGroup.Find(missingPublic), "missing public is found", missingPublic)
}

// This tests when a node first signal his intention to participate in a resharing
// and does not receive the execution message. The DKG should continue regardlee and
// the node should be left out of the final group file
func TestRunDKGReshareAbsentNodeForExecutionStart(t *testing.T) {
	oldNodeCount := 3
	newNodeCount := 4
	oldThreshold := 2
	beaconPeriod := 2 * time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, oldNodeCount, oldThreshold, beaconPeriod, beaconID, clockwork.NewFakeClockAt(time.Now()))

	group1, err := dt.RunDKG(t)
	require.NoError(t, err)

	dt.SetMockClock(t, group1.GenesisTime)
	// Note: Removing this sleep will cause the test to randomly break.
	time.Sleep(1 * time.Second)
	err = dt.WaitUntilChainIsServing(t, dt.nodes[0])
	require.NoError(t, err)

	t.Log("Check Beacon Length")
	dt.CheckBeaconLength(t, dt.nodes, 2)

	// so nodes think they are going forward with round 2
	dt.AdvanceMockClock(t, 1*time.Second)

	t.Log("Adding new nodes to the group")
	nodesToAdd := newNodeCount - oldNodeCount
	currentNodes := dt.nodes[0:]
	newNodes := dt.SetupNewNodes(t, nodesToAdd)

	// we want to stop one node right after they accept the DKG but before its execution has been kicked off
	nodeIndexToStop := 1
	nodeToStop := dt.nodes[nodeIndexToStop]

	t.Log("Setup reshare done. Starting reshare... Ignoring reshare errors")
	hooks := lifecycleHooks{
		postAcceptance: func() {
			t.Logf("Stopping node for test: %s \n", nodeToStop.addr)
			nodeToStop.daemon.Stop(context.Background())
			<-nodeToStop.daemon.WaitExit()
			t.Logf("Node %d stopped \n", nodeIndexToStop)
		},
	}
	newGroup, err := dt.RunReshareWithHooks(t, currentNodes, newNodes, hooks)
	require.NoError(t, err)
	require.NotNil(t, newGroup)

	// the node that had stopped must not be in the group
	t.Logf("Check node %d is not included in the group \n", nodeIndexToStop)
	missingPublic := nodeToStop.drand.priv.Public
	require.Nil(t, newGroup.Find(missingPublic), "missing public is found", missingPublic)
}

// ??? I am slightly puzzled by this test - the original did not do what the name said it does
// ??? and now it's basically duplicate of some others
// The test creates the scenario where one node made a complaint during the DKG, at the second phase, so normally,
// there should be a "Justification" at the third phase. In this case, there is not. This scenario
// can happen if there is an offline node right at the beginning of DKG that don't even send any message.
//
//nolint:funlen
func TestRunDKGReshareTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	oldNodes, newNodes, oldThreshold := 3, 4, 2
	beaconPeriod := 2 * time.Second
	offline := 1
	beaconID := test.GetBeaconIDFromEnv()
	sleepDuration := 100 * time.Millisecond

	dt := NewDrandTestScenario(t, oldNodes, oldThreshold, beaconPeriod, beaconID, clockwork.NewFakeClockAt(time.Now()))

	group1, err := dt.RunDKG(t)
	require.NoError(t, err)

	dt.SetMockClock(t, group1.GenesisTime)
	err = dt.WaitUntilChainIsServing(t, dt.nodes[0])
	require.NoError(t, err)

	t.Log("Check Beacon Length")
	dt.CheckBeaconLength(t, dt.nodes, 2)

	// so nodes think they are going forward with round 2
	dt.AdvanceMockClock(t, beaconPeriod)

	// + offline makes sure t
	nodesToKeep := oldNodes - offline
	nodesToAdd := newNodes - nodesToKeep
	dt.SetupNewNodes(t, nodesToAdd)

	t.Log("Setup reshare done. Starting reshare.")

	resharedGroup, err := dt.RunReshare(t, dt.nodes, nil)
	require.NoError(t, err)
	require.NotNil(t, resharedGroup)
	t.Logf("[reshare] Group: %s\n", resharedGroup)

	for {
		dt.AdvanceMockClock(t, beaconPeriod)
		time.Sleep(sleepDuration)
		dt.CheckPublicBeacon(ctx, dt.NodeAddresses(1, false)[0], false)
		if dt.clock.Now().Unix() > resharedGroup.TransitionTime {
			break
		}
	}

	for i := 0; i < 3; i++ {
		dt.AdvanceMockClock(t, beaconPeriod)
		time.Sleep(sleepDuration)
	}

	// test that all nodes in the new group have generated a new beacon
	root := dt.resharedNodes[0].drand
	rootID := root.priv.Public
	cm := root.opts.certmanager
	client := net.NewGrpcClientFromCertManager(testlogger.New(t), cm)

	resp, err := client.PublicRand(ctx, rootID, new(drand.PublicRandRequest))
	require.NoError(t, err)

	dt.AdvanceMockClock(t, beaconPeriod)
	time.Sleep(sleepDuration)

	// moving another round to make sure all nodes have time to sync in case one missed a beat
	dt.SetMockClock(t, resharedGroup.TransitionTime)
	time.Sleep(sleepDuration)

	dt.AdvanceMockClock(t, dt.period)
	time.Sleep(sleepDuration)

	for _, n := range dt.resharedNodes[1:] {
		// Make sure we pull the same round from the rest of the nodes as we received from the leader
		req := &drand.PublicRandRequest{Round: resp.Round}
		t.Logf("[reshare] Requesting round %d to %s\n", resp.Round, n.addr)
		resp2, err := client.PublicRand(ctx, n.drand.priv.Public, req)
		if errors.Is(err, derrors.ErrNoBeaconStored) {
			t.Logf("[reshare] ErrNoBeaconStored: retrying request for %s\n", n.addr)
			time.Sleep(beaconPeriod)
			resp2, err = client.PublicRand(ctx, n.drand.priv.Public, req)
		}
		require.NoError(t, err)
		require.Equal(t, resp, resp2)
	}
}

// this aborts a DKG and then runs another straight after successfully
func TestAbortDKGAndStartANewOne(t *testing.T) {
	l := testlogger.New(t)
	n := 4
	expectedBeaconPeriod := 5 * time.Second
	beaconID := test.GetBeaconIDFromEnv()
	dt := NewDrandTestScenario(t, n, key.DefaultThreshold(n), expectedBeaconPeriod, beaconID, clockwork.NewFakeClockAt(time.Now()))

	// first lets run a successful initial DKG
	group, err := dt.RunDKG(t)
	require.NoError(t, err)

	assert.Equal(t, n, len(group.Nodes))

	hooks := lifecycleHooks{
		postAcceptance: func() {
			leader := dt.nodes[0]
			leaderClient, err := net.NewDKGControlClient(l, leader.drand.opts.controlPort)
			require.NoError(t, err)

			// trigger an abort
			err = leader.dkgRunner.Abort()
			require.NoError(t, err)

			// ensure that the abort has indeed been stored on the leader node and haven't updated their epoch
			leaderStatus, err := leaderClient.DKGStatus(context.Background(), &drand.DKGStatusRequest{
				BeaconID: beaconID,
			})
			require.NoError(t, err)
			require.Equal(t, uint32(dkg2.Aborted), leaderStatus.Current.State)
			require.Equal(t, uint32(1), leaderStatus.Complete.Epoch)

			time.Sleep(1 * time.Second)
			// ensure that the followers also have the aborted status and haven't updated their epoch
			follower := dt.nodes[1]
			followerClient, err := net.NewDKGControlClient(l, follower.drand.opts.controlPort)
			require.NoError(t, err)
			followerStatus, err := followerClient.DKGStatus(context.Background(), &drand.DKGStatusRequest{
				BeaconID: beaconID,
			})
			require.NoError(t, err)
			require.Equal(t, uint32(dkg2.Aborted), followerStatus.Current.State)
			require.Equal(t, uint32(1), followerStatus.Complete.Epoch)
		},
	}

	// naturally, we want the reshare to have errored!
	_, err = dt.RunReshareWithHooks(t, dt.nodes, nil, hooks)
	require.Error(t, err)

	// now we re-run it without the abort and it should succeed
	_, err = dt.RunReshare(t, dt.nodes, nil)
	require.NoError(t, err)
}

// Check they all have same chain info
func TestDrandPublicChainInfo(t *testing.T) {
	ctx := context.Background()
	n := 10
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, thr, p, beaconID, clockwork.NewFakeClockAt(time.Now()))

	group, err := dt.RunDKG(t)
	require.NoError(t, err)

	lg := testlogger.New(t)
	chainInfo := chain2.NewChainInfo(lg, group)
	certManager := dt.nodes[0].drand.opts.certmanager
	client := NewGrpcClientFromCert(lg, chainInfo.Hash(), certManager)

	for i, node := range dt.nodes {
		d := node.drand
		t.Logf("Getting chain info from node %d \n", i)
		received, err := client.ChainInfo(ctx, d.priv.Public)

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
}

// Test if we can correctly fetch the rounds after a DKG using the PublicRand RPC call
//
//nolint:funlen // This is a longer test function
func TestDrandPublicRand(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.Skip("test is flacky in CI")
	}
	n := 4
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, thr, p, beaconID, clockwork.NewFakeClockAt(time.Now()))

	group, err := dt.RunDKG(t)
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
	client := net.NewGrpcClientFromCertManager(testlogger.New(t), cm)

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

	newN := 5
	toAdd := newN - n
	newNodes := dt.SetupNewNodes(t, toAdd)
	newGroup, err := dt.RunReshare(t, dt.nodes, newNodes)
	require.NoError(t, err)
	require.NotNil(t, newGroup)
	dt.SetMockClock(t, newGroup.TransitionTime)
	time.Sleep(newGroup.Period)
	// do a few periods
	for i := 0; i < 2; i++ {
		dt.AdvanceMockClock(t, newGroup.Period)
		time.Sleep(newGroup.Period)
	}
	// then ask the new node about a previous randomness
	newNodeID := newNodes[0].drand.priv.Public
	resp, err = client.PublicRand(ctx, newNodeID, &drand.PublicRandRequest{Round: initRound})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

// Test if the we can correctly fetch the rounds after a DKG using the
// PublicRandStream RPC call
// It also test the follow method call (it avoid redoing an expensive and long
// setup on CI to test both behaviors).
//
//nolint:funlen
func TestDrandPublicStream(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.Skip("test is flacky in CI")
	}
	n := 4
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	beaconID := test.GetBeaconIDFromEnv()
	sleepDuration := 100 * time.Millisecond

	dt := NewDrandTestScenario(t, n, thr, p, beaconID, clockwork.NewFakeClockAt(time.Now()))

	group, err := dt.RunDKG(t)
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
	client := net.NewGrpcClientFromCertManager(testlogger.New(t), cm)

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

	case <-time.After(1 * time.Second):
		t.Logf("First round NOT rcv. Timeout has passed \n")
		require.True(t, false, "too late for the first round, it didn't reply in time")
	}

	nTry := 4
	// we expect the next one now
	initRound := resp.Round + 1
	maxRound := initRound + uint64(nTry)
	t.Logf("Streaming for future rounds starting from %d until round %d\n", initRound, maxRound)

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
	t.Logf("Streaming for rounds starting from %d to %d\n", 0, maxRound)

	respCh, err = client.PublicRandStream(ctx, root.drand.priv.Public, new(drand.PublicRandRequest))
	require.NoError(t, err)

	select {
	case <-respCh:
		require.False(t, true, "shouldn't get a round if time doesn't go by")
	case <-time.After(1 * time.Second):
		// correct
	}

	dt.AdvanceMockClock(t, group.Period)
	select {
	case resp := <-respCh:
		t.Logf("Round %d rcv \n", maxRound)
		require.Equal(t, maxRound, resp.GetRound())
	case <-time.After(1 * time.Second):
		require.False(t, true, "should have gotten a round after time went by")
	}

	t.Logf("Streaming for past rounds starting from %d until %d\n", 1, maxRound+2)

	respCh, err = client.PublicRandStream(ctx, root.drand.priv.Public, &drand.PublicRandRequest{
		Round: 1,
	})
	require.NoError(t, err)

	for i := uint64(1); i < maxRound+1; i++ {
		select {
		case resp := <-respCh:
			require.Equal(t, i, resp.GetRound())
		case <-time.After(1 * time.Second):
			require.False(t, true, "should have gotten all past rounds")
		}
	}

	dt.AdvanceMockClock(t, group.Period)
	time.Sleep(sleepDuration)

	select {
	case resp := <-respCh:
		t.Logf("Round %d rcv \n", maxRound)
		require.Equal(t, maxRound+1, resp.GetRound())
	case <-time.After(1 * time.Second):
		require.False(t, true, "should have gotten a round after time went by")
	}

	select {
	case <-respCh:
		require.False(t, true, "shouldn't get a round if time doesn't go by")
	case <-time.After(1 * time.Second):
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
		t.Logf("An error was received as expected: %v\n", e)
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

	dt := NewDrandTestScenario(t, n, key.DefaultThreshold(n), p, beaconID, clockwork.NewFakeClockAt(time.Now()))

	group, err := dt.RunDKG(t)
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

	client := net.NewGrpcClientFromCertManager(dt.nodes[0].drand.log, dt.nodes[0].drand.opts.certmanager)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// get last round first
	resp, err := client.PublicRand(ctx, rootID, new(drand.PublicRandRequest))
	require.NoError(t, err)

	// TEST setup a new node and fetch history
	newNode := dt.SetupNewNodes(t, 1)[0]
	newClient, err := net.NewControlClient(newNode.drand.log, newNode.drand.opts.controlPort)
	require.NoError(t, err)

	addrToFollow := []string{rootID.Address()}
	hash := fmt.Sprintf("%x", chain2.NewChainInfo(testlogger.New(t), group).Hash())
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

	// testing with an invalid beaconID
	t.Logf(" \t [-] Trying to follow with an invalid beaconID\n")
	ctx, cancel = context.WithCancel(context.Background())
	_, errCh, _ = newClient.StartFollowChain(ctx, hash, addrToFollow, tls, 10000, "tutu")
	expectChanFail(t, errCh)
	cancel()

	fn := func(upTo, exp uint64) {
		ctx, cancel := context.WithCancel(context.Background())

		t.Logf(" \t [+] Starting to follow chain with a valid hash. %d <= %d \n", upTo, exp)
		t.Logf(" \t\t --> beaconID: %s ; hash-chain: %s", beaconID, hash)
		progress, errCh, err := newClient.StartFollowChain(ctx, hash, addrToFollow, tls, upTo, beaconID)
		require.NoError(t, err)

		for goon := true; goon; {
			select {
			case p, ok := <-progress:
				t.Logf(" \t\t --> Received progress: %d / %d \n", p.Current, p.Target)
				if ok && p.Current == exp {
					t.Logf("\t\t -->Successful beacon rcv. Round: %d.\n", exp)
					goon = false
				}
			case e := <-errCh:
				if errors.Is(e, io.EOF) { // means we've reached the end
					t.Logf("\t\t -->Got EOF from daemon.\n")
					goon = false
					break
				}
				t.Logf("\t\t -->Unexpected error received: %v.\n", e)
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
		ctx, cancel = context.WithCancel(context.Background())
		defer cancel()

		// check if the beacon is in the database
		store := newNode.drand.dbStore
		if newNode.drand.opts.dbStorageEngine == chain.BoltDB {
			store, err = newNode.drand.createDBStore(ctx)
			require.NoError(t, err)
		}
		require.NoError(t, err)
		defer store.Close()

		lastB, err := store.Last(ctx)
		require.NoError(t, err)
		require.Equal(t, exp, lastB.Round, "found %d vs expected %d", lastB.Round, exp)
	}

	fn(resp.GetRound()-2, resp.GetRound()-2)
	// there is a race condition between the context being canceled and the nextf sync request being sent
	// let's sleep a short period to ensure we don't get a `syncing is already in progress` error
	time.Sleep(2 * time.Second)
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

	dt := NewDrandTestScenario(t, n, key.DefaultThreshold(n), p, beaconID, clockwork.NewFakeClockAt(time.Now()))

	group, err := dt.RunDKG(t)
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

	client := net.NewGrpcClientFromCertManager(dt.nodes[0].drand.log, dt.nodes[0].drand.opts.certmanager)
	ctx, cancel := context.WithCancel(ctx)

	// get last round first
	resp, err := client.PublicRand(ctx, rootID, new(drand.PublicRandRequest))
	require.NoError(t, err)
	current := resp.GetRound()

	t.Log(current)

	ctrlClient, err := net.NewControlClient(dt.nodes[0].drand.log, dt.nodes[0].drand.opts.controlPort)
	require.NoError(t, err)
	tls := true

	// First try with an invalid hash info
	t.Log("Trying to resync with an invalid address")

	_, errCh, _ := ctrlClient.StartCheckChain(context.Background(), "deadbeef", nil, tls, 10000, beaconID)
	expectChanFail(t, errCh)

	// Next trying with a fully valid chain
	cancel()
	ctx, cancel = context.WithCancel(context.Background())
	hash := fmt.Sprintf("%x", chain2.NewChainInfo(testlogger.New(t), group).Hash())
	addrToFollow := []string{rootID.Address()}
	upTo := uint64(5)

	t.Logf(" \t [-] Starting resync chain with a valid hash.\n")
	t.Logf(" \t\t --> beaconID: %s ; hash-chain: %s\n", beaconID, hash)
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
	err = store.Close()
	require.NoError(t, err)

	t.Logf(" \t\t --> Re-Starting node.\n")

	// Skip why: This call will create a new database connection.
	//  However, for the MemDB engine type, this means we create a new backing array from scratch
	//  thus removing all previous items from memory. At that point, this invalidates the test.
	dt.StartDrand(ctx, t, dt.nodes[0].addr, true, false)

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

	dt := NewDrandTestScenario(t, n, thr, p, beaconID, clockwork.NewFakeClockAt(time.Now()))

	group, err := dt.RunDKG(t)
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
	t.Logf("Getting round %d\n", 0)
	resp, err := client.Get(ctx, 0)
	require.NoError(t, err)

	//  run streaming and expect responses
	t.Log("Watching new rounds generated")
	rc := client.Watch(ctx)

	// expect first round now since node already has it
	dt.AdvanceMockClock(t, group.Period)
	beacon, ok := <-rc
	require.True(t, ok, "expected beacon")

	t.Logf("Round received %d\n", beacon.Round())
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

		t.Logf("Round received %d\n", beacon.Round())
		require.Equal(t, round, beacon.Round())
	}
}

func TestModifyingGroupFileManuallyDoesNotSegfault(t *testing.T) {
	ctx := context.Background()

	// set up 3 nodes for a test
	n := 3
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	beaconID := test.GetBeaconIDFromEnv()

	dt := NewDrandTestScenario(t, n, thr, p, beaconID, clockwork.NewFakeClockAt(time.Now()))

	node := dt.nodes[0]
	dir := dt.dir
	priv := node.drand.priv

	// set a persistent keystore, as the normal test ones are ephemeral
	store := key.NewFileStore(dir, beaconID)
	node.drand.store = store

	// save the key pair, as this was done ephemerally inside `NewDrandTestScenario` >.>
	err := store.SaveKeyPair(priv)
	require.NoError(t, err)

	// run a DKG so that every node gets a group file and key share
	_, err = dt.RunDKG(t)
	require.NoError(t, err)

	// stop the node and wait for it
	node.daemon.Stop(ctx)
	<-node.daemon.exitCh
	// although the exit channel has signaled exit, the control client is stopped out of band
	// without waiting the pessimistic closing time, we may try and restart the daemon below
	// before the port has been given up and cause an error binding the new port :(
	time.Sleep(5 * time.Second)

	// modify your entry (well, all of them!) in the group file to change the TLS status
	groupPath := path.Join(dir, beaconID, key.GroupFolderName, "drand_group.toml")

	// read
	groupFileReader, err := os.Open(groupPath)
	require.NoError(t, err)
	groupFile, err := io.ReadAll(groupFileReader)
	require.NoError(t, err)
	// write
	err = os.WriteFile(groupPath, []byte(strings.ReplaceAll(string(groupFile), "true", "false")), 0o740)
	require.NoError(t, err)

	err = node.daemon.init(ctx)
	require.NoError(t, err)
	// try and reload the beacon from the store
	// the updated TLS status will fail verification
	_, err = node.daemon.LoadBeaconFromStore(ctx, beaconID, store)

	require.EqualError(t, err, "could not restore beacon info for the given identity - this can happen if you updated the group file manually")
}

func TestDKGWithMismatchedSchemes(t *testing.T) {
	t.Setenv("DRAND_TEST_LOGS", "DEBUG")
	beaconID := "blah"
	scenario := NewDrandTestScenario(t, 2, 2, 1*time.Second, beaconID, clockwork.NewFakeClockAt(time.Now()))

	// to dedupe it when we're running the tests with different default schemes
	if os.Getenv("SCHEME_ID") == crypto.ShortSigSchemeID {
		scenario.scheme = crypto.NewPedersenBLSChained()
	} else {
		scenario.scheme = crypto.NewPedersenBLSUnchainedSwapped()
	}

	t.Setenv("SCHEME_ID", scenario.scheme.Name)
	scenario.AddNodesWithOptions(t, 1, beaconID)
	t.Setenv("SCHEME_ID", "")

	_, err := scenario.RunDKG(t)
	require.ErrorContainsf(t, err, key.ErrInvalidKeyScheme.Error(), "expected node to fail DKG due to mismatch of schemes")
}

func TestPacketWithoutMetadata(t *testing.T) {
	t.Setenv("DRAND_TEST_LOGS", "DEBUG")
	beaconID := "blah"
	scenario := NewDrandTestScenario(t, 2, 2, 1*time.Second, beaconID, clockwork.NewFakeClockAt(time.Now()))

	_, err := scenario.RunDKG(t)
	require.NoError(t, err)

	_, err = scenario.nodes[0].daemon.Packet(context.Background(), &drand.GossipPacket{Packet: &drand.GossipPacket_Proposal{
		Proposal: &drand.ProposalTerms{
			BeaconID:             beaconID,
			Epoch:                2,
			Leader:               nil,
			Threshold:            uint32(scenario.thr),
			Timeout:              timestamppb.New(time.Now().Add(1 * time.Minute)),
			CatchupPeriodSeconds: 6,
			TransitionTime:       timestamppb.New(time.Now().Add(10 * time.Second)),
		}}, Metadata: nil},
	)

	// should error but not panic
	require.Error(t, err)
}

func TestDKGPacketWithoutMetadata(t *testing.T) {
	t.Setenv("DRAND_TEST_LOGS", "DEBUG")
	beaconID := "blah"
	scenario := NewDrandTestScenario(t, 2, 2, 1*time.Second, beaconID, clockwork.NewFakeClockAt(time.Now()))

	_, err := scenario.RunDKG(t)
	require.NoError(t, err)

	_, err = scenario.nodes[0].daemon.BroadcastDKG(context.Background(), &drand.DKGPacket{
		Dkg: &dkg3.Packet{
			Bundle: &dkg3.Packet_Deal{
				Deal: &dkg3.DealBundle{
					DealerIndex: 1,
					Commits:     nil,
					Deals:       nil,
					SessionId:   nil,
					Signature:   nil,
				}},
			Metadata: nil,
		},
	})

	// should error but not panic
	require.Error(t, err)
}

func TestDKGPacketWithNilInArray(t *testing.T) {
	t.Setenv("DRAND_TEST_LOGS", "DEBUG")
	beaconID := "blah"
	scenario := NewDrandTestScenario(t, 2, 2, 1*time.Second, beaconID, clockwork.NewFakeClockAt(time.Now()))

	// the first slot will be nil
	joiners := make([]*drand.Participant, len(scenario.nodes)+1)
	for i, node := range scenario.nodes {
		identity := node.drand.priv.Public
		pk, err := identity.Key.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		// + 1 here, so the first entry is nil
		joiners[i+1] = &drand.Participant{
			Address:   identity.Addr,
			Tls:       identity.TLS,
			PubKey:    pk,
			Signature: identity.Signature,
		}
	}
	err := scenario.nodes[0].dkgRunner.StartNetwork(2, 1, crypto.DefaultSchemeID, 1*time.Minute, 1, joiners)

	require.NoError(t, err)
}

func TestFailedReshareContinuesUsingOldGroupfile(t *testing.T) {
	t.Setenv("DRAND_TEST_LOGS", "DEBUG")
	period := 1 * time.Second
	beaconID := "blah"
	scenario := NewDrandTestScenario(t, 2, 2, period, beaconID, clockwork.NewFakeClockAt(time.Now()))

	g, err := scenario.RunDKG(t)
	require.NoError(t, err)

	scenario.SetMockClock(t, g.GenesisTime)

	leader := scenario.nodes[0]
	err = scenario.RunFailingReshare()
	require.Equal(t, test.ErrDKGFailed, err)
	require.Equal(t, leader.drand.group, g)
}

// AddNodesWithOptions creates new additional nodes that can participate during the initial DKG.
// The options set will overwrite the existing ones.
func (d *DrandTestScenario) AddNodesWithOptions(t *testing.T, n int, beaconID string, opts ...ConfigOption) []*MockNode {
	t.Logf("Setup of %d new nodes for tests", n)
	beaconID = common.GetCanonicalBeaconID(beaconID)

	d.n += n

	opts = append(opts, WithCallOption(grpc.WaitForReady(true)))
	daemons, drands, _, _, newCertPaths := BatchNewDrand(t, 0, n, false, d.scheme, beaconID, opts...)
	//nolint:prealloc // We don't preallocate this as it's not going to be big enough to warrant such an operation
	var result []*MockNode
	for i, drandInstance := range drands {
		node, err := newNode(d.clock.Now(), newCertPaths[i], daemons[i], drandInstance)
		require.NoError(t, err)
		d.nodes = append(d.nodes, node)
		result = append(result, node)
	}

	oldCertPaths := make([]string, len(d.nodes))

	// add certificates of new nodes to the old nodes and populate old cert list
	for i, node := range d.nodes {
		oldCertPaths[i] = node.certPath
		inst := node.drand
		for _, cp := range newCertPaths {
			err := inst.opts.certmanager.Add(cp)
			require.NoError(t, err)
		}
	}

	// store new part. and add certificate path of old nodes to the new ones
	d.newNodes = make([]*MockNode, n)
	for i, inst := range drands {
		node, err := newNode(d.clock.Now(), newCertPaths[i], daemons[i], inst)
		require.NoError(t, err)
		d.newNodes[i] = node
		for _, cp := range oldCertPaths {
			err := inst.opts.certmanager.Add(cp)
			require.NoError(t, err)
		}
	}

	return result
}
