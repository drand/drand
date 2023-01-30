package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/log"
	"github.com/drand/drand/test"
)

func TestBeaconProcess_Stop(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	privs, _ := test.BatchIdentities(1, sch, t.Name())

	port := test.FreePort()

	confOptions := []ConfigOption{
		WithConfigFolder(t.TempDir()),
		WithPrivateListenAddress("127.0.0.1:0"),
		WithControlPort(port),
		WithInsecure(),
		WithLogLevel(log.LogDebug, false),
	}

	confOptions = append(confOptions, WithTestDB(t, test.ComputeDBName())...)

	dd, err := NewDrandDaemon(NewConfig(confOptions...))
	require.NoError(t, err)

	store := test.NewKeyStore()
	require.NoError(t, store.SaveKeyPair(privs[0]))
	proc, err := dd.InstantiateBeaconProcess(t.Name(), store)
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
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	privs, _ := test.BatchIdentities(1, sch, t.Name())

	port := test.FreePort()

	confOptions := []ConfigOption{
		WithConfigFolder(t.TempDir()),
		WithPrivateListenAddress("127.0.0.1:0"),
		WithControlPort(port),
		WithInsecure(),
		WithLogLevel(log.LogDebug, false),
	}

	confOptions = append(confOptions, WithTestDB(t, test.ComputeDBName())...)

	dd, err := NewDrandDaemon(NewConfig(confOptions...))
	require.NoError(t, err)

	store := test.NewKeyStore()
	require.NoError(t, store.SaveKeyPair(privs[0]))
	proc, err := dd.InstantiateBeaconProcess(t.Name(), store)
	require.NoError(t, err)
	require.NotNil(t, proc)

	proc2, err := dd.InstantiateBeaconProcess(t.Name()+"second", store)
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
	const existingNodesCount = 3
	const thr = 4
	const period = 1 * time.Second
	beaconName := t.Name()

	ts := NewDrandTestScenario(t, existingNodesCount, thr, period, beaconName)

	// We want to explicitly run a node with the chain.MemDB backend
	newNodes := ts.AddNodesWithOptions(t, 1, beaconName, WithDBStorageEngine(chain.MemDB))
	group, err := ts.RunDKG()
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
	const existingNodesCount = 3
	const newNodesCount = 1
	const thr = 3
	const period = 1 * time.Second
	beaconName := "default"

	ts := NewDrandTestScenario(t, existingNodesCount, thr, period, beaconName)
	group, err := ts.RunDKG()
	require.NoError(t, err)

	ts.SetMockClock(t, group.GenesisTime)

	err = ts.WaitUntilChainIsServing(t, ts.nodes[0])
	require.NoError(t, err)

	ts.AdvanceMockClock(t, period)

	t.Log("SetupNewNodes")

	// We want to explicitly run a node with the chain.MemDB backend
	newNodes := ts.SetupNewNodes(t, newNodesCount, WithDBStorageEngine(chain.MemDB))
	memDBNode := newNodes[0]

	t.Log("running reshare")
	newGroup, err := ts.RunReshare(t, &reshareConfig{
		oldRun:  existingNodesCount,
		newRun:  newNodesCount,
		newThr:  thr + newNodesCount,
		timeout: time.Second,
	})
	require.NoError(t, err)
	require.NotNil(t, newGroup)

	ts.SetMockClock(t, newGroup.TransitionTime)
	time.Sleep(getSleepDuration())

	ts.AdvanceMockClock(t, newGroup.Period)
	time.Sleep(getSleepDuration())

	t.Log("running WaitUntilChainIsServing")
	err = ts.WaitUntilChainIsServing(t, memDBNode)
	require.NoError(t, err)
}
