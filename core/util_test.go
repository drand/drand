package core

import (
	"context"
	"errors"
	"fmt"
	gnet "net"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	clock "github.com/jonboulle/clockwork"
	"github.com/kabukky/httpscerts"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/common"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"
)

//nolint:gocritic
type DrandTestScenario struct {
	sync.Mutex

	// note: do we need this here?
	t *testing.T

	// tmp dir for certificates, keys etc
	dir    string
	newDir string

	// global clock on which all drand clocks are synchronized
	clock clock.FakeClock

	n             int
	thr           int
	period        time.Duration
	catchupPeriod time.Duration
	scheme        *crypto.Scheme
	beaconID      string

	// only set after the DKG
	group *key.Group
	// needed to give the group to new nodes during a resharing - only set after
	// a successful DKG
	groupPath string
	// nodes that are created for running a first DKG
	nodes []*MockNode
	// new additional nodes that are created for running a resharing
	newNodes []*MockNode
	// nodes that actually ran the resharing phase - it's a combination of nodes
	// and new nodes. These are the one that should appear in the newGroup
	resharedNodes []*MockNode
}

// BatchNewDrand returns n drands, using TLS or not, with the given
// options. It returns the list of Drand structures, the group created,
// the folder where db, certificates, etc are stored. It is the folder
// to delete at the end of the test. As well, it returns a public grpc
// client that can reach any drand node.
// Deprecated: do not use
//
//nolint:funlen
func BatchNewDrand(
	t *testing.T,
	currentNodeCount,
	n int,
	insecure bool,
	sch *crypto.Scheme,
	beaconID string,
	opts ...ConfigOption,
) (
	daemons []*DrandDaemon, drands []*BeaconProcess, group *key.Group, dir string, certPaths []string,
) {
	t.Logf("Creating %d nodes for beaconID %s", n, beaconID)
	var privs []*key.Pair
	if insecure {
		privs, group = test.BatchIdentities(n, sch, beaconID)
	} else {
		privs, group = test.BatchTLSIdentities(n, sch, beaconID)
	}

	ports := test.Ports(n)
	daemons = make([]*DrandDaemon, n)
	drands = make([]*BeaconProcess, n)

	// notice t.TempDir means the temp directory is deleted thanks to t.Cleanup at the end
	dir = t.TempDir()

	certPaths = make([]string, n)
	keyPaths := make([]string, n)
	dirs := make([]string, n)

	for i := 0; i < n; i++ {
		testNodeIndex := currentNodeCount + i
		dirs[i] = path.Join(dir, fmt.Sprintf("drand-%d", testNodeIndex))
		if err := os.MkdirAll(dirs[i], 0o777); err != nil {
			panic(err)
		}
	}

	if !insecure {
		for i := 0; i < n; i++ {
			testNodeIndex := currentNodeCount + i
			certPath := path.Join(dirs[i], fmt.Sprintf("server-%d.crt", testNodeIndex))
			keyPath := path.Join(dirs[i], fmt.Sprintf("server-%d.key", testNodeIndex))

			if httpscerts.Check(certPath, keyPath) != nil {
				h, _, err := gnet.SplitHostPort(privs[i].Public.Address())
				require.NoError(t, err)

				t.Logf("generate keys for drand %d", testNodeIndex)
				err = httpscerts.Generate(certPath, keyPath, h)
				require.NoError(t, err)
			}
			certPaths[i] = certPath
			keyPaths[i] = keyPath
		}
	}

	for i := 0; i < n; i++ {
		s := test.NewKeyStore()

		require.NoError(t, s.SaveKeyPair(privs[i]))

		// give each one their own private folder
		confOptions := []ConfigOption{
			WithConfigFolder(dirs[i]),
		}

		confOptions = append(confOptions, WithTestDB(t, test.ComputeDBName())...)
		confOptions = append(append(confOptions, WithDkgKickoffGracePeriod(3*time.Second)), WithPrivateListenAddress(privs[i].Public.Address()))
		if !insecure {
			confOptions = append(confOptions,
				WithTLS(certPaths[i], keyPaths[i]),
				WithTrustedCerts(certPaths...))
		} else {
			confOptions = append(confOptions, WithInsecure())
		}

		confOptions = append(confOptions,
			WithControlPort(ports[i]),
			WithLogLevel(test.LogLevel(t), false),
			WithNamedLogger(fmt.Sprintf("[node %d]", currentNodeCount+i)),
			WithMemDBSize(100),
		)
		// add options in last so it overwrites the default
		confOptions = append(confOptions, opts...)

		t.Logf("Creating node %d", i)

		daemon, err := NewDrandDaemon(NewConfig(confOptions...))
		require.NoError(t, err)

		bp, err := daemon.InstantiateBeaconProcess(beaconID, s)
		require.NoError(t, err)

		daemons[i] = daemon
		drands[i] = bp

		// Make sure to stop all daemon after each test
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			daemon.Stop(ctx)
		})
	}

	return daemons, drands, group, dir, certPaths
}

