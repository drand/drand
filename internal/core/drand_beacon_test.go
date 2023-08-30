package core

import (
	"context"
	"os"
	"path"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/common"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/internal/chain"
	"github.com/drand/drand/internal/dkg"
	"github.com/drand/drand/internal/test"
	"github.com/drand/drand/internal/test/testlogger"
	"github.com/drand/drand/protobuf/drand"
)

func TestBeaconProcess_Stop(t *testing.T) {
	l := testlogger.New(t)
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	privs, _ := test.BatchIdentities(1, sch, t.Name())

	port := test.FreePort()

	confOptions := []ConfigOption{
		WithConfigFolder(t.TempDir()),
		WithPrivateListenAddress("127.0.0.1:0"),
		WithControlPort(port),
	}

	confOptions = append(confOptions, WithTestDB(t, test.ComputeDBName())...)

	dd, err := NewDrandDaemon(ctx, NewConfig(l, confOptions...))
	require.NoError(t, err)

	store := test.NewKeyStore()
	require.NoError(t, store.SaveKeyPair(privs[0]))
	proc, err := dd.InstantiateBeaconProcess(ctx, t.Name(), store)
	require.NoError(t, err)
	require.NotNil(t, proc)

	time.Sleep(250 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	proc.Stop(ctx)
	closed, ok := <-proc.WaitExit()
	require.True(t, ok, "Expecting to receive from exit channel")
	require.True(t, closed, "Expecting to receive from exit channel")

	_, ok = <-proc.WaitExit()
	require.False(t, ok, "Expecting exit channel to be closed")
}

func TestBeaconProcess_Stop_MultiBeaconOneBeaconAlreadyStopped(t *testing.T) {
	l := testlogger.New(t)
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	privs, _ := test.BatchIdentities(1, sch, t.Name())

	port := test.FreePort()

	confOptions := []ConfigOption{
		WithConfigFolder(t.TempDir()),
		WithPrivateListenAddress("127.0.0.1:0"),
		WithControlPort(port),
	}

	confOptions = append(confOptions, WithTestDB(t, test.ComputeDBName())...)

	dd, err := NewDrandDaemon(ctx, NewConfig(l, confOptions...))
	require.NoError(t, err)

	store := test.NewKeyStore()
	require.NoError(t, store.SaveKeyPair(privs[0]))
	proc, err := dd.InstantiateBeaconProcess(ctx, t.Name(), store)
	require.NoError(t, err)
	require.NotNil(t, proc)

	proc2, err := dd.InstantiateBeaconProcess(ctx, t.Name()+"second", store)
	require.NoError(t, err)
	require.NotNil(t, proc2)

	time.Sleep(250 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	proc.Stop(ctx)
	closed, ok := <-proc.WaitExit()
	require.True(t, ok, "Expecting to receive from exit channel")
	require.True(t, closed, "Expecting to receive from exit channel")

	_, ok = <-proc.WaitExit()
	require.False(t, ok, "Expecting exit channel to be closed")

	time.Sleep(250 * time.Millisecond)

	dd.Stop(ctx)
	closed, ok = <-dd.WaitExit()
	require.True(t, ok)
	require.True(t, closed)

	_, ok = <-dd.WaitExit()
	require.False(t, ok, "Expecting exit channel to be closed")
}

func TestMemDBBeaconJoinsNetworkAtStart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode.")
	}

	const existingNodesCount = 3
	const thr = 4
	const period = 1 * time.Second
	beaconName := t.Name()

	ts := NewDrandTestScenario(t, existingNodesCount, thr, period, beaconName, clockwork.NewFakeClockAt(time.Now()))

	// We want to explicitly run a node with the chain.MemDB backend
	newNodes := ts.AddNodesWithOptions(t, 1, beaconName, WithDBStorageEngine(chain.MemDB))
	group, err := ts.RunDKG(t)
	require.NoError(t, err)

	ts.SetMockClock(t, group.GenesisTime)

	memDBNode := newNodes[0]
	err = ts.WaitUntilChainIsServing(t, memDBNode)
	require.NoError(t, err)

	ts.AdvanceMockClock(t, period)

	err = ts.WaitUntilRound(t, memDBNode, 2)
	require.NoError(t, err)
}

