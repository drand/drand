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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/common"
	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"
	"github.com/drand/kyber/share/dkg"
)

const (
	ReshareUnlock = iota
	ReshareLock
)

type MockNode struct {
	addr     string
	certPath string
	daemon   *DrandDaemon
	drand    *BeaconProcess
	clock    clock.FakeClock
}

//nolint:gocritic
type DrandTestScenario struct {
	sync.Mutex

	// tmp dir for certificates, keys etc
	dir    string
	newDir string

	certPaths    []string
	newCertPaths []string
	// global clock on which all drand clocks are synchronized
	clock clock.FakeClock

	n             int
	thr           int
	newN          int
	newThr        int
	period        time.Duration
	catchupPeriod time.Duration
	scheme        scheme.Scheme
	beaconID      string

	// only set after the DKG
	group *key.Group
	// needed to give the group to new nodes during a resharing - only set after
	// a successful DKG
	groupPath string
	// only set after the resharing
	newGroup *key.Group
	// nodes that are created for running a first DKG
	nodes []*MockNode
	// new additional nodes that are created for running a resharing
	newNodes []*MockNode
	// nodes that actually ran the resharing phase - it's a combination of nodes
	// and new nodes. These are the one that should appear in the newGroup
	resharedNodes []*MockNode

	// the different beacon processes launched during the test
	drands []*BeaconProcess

	// used to write logs to files
	logFiles []*logFile
}

type logFile struct {
	*os.File
}

func newLogFile(t *testing.T, filename string) *logFile {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	require.NoError(t, err)
	return &logFile{
		File: file,
	}
}

func (lb *logFile) Close(t *testing.T, i int) {
	require.NoError(t, lb.File.Close())
	t.Logf("wrote server %d log file at %s\n", i, lb.File.Name())
}

// BatchNewDrand returns n drands, using TLS or not, with the given
// options. It returns the list of Drand structures, the group created,
// the folder where db, certificates, etc are stored. It is the folder
// to delete at the end of the test. As well, it returns a public grpc
// client that can reach any drand node.
// Deprecated: do not use
//
//nolint funlen // ignore this test
func BatchNewDrand(t *testing.T, n int, insecure bool, sch scheme.Scheme, beaconID string, opts ...ConfigOption) (
	daemons []*DrandDaemon, drands []*BeaconProcess, group *key.Group, dir string, certPaths []string, lBuffers []*logFile,
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

	logToFileString, _ := os.LookupEnv("DRAND_TEST_FILE_LOGS")
	logToFile := logToFileString == "true"

	tempDir := t.TempDir()
	var err error
	if logToFile {
		tempDir, err = os.MkdirTemp(os.TempDir(), fmt.Sprintf("drand-%s-*", t.Name()))
		require.NoError(t, err)

		// When writing the logs to files, want to have a fixed time value to make it easier
		// to compare the log files
		opts = append(opts, WithFixedClockLogs())
	}
	dir = path.Join(tempDir, common.MultiBeaconFolder)

	certPaths = make([]string, n)
	keyPaths := make([]string, n)
	dirs := make([]string, n)

	for i := 0; i < n; i++ {
		dirs[i] = path.Join(dir, fmt.Sprintf("drand-%d", i))
		if err := os.MkdirAll(dirs[i], 0o777); err != nil {
			require.NoError(t, err)
		}
	}

	if !insecure {
		for i := 0; i < n; i++ {
			certPath := path.Join(dirs[i], fmt.Sprintf("server-%d.crt", i))
			keyPath := path.Join(dirs[i], fmt.Sprintf("server-%d.key", i))

			if httpscerts.Check(certPath, keyPath) != nil {
				h, _, err := gnet.SplitHostPort(privs[i].Public.Address())
				assert.NoError(t, err)

				t.Logf("generate keys for drand %d", i)
				err = httpscerts.Generate(certPath, keyPath, h)
				assert.NoError(t, err)
			}
			certPaths[i] = certPath
			keyPaths[i] = keyPath
		}
	}

	logLevel := log.LogInfo
	debugEnv, isDebug := os.LookupEnv("DRAND_TEST_LOGS")
	if isDebug && debugEnv == "DEBUG" {
		t.Log("Enabling LogDebug logs")
		logLevel = log.LogDebug
	}

	var logFiles []*logFile

	for i := 0; i < n; i++ {
		s := test.NewKeyStore()

		assert.NoError(t, s.SaveKeyPair(privs[i]))

		// give each one their own private folder
		confOptions := []ConfigOption{WithConfigFolder(dirs[i])}
		confOptions = append(confOptions, WithPrivateListenAddress(privs[i].Public.Address()))
		if !insecure {
			confOptions = append(confOptions,
				WithTLS(certPaths[i], keyPaths[i]),
				WithTrustedCerts(certPaths...))
		} else {
			confOptions = append(confOptions, WithInsecure())
		}

		loggingOption := WithLogLevel(logLevel, false)
		if logToFile {
			fileName := path.Join(dir, fmt.Sprintf("server-log-%d.log", len(logFiles)))
			logger := newLogFile(t, fileName)
			logFiles = append(logFiles, logger)
			loggingOption = WithOutputAndLogLevel(logger, logLevel, true)
		}

		confOptions = append(confOptions,
			WithControlPort(ports[i]),
			loggingOption,
		)
		// add options in last so it overwrites the default
		confOptions = append(confOptions, opts...)

		t.Logf("Creating node %d", i)

		daemon, err := NewDrandDaemon(NewConfig(confOptions...))
		assert.NoError(t, err)

		bp, err := daemon.InstantiateBeaconProcess(beaconID, s)
		assert.NoError(t, err)

		daemons[i] = daemon
		drands[i] = bp
	}

	return daemons, drands, group, dir, certPaths, logFiles
}