// Deprecated: do not use sleeps in your tests
func getSleepDuration() time.Duration {
	if os.Getenv("CI") != "" {
		fmt.Println("--- Sleeping on CI")
		return time.Duration(800) * time.Millisecond
	}
	return time.Duration(500) * time.Millisecond
}

// NewDrandTest creates a drand test scenario with initial n nodes and ready to
// run a DKG for the given threshold that will then launch the beacon with the
// specified period
func NewDrandTestScenario(t *testing.T, n, thr int, period time.Duration, beaconID string) *DrandTestScenario {
	sch, err := crypto.GetSchemeFromEnv()
	if err != nil {
		panic(err)
	}
	dt := new(DrandTestScenario)
	beaconID = common.GetCanonicalBeaconID(beaconID)

	// hmm it seems like this _has_ to be insecure as the `ControlClient` uses insecure credentials?
	// dunno how any tests were passing if this was the case though
	daemons, drands, _, dir, certPaths := BatchNewDrand(
		t, 0, n, false, sch, beaconID, WithCallOption(grpc.WaitForReady(true)),
	)

	dt.t = t
	dt.dir = dir
	dt.groupPath = path.Join(dt.dir, "group.toml")
	dt.n = n
	dt.scheme = sch
	dt.beaconID = beaconID
	dt.thr = thr
	dt.period = period
	dt.clock = clock.NewFakeClock()
	dt.nodes = make([]*MockNode, 0, n)

	for i, drandInstance := range drands {
		node, err := newNode(dt.clock.Now(), certPaths[i], daemons[i], drandInstance)
		if err != nil {
			panic("couldn't construct mock node")
		}
		dt.nodes = append(dt.nodes, node)
	}

	return dt
}

// Ids returns the list of the first n ids given the newGroup parameter (either
// in the original group or the reshared one)
// Deprecated: Rename this to addresses to align naming
func (d *DrandTestScenario) Ids(n int, newGroup bool) []string {
	nodes := d.nodes
	if newGroup {
		nodes = d.resharedNodes
	}

	addresses := make([]string, 0, n)
	for _, node := range nodes[:n] {
		addresses = append(addresses, node.addr)
	}

	return addresses
}

// GetBeacon returns the beacon of the given round for the specified drand id
func (d *DrandTestScenario) GetBeacon(id string, round int, newGroup bool) (*chain.Beacon, error) {
	nodes := d.nodes
	if newGroup {
		nodes = d.resharedNodes
	}
	for _, node := range nodes {
		if node.addr != id {
			continue
		}
		return node.drand.beacon.Store().Get(context.Background(), uint64(round))
	}
	return nil, errors.New("that should not happen")
}

// GetMockNode returns the node associated with this address, either in the new
// group or the current group.
func (d *DrandTestScenario) GetMockNode(nodeAddress string, newGroup bool) *MockNode {
	nodes := d.nodes
	if newGroup {
		nodes = d.resharedNodes
	}

	for _, node := range nodes {
		if node.addr == nodeAddress {
			return node
		}
	}

	require.FailNow(d.t, "no nodes found at this nodeAddress: "+nodeAddress)
	return nil
}