func TestMemDBBeaconJoinsNetworkAfterDKG(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	if os.Getenv("CI") == "true" {
		t.Skip("test is flacky in CI")
	}

	const existingNodesCount = 3
	const newNodesCount = 1
	const thr = 3
	const period = 1 * time.Second
	beaconName := "default"
	sleepDuration := 100 * time.Millisecond

	ts := NewDrandTestScenario(t, existingNodesCount, thr, period, beaconName, clockwork.NewFakeClockAt(time.Now()))
	group, err := ts.RunDKG(t)
	require.NoError(t, err)
	ts.AdvanceMockClock(t, ts.nodes[0].daemon.opts.dkgKickoffGracePeriod)
	time.Sleep(sleepDuration)

	ts.SetMockClock(t, group.GenesisTime)
	time.Sleep(sleepDuration)

	err = ts.WaitUntilChainIsServing(t, ts.nodes[0])
	require.NoError(t, err)

	ts.AdvanceMockClock(t, period)
	time.Sleep(sleepDuration)

	err = ts.WaitUntilRound(t, ts.nodes[0], 2)
	require.NoError(t, err)

	t.Log("SetupNewNodes")

	// We want to explicitly run a node with the chain.MemDB backend
	newNodes := ts.SetupNewNodes(t, newNodesCount, WithDBStorageEngine(chain.MemDB))
	memDBNode := newNodes[0]

	t.Log("running reshare")
	newGroup, err := ts.RunReshare(t, ts.clock.Now().Add(3*period), ts.nodes, newNodes)
	require.NoError(t, err)
	require.NotNil(t, newGroup)
	t.Logf("expect transition at time %d", newGroup.TransitionTime)

	for {
		ts.AdvanceMockClock(t, period)
		time.Sleep(sleepDuration)
		if ts.clock.Now().Unix() > newGroup.TransitionTime {
			break
		}
	}

	ts.AdvanceMockClock(t, period)
	time.Sleep(sleepDuration)

	t.Log("running WaitUntilChainIsServing")
	err = ts.WaitUntilChainIsServing(t, memDBNode)
	require.NoError(t, err)

	ts.AdvanceMockClock(t, period)
	time.Sleep(sleepDuration)

	ts.AdvanceMockClock(t, period)
	time.Sleep(sleepDuration)

	expectedRound := common.CurrentRound(ts.clock.Now().Unix(), newGroup.Period, newGroup.GenesisTime)
	err = ts.WaitUntilRound(t, memDBNode, expectedRound-1)
	require.NoError(t, err)
}

func TestMigrateMissingDKGDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping slow test in short mode.")
	}
	const nodeCount = 3
	const thr = 2
	const period = 1 * time.Second
	sleepDuration := 100 * time.Millisecond

	// set up a few nodes and run a DKG
	beaconName := t.Name()
	ts := NewDrandTestScenario(t, nodeCount, thr, period, beaconName, clockwork.NewFakeClockAt(time.Now()))

	group, err := ts.RunDKG(t)
	require.NoError(t, err)

	ts.SetMockClock(t, group.GenesisTime)
	ts.AdvanceMockClock(t, period)
	time.Sleep(sleepDuration)

	err = ts.WaitUntilRound(t, ts.nodes[0], 2)
	require.NoError(t, err)

	// nuke the DKG state for a node and reload
	// the DKG process to clear any open handles
	node := ts.nodes[0]
	err = os.Remove(path.Join(node.daemon.opts.configFolder, dkg.BoltFileName))
	require.NoError(t, err)
	dkgStore, err := dkg.NewDKGStore(node.daemon.opts.configFolder, node.daemon.opts.boltOpts)
	require.NoError(t, err)
	node.daemon.dkg = dkg.NewDKGProcess(
		dkgStore, node.daemon, node.daemon.completedDKGs, node.daemon.privGateway, dkg.Config{}, node.daemon.log,
	)
	require.NoError(t, err)

	// there should be no completed DKGs now for that node
	status, err := node.daemon.DKGStatus(context.Background(), &drand.DKGStatusRequest{BeaconID: ts.beaconID})
	require.NoError(t, err)
	require.Nil(t, status.Complete)

	// run the migration and check that there now is a completed DKG
	share := node.drand.share
	err = node.daemon.dkg.Migrate(ts.beaconID, group, share)
	require.NoError(t, err)
	status2, err := node.daemon.DKGStatus(context.Background(), &drand.DKGStatusRequest{BeaconID: ts.beaconID})
	require.NoError(t, err)
	require.NotNil(t, status2.Complete)
}