// CloseAllDrands closes all drands
func CloseAllDrands(drands []*BeaconProcess) {
	for i := 0; i < len(drands); i++ {
		drands[i].Stop(context.Background())
	}
	for i := 0; i < len(drands); i++ {
		<-drands[i].WaitExit()
	}
}

// Deprecated: do not use sleeps in your tests
func getSleepDuration() time.Duration {
	if os.Getenv("CIRCLE_CI") != "" {
		fmt.Println("--- Sleeping on CIRCLECI")
		return time.Duration(600) * time.Millisecond
	}
	return time.Duration(500) * time.Millisecond
}

// NewDrandTest creates a drand test scenario with initial n nodes and ready to
// run a DKG for the given threshold that will then launch the beacon with the
// specified period
func NewDrandTestScenario(t *testing.T, n, thr int, period time.Duration, sch scheme.Scheme, beaconID string) *DrandTestScenario {
	dt := new(DrandTestScenario)
	beaconID = common.GetCanonicalBeaconID(beaconID)

	daemons, drands, _, dir, certPaths, logFiles := BatchNewDrand(
		t, n, false, sch, beaconID, WithCallOption(grpc.WaitForReady(true)),
	)

	dt.dir = dir
	dt.certPaths = certPaths
	dt.logFiles = logFiles
	dt.drands = drands
	dt.groupPath = path.Join(dt.dir, "group.toml")
	dt.n = n
	dt.scheme = sch
	dt.beaconID = beaconID
	dt.thr = thr
	dt.period = period
	dt.clock = clock.NewFakeClock()
	dt.nodes = make([]*MockNode, 0, n)

	for i, drandInstance := range drands {
		node := newNode(dt.clock.Now(), dt.certPaths[i], daemons[i], drandInstance)
		dt.nodes = append(dt.nodes, node)
		t.Logf("started node %d\n", len(dt.nodes)-1)
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

// waitForStatus waits and retries calling Status until the condition is satisfied or the max retries is reached
func (d *DrandTestScenario) waitFor(
	t *testing.T,
	client *net.ControlClient,
	maxRetries int, //nolint
	waitFor func(r *drand.StatusResponse) bool,
) bool {
	retry := 0
	for {
		r, err := client.Status(d.beaconID)
		require.NoError(t, err)
		if waitFor(r) {
			return true
		}
		if retry >= maxRetries {
			return false
		}

		time.Sleep(100 * time.Millisecond)
		retry++
	}
}

// RunDKG runs the DKG with the initial nodes created during NewDrandTest
//
//nolint:funlen
func (d *DrandTestScenario) RunDKG(t *testing.T) *key.Group {
	// common secret between participants
	secret := "thisisdkg"

	leaderNode := d.nodes[0]
	controlClient, err := net.NewControlClient(leaderNode.drand.opts.controlPort)
	required := require.New(t)
	required.NoError(err)

	t.Log("[RunDKG] Start: Leader = ", leaderNode.GetAddr())

	errDetector := make(chan error, d.n+1)
	var wg sync.WaitGroup
	wg.Add(d.n)

	runLeaderNode := func() {
		defer wg.Done()
		t.Log("[RunDKG] Leader (", leaderNode.GetAddr(), ") init")

		// TODO: Control Client needs every single parameter, not a protobuf type. This means that it will be difficult to extend
		groupPacket, err := controlClient.InitDKGLeader(
			d.n, d.thr, d.period, d.catchupPeriod, testDkgTimeout, nil, secret, testBeaconOffset, d.scheme.ID, d.beaconID)
		if err != nil {
			errDetector <- err
			return
		}

		t.Log("[RunDKG] Leader obtain group")
		group, err := key.GroupFromProto(groupPacket)
		if err != nil {
			errDetector <- err
			return
		}

		t.Logf("[RunDKG] Leader    Finished. GroupHash %x", group.Hash())

		// We need to make sure the daemon is running before continuing
		d.waitFor(t, controlClient, 10, func(r *drand.StatusResponse) bool {
			/// XXX: maybe needs to be changed if running and started aren't both necessary, using "isStarted" could maybe work too
			return r.Beacon.IsRunning
		})
		t.Logf("[DEBUG] leader node %s Status: isRunning", leaderNode.GetAddr())
	}

	// first run the leader and then run the other nodes
	go runLeaderNode()

	require.True(t, d.waitFor(t, controlClient, 10, func(r *drand.StatusResponse) bool {
		return r.Dkg.Status == uint32(DkgInProgress)
	}))
	t.Logf("[DEBUG] node: %s DKG Status: is in progress", leaderNode.GetAddr())

	// all other nodes will send their PK to the leader that will create the group
	for idx, node := range d.nodes[1:] {
		idx := idx
		node := node
		go func(n *MockNode) {
			defer wg.Done()

			t.Logf("[RunDKG] Node %d (%s) DKG init", idx+1, n.GetAddr())

			client, err := net.NewControlClient(n.drand.opts.controlPort)
			if err != nil {
				errDetector <- err
				return
			}
			groupPacket, err := client.InitDKG(leaderNode.drand.priv.Public, nil, secret, d.beaconID)
			if err != nil {
				errDetector <- err
				return
			}
			group, err := key.GroupFromProto(groupPacket)
			if err != nil {
				errDetector <- err
				return
			}

			t.Logf("[RunDKG] NonLeader %s Finished. GroupHash %x", n.GetAddr(), group.Hash())

			// We need to make sure the daemon is running before continuing
			d.waitFor(t, client, 10, func(r *drand.StatusResponse) bool {
				/// XXX: maybe needs to be changed if running and started aren't both necessary, using "isStarted" could maybe work too
				return r.Beacon.IsRunning
			})
			t.Logf("[DEBUG] follower node %s Status: isRunning", n.GetAddr())
		}(node)
	}

	// wait for all to return
	wg.Wait()

	close(errDetector)
	for e := range errDetector {
		required.NoError(e)
	}

	t.Logf("[RunDKG] Leader %s FINISHED", leaderNode.addr)

	// we check that we can fetch the group using control functionalities on the leaderNode node
	groupProto, err := controlClient.GroupFile(d.beaconID)
	require.NoError(t, err)
	t.Logf("[-------] Leader %s FINISHED", leaderNode.addr)
	group, err := key.GroupFromProto(groupProto)
	require.NoError(t, err)

	// we check all nodes are included in the group
	for _, node := range d.nodes {
		require.NotNil(t, group.Find(node.drand.priv.Public))
	}

	// we check the group has the right threshold
	require.Len(t, group.PublicKey.Coefficients, d.thr)
	require.Equal(t, d.thr, group.Threshold)
	require.NoError(t, key.Save(d.groupPath, group, false))

	d.group = group
	t.Log("[RunDKG] READY!")
	return group
}

func (d *DrandTestScenario) Cleanup(t *testing.T) {
	d.CloseAllDaemons(t)

	if len(d.logFiles) != 0 {
		closeLogFiles(t, d.logFiles)
		return
	}

	// Do not perform a cleanup of temp dirs if we write logs to files
	// when using DRAND_TEST_FILE_LOGS=true env var setting

	err := os.RemoveAll(d.dir)
	require.NoError(t, err)
	err = os.RemoveAll(d.newDir)
	require.NoError(t, err)
}

func closeLogFiles(t *testing.T, logFiles []*logFile) {
	for i := 0; i < len(logFiles); i++ {
		logFiles[i].Close(t, i)
	}
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
		return node.drand.beacon.Store().Get(uint64(round))
	}
	return nil, errors.New("that should not happen")
}

// GetMockNode returns the node associated with this address, either in the new
// group or the current group.
func (d *DrandTestScenario) GetMockNode(t *testing.T, nodeAddress string, newGroup bool) *MockNode {
	nodes := d.nodes
	if newGroup {
		nodes = d.resharedNodes
	}

	var result *MockNode
	for _, node := range nodes {
		if node.addr == nodeAddress {
			result = node
			break
		}
	}

	require.NotNil(t, result)
	return result
}

// StopMockNode stops a node from the first group
func (d *DrandTestScenario) StopMockNode(t *testing.T, nodeAddr string, newGroup bool) {
	node := d.GetMockNode(t, nodeAddr, newGroup)

	dr := node.drand
	dr.Stop(context.Background())
	t.Logf("[drand] stop %s", dr.priv.Public.Address())

	controlClient, err := net.NewControlClient(dr.opts.controlPort)
	require.NoError(t, err)

	retryCount := 1
	maxRetries := 5
	for range time.Tick(100 * time.Millisecond) {
		t.Logf("[drand] ping %s: %d/%d", dr.priv.Public.Address(), retryCount, maxRetries)
		response, err := controlClient.Status(d.beaconID)
		if err != nil {
			break
		}
		if response.Beacon.Status == uint32(BeaconNotInited) {
			break
		}

		retryCount++
		require.LessOrEqual(t, retryCount, maxRetries)
	}

	t.Logf("[drand] stopped %s", dr.priv.Public.Address())
}

// StartDrand fetches the drand given the id, in the respective group given the
// newGroup parameter and runs the beacon
func (d *DrandTestScenario) StartDrand(t *testing.T, nodeAddress string, catchup, newGroup bool) {
	node := d.GetMockNode(t, nodeAddress, newGroup)
	dr := node.drand

	t.Logf("[drand] Start")
	dr.StartBeacon(catchup)
	t.Logf("[drand] Started")
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
		t.Logf("ALREADY PASSED")
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
func (d *DrandTestScenario) CheckPublicBeacon(t *testing.T, nodeAddress string, newGroup bool) *drand.PublicRandResponse {
	node := d.GetMockNode(t, nodeAddress, newGroup)
	dr := node.drand

	client := net.NewGrpcClientFromCertManager(dr.opts.certmanager, dr.opts.grpcOpts...)
	resp, err := client.PublicRand(context.TODO(), test.NewTLSPeer(dr.priv.Public.Addr), &drand.PublicRandRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp)
	return resp
}

// SetupNewNodes creates new additional nodes that can participate during the resharing
func (d *DrandTestScenario) SetupNewNodes(t *testing.T, newNodes int) []*MockNode {
	newDaemons, newDrands, _, newDir, newCertPaths, logFiles := BatchNewDrand(t, newNodes, false, d.scheme, d.beaconID,
		WithCallOption(grpc.WaitForReady(false)))
	d.logFiles = append(d.logFiles, logFiles...)
	d.drands = append(d.drands, newDrands...)
	d.newCertPaths = newCertPaths
	d.newDir = newDir
	d.newNodes = make([]*MockNode, newNodes)

	// add certificates of new nodes to the old nodes
	for _, node := range d.nodes {
		inst := node.drand
		for _, cp := range newCertPaths {
			err := inst.opts.certmanager.Add(cp)
			require.NoError(t, err)
		}
	}

	// store new part. and add certificate path of current nodes to the new
	d.newNodes = make([]*MockNode, 0, newNodes)
	for i, inst := range newDrands {
		node := newNode(d.clock.Now(), newCertPaths[i], newDaemons[i], inst)
		d.newNodes = append(d.newNodes, node)
		for _, cp := range d.certPaths {
			err := inst.opts.certmanager.Add(cp)
			require.NoError(t, err)
		}
		t.Logf("started new node %d\n", len(d.newNodes)-1)
	}
	return d.newNodes
}

func (d *DrandTestScenario) WaitUntilRound(t *testing.T, node *MockNode, round uint64) error {
	counter := 0

	newClient, err := net.NewControlClient(node.drand.opts.controlPort)
	require.NoError(t, err)

	for {
		status, err := newClient.Status(d.beaconID)
		require.NoError(t, err)

		if !status.ChainStore.IsEmpty && status.ChainStore.LastRound == round {
			t.Logf("node %s is on expected round (%d)", node.addr, status.ChainStore.LastRound)
			return nil
		}

		counter++
		if counter == 10 {
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

		t.Logf("node %s has its beacon chain not running yet, waiting some time to ask again...", node.addr)
		time.Sleep(500 * time.Millisecond)
	}
}

func (d *DrandTestScenario) runNodeReshare(t *testing.T, n *MockNode, errCh chan error, force bool) {
	secret := "thisistheresharing"

	leader := d.nodes[0]

	// instruct to be ready for resharing
	client, err := net.NewControlClient(n.drand.opts.controlPort)
	require.NoError(t, err)

	t.Logf("[reshare:node] init reshare")
	_, err = client.InitReshare(leader.drand.priv.Public, secret, d.groupPath, force, d.beaconID)
	if err != nil {
		t.Log("[reshare:node] error in NON LEADER: ", err)
		errCh <- err
		return
	}

	t.Logf("[reshare]  non-leader drand %s DONE - %s", n.drand.priv.Public.Address(), n.drand.priv.Public.Key)
}

func (d *DrandTestScenario) runLeaderReshare(
	t *testing.T,
	leader *MockNode,
	client *net.ControlClient,
	newN, newThr int,
	timeout time.Duration,
	errCh chan error,
	groupReceivedCh chan *key.Group) {
	secret := "thisistheresharing"

	oldNode := d.group.Find(leader.drand.priv.Public)
	require.NotNil(t, oldNode, "[reshare:leader] leader not found in old group")

	// Start reshare
	t.Logf("[reshare:leader] init reshare")
	finalGroup, err := client.InitReshareLeader(newN, newThr, timeout, 0, secret, "", testBeaconOffset, d.beaconID)
	if err != nil {
		t.Log("[reshare:leader] error: ", err)
		errCh <- err
		return
	}

	t.Logf("[reshare:leader] reshare finished - got group")
	fg, err := key.GroupFromProto(finalGroup)
	if err != nil {
		errCh <- err
		return
	}

	groupReceivedCh <- fg
}

type reshareConfig struct {
	oldRun     int           // how many current nodes do we take for resharing
	newRun     int           // how many new nodes do we spawn
	newThr     int           // what is the new threshold
	timeout    time.Duration // timeout of the DKG parameter
	force      bool          // is this a force reshare - leader parameter
	onlyLeader bool          // only the leader will start the process
	ignoreErr  bool          // we ignore the error returned by process
	stateCh    chan int      // pass info on the state of the resharing
	expDeals   int           // how many deals a receiving node should receive
	expResps   int           // how many resps a receiving node should receive
}

func (r *reshareConfig) RelyOnTimeout() bool {
	if r.expDeals == 0 && r.expResps == 0 {
		return true
	}
	return false
}

//nolint:gocritic
func (r *reshareConfig) ExpectedDealsAndResps() (int, int) {
	expDeals := r.expDeals
	if r.expDeals == 0 {
		// only the old nodes send a deal but everyone receives it
		expDeals = (r.oldRun + r.newRun) * (r.oldRun - 1)
	}
	expResps := r.expResps
	if r.expResps == 0 {
		// everyone participating send a response to everyone except themselves
		expResps = (r.oldRun + r.newRun) * (r.oldRun + r.newRun - 1)
	}
	return expDeals, expResps
}

// RunReshare runs the resharing procedure with only "oldRun" current nodes
// running, and "newRun" new nodes running (the ones created via SetupNewNodes).
// It sets the given threshold to the group.
// It stops the nodes excluded first.
//
//nolint:funlen
func (d *DrandTestScenario) RunReshare(t *testing.T, c *reshareConfig) (*key.Group, error) {
	if c.ignoreErr {
		t.Log("[reshare] WARNING IGNORING ERRORS!!!")
	}

	d.Lock()
	if c.stateCh != nil {
		c.stateCh <- ReshareLock
	}

	t.Logf("[reshare] LOCK")
	t.Logf("[reshare] old: %d/%d | new: %d/%d", c.oldRun, len(d.nodes), c.newRun, len(d.newNodes))

	// stop the excluded nodes
	for i, node := range d.nodes[c.oldRun:] {
		t.Logf("[reshare] stop old %d | %s", i, node.addr)
		d.StopMockNode(t, node.addr, false)
	}

	if len(d.newNodes) > 0 {
		for i, node := range d.newNodes[c.newRun:] {
			t.Logf("[reshare] stop new %d | %s", i, node.addr)
			d.StopMockNode(t, node.addr, true)
		}
	}

	d.newN = c.oldRun + c.newRun
	d.newThr = c.newThr
	leader := d.nodes[0]

	// Create channels
	errCh := make(chan error, 1)
	leaderGroupReadyCh := make(chan *key.Group, 1)
	// setup the channel where we can see all node-initiated outgoing packets
	// for the DKG
	outgoingChan := make(chan packet, 100)
	incomingChan := make(chan packet, 100)
	broadcastSetup := func(b Broadcast) Broadcast {
		return newTestBroadcast(b, outgoingChan, incomingChan)
	}
	leader.drand.dkgBoardSetup = broadcastSetup

	// wait until leader is listening
	controlClient, err := net.NewControlClient(leader.drand.opts.ControlPort())
	require.NoError(t, err)

	// first run the leader, then the other nodes will send their PK to the
	// leader and then the leader will answer back with the new group
	go d.runLeaderReshare(t, leader, controlClient, d.newN, d.newThr, c.timeout, errCh, leaderGroupReadyCh)
	d.resharedNodes = append(d.resharedNodes, leader)

	require.True(t, d.waitFor(t, controlClient, 10, func(r *drand.StatusResponse) bool {
		return r.Reshare.Status == uint32(ReshareInProgress)
	}))
	t.Logf("[DEBUG] node: %s Reshare Status: is in progress", leader.GetAddr())

	// run the current nodes
	for _, node := range d.nodes[1:c.oldRun] {
		node := node
		d.resharedNodes = append(d.resharedNodes, node)
		if !c.onlyLeader {
			node.drand.dkgBoardSetup = broadcastSetup
			t.Logf("[reshare] run node reshare %s", node.addr)
			go d.runNodeReshare(t, node, errCh, c.force)
		}
	}

	// run the new ones
	if len(d.newNodes) > 0 {
		for _, node := range d.newNodes[:c.newRun] {
			node := node
			d.resharedNodes = append(d.resharedNodes, node)
			if !c.onlyLeader {
				node.drand.dkgBoardSetup = broadcastSetup
				t.Logf("[reshare] run node reshare %s (new)", node.addr)
				go d.runNodeReshare(t, node, errCh, c.force)
			}
		}
	}

	t.Logf("[reshare] unlock")
	d.Unlock()

	if c.stateCh != nil {
		c.stateCh <- ReshareUnlock
		if c.onlyLeader {
			// no need to continue since only the leader wont do
			// we only use this for DKgReshareForce
			t.Logf(" \n LEAVING THE LEADER_ONLY RESHARING\n\n")
			return nil, errPreempted
		}
	}

	// wait for the return of the clients
	var howManyDeals int
	var howManyResps int
	expDeals, expResps := c.ExpectedDealsAndResps()
	relyOnTimeout := c.RelyOnTimeout()
	for {
		select {
		case finalGroup := <-leaderGroupReadyCh:
			t.Logf("[reshare] Received group!")
			d.newGroup = finalGroup
			require.NoError(t, key.Save(d.groupPath, d.newGroup, false))
			t.Logf("[reshare] Finish")
			return finalGroup, nil

		case err := <-errCh:
			t.Logf("[reshare] ERROR: %s", err)
			if !c.ignoreErr {
				t.Logf("[reshare] Finish - ignore error")
				return nil, err
			}
		case <-time.After(500 * time.Millisecond):
			// we only advance at the justification phase freely
			// we wait for all messages during deal and
			// response phase
			if !relyOnTimeout {
				continue
			}
			// XXX: check if this is really intended
			d.AdvanceMockClock(t, c.timeout)
			t.Logf("[reshare] Advance clock: %d", d.Now().Unix())
		case <-outgoingChan:
			continue
		case p := <-incomingChan:
			if relyOnTimeout {
				continue
			}
			switch p.(type) {
			case *dkg.DealBundle:
				howManyDeals++
				t.Logf("\n!!! -- %d DEALS (vs exp %d)-- received so far !!! \n", howManyDeals, expDeals)
			case *dkg.ResponseBundle:
				howManyResps++
				t.Logf("\n!!! -- %d RESPS (vs exp %d)-- received so far !!! \n", howManyResps, expResps)
			case *dkg.JustificationBundle:
				continue
			default:
				t.Fatal("impossible to receive an unknown packet type")
			}
			if howManyDeals == expDeals {
				howManyDeals++ // make sure we dont do that again
				d.AdvanceMockClock(t, d.period)
				t.Logf("[reshare] All deals RECEIVED -> Advance clock: %d", d.Now().Unix())
			} else if howManyResps == expResps {
				howManyResps++
				d.AdvanceMockClock(t, d.period)
				t.Logf("[reshare] All responses RECEIVED -> Advance clock: %d", d.Now().Unix())
				relyOnTimeout = true
			}
		}
	}
}

// CloseAllDrands closes all running
func (d *DrandTestScenario) CloseAllDrands() {
	CloseAllDrands(d.drands)
}

// CloseAllDaemons closes all the nodes in a proper manner
func (d *DrandTestScenario) CloseAllDaemons(t *testing.T) {
	for i := 0; i < len(d.nodes); i++ {
		t.Logf("stopping node %d\n", i)
		d.nodes[i].Shutdown(t)
	}
	for i := 0; i < len(d.newNodes); i++ {
		t.Logf("stopping new node %d\n", i)
		d.newNodes[i].Shutdown(t)
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

func (d *DenyClient) BroadcastDKG(c context.Context, p net.Peer, in *drand.DKGPacket, opts ...net.CallOption) error {
	if !d.isAllowed(p) {
		d.t.Logf("[DKG] Deny communication %s\n", p.Address())
		return errors.New("dkg broadcast denied")
	}

	return d.ProtocolClient.BroadcastDKG(c, p, in)
}

func (d *DenyClient) isAllowed(p net.Peer) bool {
	for _, s := range d.deny {
		if p.Address() == s {
			return false
		}
	}
	return true
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

// newNode creates a node struct from a drand and sets the clock according to the drand test clock.
func newNode(now time.Time, certPath string, daemon *DrandDaemon, dr *BeaconProcess) *MockNode {
	id := dr.priv.Public.Address()
	c := clock.NewFakeClockAt(now)

	// Note: not pure
	dr.opts.clock = c

	return &MockNode{
		certPath: certPath,
		addr:     id,
		daemon:   daemon,
		drand:    dr,
		clock:    c,
	}
}

type testBroadcast struct {
	*echoBroadcast
	outgoing chan packet
	incoming chan packet
	hashes   set
}

func newTestBroadcast(b Broadcast, ingoing, outgoing chan packet) Broadcast {
	return &testBroadcast{
		hashes:        new(arraySet),
		echoBroadcast: b.(*echoBroadcast),
		outgoing:      outgoing,
		incoming:      ingoing,
	}
}

func (b *testBroadcast) PushDeals(bundle *dkg.DealBundle) {
	b.echoBroadcast.PushDeals(bundle)
	b.outgoing <- bundle
}

func (b *testBroadcast) PushResponses(bundle *dkg.ResponseBundle) {
	b.echoBroadcast.PushResponses(bundle)
	b.outgoing <- bundle
}

func (b *testBroadcast) PushJustifications(bundle *dkg.JustificationBundle) {
	b.echoBroadcast.PushJustifications(bundle)
	b.outgoing <- bundle
}

func (b *testBroadcast) BroadcastDKG(c context.Context, p *drand.DKGPacket) error {
	err := b.echoBroadcast.BroadcastDKG(c, p)
	if err != nil {
		return err
	}

	dkgPacket, _ := protoToDKGPacket(p.GetDkg())
	hash := hash(dkgPacket.Hash())
	b.Lock()
	defer b.Unlock()
	if !b.hashes.exists(hash) {
		b.incoming <- dkgPacket
	}
	return nil
}

func (n *MockNode) GetAddr() string {
	return n.addr
}

// Shutdown sends the shutdown commadn to the daemon and then it waits for it to complete
// all the requests. The signal will come on the exit channel.
// NOTE: The channel is not currently closed so it has to be drained every time we call
//
//	the Shutdown command. Otherwise, we risk having the channel stuck at the sending
//	operation and thus never finish the operation.
func (n *MockNode) Shutdown(t *testing.T) {
	echan := make(chan error, 1)
	go func() {
		_, err := n.daemon.Shutdown(context.Background(), &drand.ShutdownRequest{})
		echan <- err
	}()

	// Wait for the daemon to stop properly
	<-n.daemon.WaitExit()

	if err := <-echan; err != nil {
		if !assert.NoError(t, err, "failed to shutdown") {
			t.FailNow()
		}
	}
}