// StopMockNode stops a node from the first group
func (d *DrandTestScenario) StopMockNode(nodeAddr string, newGroup bool) {
	node := d.GetMockNode(nodeAddr, newGroup)

	dr := node.drand
	dr.Stop(context.Background())
	<-dr.WaitExit()

	d.t.Logf("[drand] stop %s", dr.priv.Public.Address())

	controlClient, err := net.NewControlClient(dr.opts.controlPort)
	require.NoError(d.t, err)

	retryCount := 1
	maxRetries := 5
	for range time.Tick(100 * time.Millisecond) {
		d.t.Logf("[drand] ping %s: %d/%d", dr.priv.Public.Address(), retryCount, maxRetries)
		response, err := controlClient.Status(d.beaconID)
		if err != nil {
			break
		}
		if response.Beacon.Status == uint32(BeaconNotInited) {
			break
		}

		retryCount++
		require.LessOrEqual(d.t, retryCount, maxRetries)
	}

	d.t.Logf("[drand] stopped %s", dr.priv.Public.Address())
}

// StartDrand fetches the drand given the id, in the respective group given the
// newGroup parameter and runs the beacon
func (d *DrandTestScenario) StartDrand(t *testing.T, nodeAddress string, catchup, newGroup bool) {
	node := d.GetMockNode(nodeAddress, newGroup)
	dr := node.drand

	d.t.Logf("[drand] Start")
	err := dr.StartBeacon(catchup)
	if err != nil {
		d.t.Logf("[drand] Start had an error: %v\n", err)
	}
	d.t.Logf("[drand] Started")
}

func (d *DrandTestScenario) Now() time.Time {
	return d.clock.Now()
}

// SetMockClock sets the clock of all drands to the designated unix timestamp in
// seconds
func (d *DrandTestScenario) SetMockClock(t *testing.T, targetUnixTime int64) {
	if now := d.Now().Unix(); now < targetUnixTime {
		d.AdvanceMockClock(t, time.Duration(targetUnixTime-now)*time.Second)
	} else {
		d.t.Logf("ALREADY PASSED")
	}

	t.Logf("Set time to genesis time: %d", d.Now().Unix())
}

// AdvanceMockClock advances the clock of all drand by the given duration
func (d *DrandTestScenario) AdvanceMockClock(t *testing.T, p time.Duration) {
	t.Log("Advancing time by", p, "from", d.clock.Now().Unix())
	for _, node := range d.nodes {
		node.clock.Advance(p)
	}
	for _, node := range d.newNodes {
		node.clock.Advance(p)
	}
	d.clock.Advance(p)
	// we sleep to make sure everyone has the time to get the new time before continuing
	time.Sleep(10 * time.Millisecond)
}

// CheckBeaconLength looks if the beacon chain on the given addresses is of the
// expected length (actual round plus 1, as beacons go from 0 to n)
func (d *DrandTestScenario) CheckBeaconLength(t *testing.T, nodes []*MockNode, expectedLength int) {
	for _, node := range nodes {
		err := d.WaitUntilRound(t, node, uint64(expectedLength-1))
		require.NoError(t, err)
	}
}

// CheckPublicBeacon looks if we can get the latest beacon on this node
func (d *DrandTestScenario) CheckPublicBeacon(nodeAddress string, newGroup bool) *drand.PublicRandResponse {
	node := d.GetMockNode(nodeAddress, newGroup)
	dr := node.drand

	client := net.NewGrpcClientFromCertManager(dr.opts.certmanager, dr.opts.grpcOpts...)
	resp, err := client.PublicRand(context.TODO(), test.NewTLSPeer(dr.priv.Public.Addr), &drand.PublicRandRequest{})

	require.NoError(d.t, err)
	require.NotNil(d.t, resp)
	return resp
}

// SetupNewNodes creates new additional nodes that can participate during the resharing
func (d *DrandTestScenario) SetupNewNodes(t *testing.T, countOfAdditionalNodes int, options ...ConfigOption) []*MockNode {
	t.Log("Setup of", countOfAdditionalNodes, "new nodes for tests")
	currentNodeCount := len(d.nodes)

	newDaemons, newDrands, _, newDir, newCertPaths := BatchNewDrand(
		d.t,
		currentNodeCount,
		countOfAdditionalNodes,
		false,
		d.scheme,
		d.beaconID,
		append(options, WithCallOption(grpc.WaitForReady(false)))...,
	)
	d.newDir = newDir

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
	d.newNodes = make([]*MockNode, countOfAdditionalNodes)
	for i, inst := range newDrands {
		node, err := newNode(d.clock.Now(), newCertPaths[i], newDaemons[i], inst)
		if err != nil {
			fmt.Println("could not construct mock node")
			t.Fail()
		}
		d.newNodes[i] = node
		node.daemon.opts.logger.Named(fmt.Sprintf("node %d", len(d.nodes)+1))
		for _, cp := range oldCertPaths {
			err := inst.opts.certmanager.Add(cp)
			require.NoError(t, err)
		}
	}

	return d.newNodes
}

