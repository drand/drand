package core

import (
	"context"
	"os"
	"path"
	"testing"
	"time"

	"github.com/drand/drand/common/key"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share"
	kyberDKG "github.com/drand/kyber/share/dkg"

	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/common"
	"github.com/drand/drand/common/testlogger"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/internal/chain"
	"github.com/drand/drand/internal/dkg"
	"github.com/drand/drand/internal/test"
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
	nodeShare := node.drand.share
	err = node.daemon.dkg.Migrate(ts.beaconID, group, nodeShare)
	require.NoError(t, err)
	status2, err := node.daemon.DKGStatus(context.Background(), &drand.DKGStatusRequest{BeaconID: ts.beaconID})
	require.NoError(t, err)
	require.NotNil(t, status2.Complete)
}

func TestMigrateOldGroupFileWithLeavers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping slow test in short mode.")
	}

	// first we create a bunch of nodes
	const nodeCount = 3
	const thr = 2
	const period = 1 * time.Second

	// set up a few nodes and run a DKG
	beaconID := t.Name()
	dt := NewDrandTestScenario(t, nodeCount, thr, period, beaconID, clockwork.NewFakeClockAt(time.Now()))

	// then we turn them into an existing group file, as if we had completed a DKG using v1
	sch := dt.scheme
	k1 := dt.nodes[0].drand.priv
	k2 := dt.nodes[1].drand.priv
	k3 := dt.nodes[2].drand.priv
	group := key.Group{
		Threshold:     2,
		Period:        1,
		Scheme:        sch,
		ID:            "banana",
		CatchupPeriod: 1,
		Nodes: []*key.Node{
			{
				Identity: &key.Identity{
					Key:       k1.Public.Key,
					Addr:      k1.Public.Addr,
					Signature: []byte("deadbeef"),
					Scheme:    sch,
				},
				Index: 0,
			}, {
				Identity: &key.Identity{
					Key:       k2.Public.Key,
					Addr:      k2.Public.Addr,
					Signature: []byte("deadbeef"),
					Scheme:    sch,
				},
				Index: 1,
			},
			{
				Identity: &key.Identity{
					Key:       k3.Public.Key,
					Addr:      k3.Public.Addr,
					Signature: []byte("deadbeef"),
					Scheme:    sch,
				},
				Index: 2,
			},
		},
		GenesisTime:    0,
		GenesisSeed:    []byte("deadbeef"),
		TransitionTime: 1,
		PublicKey:      &key.DistPublic{Coefficients: []kyber.Point{sch.KeyGroup.Point()}},
	}

	// we run the migrate command manually, as the `MockNode` doesn't use `LoadBeaconFromStore`,
	// but `InstantiateBeaconProcess`
	for i, node := range dt.nodes {
		node.daemon.dkg.Migrate(beaconID, &group, &key.Share{
			DistKeyShare: kyberDKG.DistKeyShare{
				Commits: nil,
				Share: &share.PriShare{
					I: i,
					V: sch.KeyGroup.Scalar(),
				},
			},
			Scheme: sch,
		})
	}

	// then we run a reshare which should succeed
	_, err := dt.RunReshare(t, time.Now().Add(10*time.Second), dt.nodes, []*MockNode{})
	require.NoError(t, err)
}