func (d *DrandTestScenario) RunDKG() (*key.Group, error) {
	if len(d.nodes) == 0 {
		return nil, errors.New("cannot run a DKG with 0 nodes in the drand test scenario")
	}

	joiners := make([]*drand.Participant, len(d.nodes))
	for i, node := range d.nodes {
		identity := node.drand.priv.Public
		pk, err := identity.Key.MarshalBinary()
		if err != nil {
			return nil, err
		}
		joiners[i] = &drand.Participant{
			Address:   identity.Addr,
			Tls:       identity.TLS,
			PubKey:    pk,
			Signature: identity.Signature,
		}
	}

	leader := d.nodes[0]
	followers := d.nodes[1:]

	err := leader.dkgRunner.StartNetwork(d.thr, int(d.period.Seconds()), d.scheme.Name, int(d.catchupPeriod.Seconds()), joiners)

	if err != nil {
		return nil, err
	}

	for _, follower := range followers {
		err = follower.dkgRunner.JoinDKG()
		if err != nil {
			return nil, err
		}
	}

	err = leader.dkgRunner.StartExecution()
	if err != nil {
		return nil, err
	}

	// advance by the grace period so all nodes kick off the DKG
	d.AdvanceMockClock(d.t, d.nodes[0].daemon.opts.dkgKickoffGracePeriod)

	groupFile, err := d.WaitForDKG(leader, 1, 100)
	if err != nil {
		return nil, err
	}
	d.group = groupFile
	return groupFile, nil
}

// WaitForDKG waits for the DKG complete and returns the group file
// it takes the gorup file from the leader node and thus assumes the leader has not been evicted!
func (d *DrandTestScenario) WaitForDKG(node *MockNode, epoch uint32, numberOfSeconds int) (*key.Group, error) {
	err := node.dkgRunner.WaitForDKG(d.beaconID, epoch, numberOfSeconds)
	if err != nil {
		return nil, err
	}

	group := d.nodes[0].daemon.beaconProcesses[d.beaconID].group
	if group == nil {
		panic("group file was nil despite completion!")
	}
	return group, nil
}

type lifecycleHooks struct {
	postAcceptance     func()
	postExecutionStart func()
}

func (d *DrandTestScenario) RunReshare(
	remainingNodes []*MockNode,
	joiningNodes []*MockNode,
) (*key.Group, error) {
	return d.RunReshareWithHooks(remainingNodes, joiningNodes, lifecycleHooks{})
}

//nolint:funlen
func (d *DrandTestScenario) RunReshareWithHooks(
	remainingNodes []*MockNode,
	joiningNodes []*MockNode,
	hooks lifecycleHooks,
) (*key.Group, error) {
	if len(remainingNodes) == 0 {
		return nil, errors.New("cannot run a DKG with 0 nodes in the drand test scenario")
	}

	// our first node will be the leader
	leader := remainingNodes[0]

	// map all the remainers to participants
	remainers := make([]*drand.Participant, len(remainingNodes))
	for i, node := range remainingNodes {
		identity := node.drand.priv.Public
		pk, err := identity.Key.MarshalBinary()
		if err != nil {
			return nil, err
		}
		remainers[i] = &drand.Participant{
			Address:   identity.Addr,
			Tls:       identity.TLS,
			PubKey:    pk,
			Signature: identity.Signature,
		}
	}

	// map all the joiners to participants
	joiners := make([]*drand.Participant, len(joiningNodes))
	for i, node := range joiningNodes {
		identity := node.drand.priv.Public
		pk, err := identity.Key.MarshalBinary()
		if err != nil {
			return nil, err
		}

		joiners[i] = &drand.Participant{
			Address:   identity.Addr,
			Tls:       identity.TLS,
			PubKey:    pk,
			Signature: identity.Signature,
		}
	}

	// set the transition time to round 3
	transitionTime := time.Unix(d.group.GenesisTime+(3*int64(d.period.Seconds())), 0)
	err := leader.dkgRunner.StartProposal(
		d.thr,
		transitionTime,
		int(d.catchupPeriod.Seconds()),
		joiners,
		remainers,
		[]*drand.Participant{},
	)
	if err != nil {
		return nil, err
	}

	// all the remainers except the leader accept
	for _, remainer := range remainingNodes[1:] {
		err := remainer.dkgRunner.Accept()
		if err != nil {
			return nil, err
		}
	}

	// if there are any hooks now (such as errors to trigger) we trigger them
	if hooks.postAcceptance != nil {
		hooks.postAcceptance()
	}

	// all the joiners join
	for _, joiner := range joiningNodes {
		err = joiner.dkgRunner.JoinReshare(d.group)
		if err != nil {
			return nil, err
		}
	}

	// the leader kicks off the execution phase
	err = leader.dkgRunner.StartExecution()
	if err != nil {
		return nil, err
	}

	// advance by the grace period so all nodes kick off the DKG
	d.AdvanceMockClock(d.t, d.nodes[0].daemon.opts.dkgKickoffGracePeriod)

	// if there are any more hooks now (such as errors to trigger) we trigger them
	if hooks.postExecutionStart != nil {
		hooks.postExecutionStart()
	}

	// we wait up to 100 seconds for it to finish (or for the leader to get evicted)
	groupFile, err := d.WaitForDKG(leader, 2, 120)
	if err != nil {
		return nil, err
	}
	d.group = groupFile

	// these counts are small, let's just nested loop
	var reshared []*MockNode
	for _, groupNode := range groupFile.Nodes {
		for _, node := range d.nodes {
			if groupNode.Addr == node.addr {
				reshared = append(reshared, node)
			}
		}
	}
	d.resharedNodes = reshared
	return groupFile, nil
}

func (d *DrandTestScenario) WaitUntilRound(t *testing.T, node *MockNode, round uint64) error {
	counter := 0

	newClient, err := net.NewControlClient(node.drand.opts.controlPort)
	require.NoError(t, err)

	for {
		status, err := newClient.Status(d.beaconID)
		require.NoError(t, err)

		if !status.ChainStore.IsEmpty && status.ChainStore.LastRound >= round {
			t.Logf("node %s has reached expected round (%d)", node.addr, status.ChainStore.LastRound)
			return nil
		}

		counter++
		if counter == int(round)+10 {
			return fmt.Errorf("timeout waiting node %s to reach %d round", node.addr, round)
		}

		t.Logf("node %s is on %d round (vs expected %d), waiting some time to ask again...", node.addr, status.ChainStore.LastRound, round)
		time.Sleep(d.period)
	}
}

func (d *DrandTestScenario) WaitUntilChainIsServing(t *testing.T, node *MockNode) error {
	counter := 0

	newClient, err := net.NewControlClient(node.drand.opts.controlPort)
	require.NoError(t, err)

	for {
		status, err := newClient.Status(d.beaconID)
		require.NoError(t, err)

		if status.Beacon.IsServing {
			t.Logf("node %s has its beacon chain running on round %d", node.addr, status.ChainStore.LastRound)
			return nil
		}

		counter++
		if counter == 10 {
			return fmt.Errorf("timeout waiting node %s to run beacon chain", node.addr)
		}

		t.Logf("node %s has not got its beacon chain running yet, waiting some time to ask again...", node.addr)
		time.Sleep(1000 * time.Millisecond)
	}
}

// DenyClient can abort request to other needs based on a peer list
type DenyClient struct {
	t *testing.T
	net.ProtocolClient
	deny []string
}

func (bp *BeaconProcess) DenyBroadcastTo(t *testing.T, addresses ...string) {
	client := bp.privGateway.ProtocolClient
	bp.privGateway.ProtocolClient = &DenyClient{
		t:              t,
		ProtocolClient: client,
		deny:           addresses,
	}
}

func unixGetLimit() (curr, max uint64, err error) {
	rlimit := unix.Rlimit{}
	err = unix.Getrlimit(unix.RLIMIT_NOFILE, &rlimit)
	return rlimit.Cur, rlimit.Max, err
}

func unixSetLimit(soft, max uint64) error {
	rlimit := unix.Rlimit{
		Cur: soft,
		Max: max,
	}
	return unix.Setrlimit(unix.RLIMIT_NOFILE, &rlimit)
}
