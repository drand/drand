package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"
	clock "github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type Node struct {
	addr     string
	certPath string
	drand    *Drand
	clock    clock.FakeClock
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

	n             int
	thr           int
	newN          int
	newThr        int
	period        time.Duration
	catchupPeriod time.Duration
	// only set after the DKG
	group *key.Group
	// needed to give the group to new nodes during a resharing - only set after
	// a successful DKG
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
		WithCallOption(grpc.WaitForReady(true)),
	)
	dt.t = t
	dt.dir = dir
	dt.certPaths = certPaths
	dt.n = n
	dt.thr = thr
	dt.period = period
	dt.clock = clock.NewFakeClock()
	dt.nodes = make([]*Node, 0, n)
	for i, drand := range drands {
		node := dt.newNode(drand, dt.certPaths[i])
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
		_, err := controlClient.InitDKGLeader(d.n, d.thr, d.period, d.catchupPeriod, testDkgTimeout, nil, secret, testBeaconOffset)
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
			_, err = client.InitDKG(root.drand.priv.Public, nil, secret)
			require.NoError(d.t, err)
			fmt.Printf("\n\nTEST NONLEADER FINISHED\n\n")
			wg.Done()
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
func (d *DrandTest2) GetBeacon(id string, round int, newGroup bool) (*chain.Beacon, error) {
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
	dr.Stop(context.Background())
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
func (d *DrandTest2) StartDrand(id string, catchup, newGroup bool) {
	node := d.GetDrand(id, newGroup)
	dr := node.drand
	// we load it from scratch as if the operator restarted its node
	newDrand, err := LoadDrand(dr.store, dr.opts)
	require.NoError(d.t, err)
	node.drand = newDrand
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
	} else {
		fmt.Println("ALREADY PASSED")
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
			var found bool
			for i := 0; i < 10; i++ {
				inst := node.drand
				if length != inst.beacon.Store().Len() {
					time.Sleep(getSleepDuration())
					continue
				}
				found = true
				break
			}
			require.True(d.t, found, "node %d not have enough beacon", id)
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
func (d *DrandTest2) SetupNewNodes(newNodes int) []*Node {
	newDrands, _, newDir, newCertPaths := BatchNewDrand(newNodes, false,
		WithCallOption(grpc.WaitForReady(false)), WithLogLevel(log.LogDebug))
	d.newCertPaths = newCertPaths
	d.newDir = newDir
	d.newNodes = make([]*Node, newNodes)
	// add certificates of new nodes to the old nodes
	for _, node := range d.nodes {
		inst := node.drand
		for _, cp := range newCertPaths {
			inst.opts.certmanager.Add(cp)
		}
	}
	// store new part. and add certificate path of current nodes to the new
	d.newNodes = make([]*Node, 0, newNodes)
	for i, inst := range newDrands {
		node := d.newNode(inst, newCertPaths[i])
		d.newNodes = append(d.newNodes, node)
		for _, cp := range d.certPaths {
			inst.opts.certmanager.Add(cp)
		}
	}
	return d.newNodes
}

// RunReshare runs the resharing procedure with only "oldRun" current nodes
// running, and "newRun" new nodes running (the ones created via SetupNewNodes).
// It sets the given threshold to the group.
// It stops the nodes excluded first.
func (d *DrandTest2) RunReshare(oldRun, newRun, newThr int, timeout time.Duration, force, onlyLeader bool) (*key.Group, error) {
	d.Lock()
	fmt.Printf(" -- Running RESHARE with %d/%d old, %d/%d new nodes\n", oldRun, len(d.nodes), newRun, len(d.newNodes))
	var secret = "thisistheresharing"
	// stop the exluded nodes
	for _, node := range d.nodes[oldRun:] {
		d.StopDrand(node.addr, false)
	}
	if len(d.newNodes) > 0 {
		for _, node := range d.newNodes[newRun:] {
			d.StopDrand(node.addr, true)
		}
	}

	d.newN = oldRun + newRun
	d.newThr = newThr
	leader := d.nodes[0]
	errCh := make(chan error, 1)
	groupCh := make(chan *key.Group, 1)
	// function that each non-leader runs to start the resharing
	runreshare := func(n *Node) {
		if onlyLeader {
			return
		}
		// instruct to be ready for a reshare
		client, err := net.NewControlClient(n.drand.opts.controlPort)
		require.NoError(d.t, err)
		_, err = client.InitReshare(leader.drand.priv.Public, secret, d.groupPath, force)
		if err != nil {
			errCh <- err
			return
		}
		fmt.Printf("\n\nRESHARING TEST: non-leader drand %s DONE RESHARING - %s\n", n.drand.priv.Public.Address(), n.drand.priv.Public.Key)
	}
	// first run the leader, then the other nodes will send their PK to the
	// leader and then the leader will answer back with the new group
	go func() {
		oldNode := d.group.Find(leader.drand.priv.Public)
		if oldNode == nil {
			panic("leader not found in old group")
		}
		// old root: oldNode.Index leater: leader.addr
		client, err := net.NewControlClient(leader.drand.opts.controlPort)
		require.NoError(d.t, err)
		finalGroup, err := client.InitReshareLeader(d.newN, d.newThr, timeout, 0, secret, "", testBeaconOffset)
		// Done resharing
		if err != nil {
			errCh <- err
		}
		fg, err := key.GroupFromProto(finalGroup)
		if err != nil {
			errCh <- err
		}
		groupCh <- fg
	}()
	d.resharedNodes = append(d.resharedNodes, leader)
	// leave some time to make sure leader is listening
	time.Sleep(1 * time.Second)

	// run the current nodes
	for _, node := range d.nodes[1:oldRun] {
		d.resharedNodes = append(d.resharedNodes, node)
		go runreshare(node)
	}

	// run the new ones
	if len(d.newNodes) > 0 {
		for _, node := range d.newNodes[:newRun] {
			d.resharedNodes = append(d.resharedNodes, node)
			fmt.Printf("\n ++ NEW NODE running RESHARE: %s\n", node.addr)
			go runreshare(node)
		}
	}
	d.Unlock()
	// wait for the return of the clients
	select {
	case finalGroup := <-groupCh:
		d.newGroup = finalGroup
		require.NoError(d.t, key.Save(d.groupPath, d.newGroup, false))
		return finalGroup, nil
	case err := <-errCh:
		fmt.Println("ERRROR: ", err)
		return nil, err
	}
}

// newNode creates a node struct from a drand and sets the clock according to
// the drand test clock.
func (d *DrandTest2) newNode(dr *Drand, certPath string) *Node {
	id := dr.priv.Public.Address()
	now := d.clock.Now()
	c := clock.NewFakeClockAt(now)
	dr.opts.clock = c
	return &Node{
		certPath: certPath,
		addr:     id,
		drand:    dr,
		clock:    c,
	}
}

// DenyClient can abort request to other needs based on a peer list
type DenyClient struct {
	net.ProtocolClient
	deny []string
}

func (d *Drand) DenyBroadcastTo(addrs ...string) {
	client := d.privGateway.ProtocolClient
	d.privGateway.ProtocolClient = &DenyClient{
		ProtocolClient: client,
		deny:           addrs,
	}
}

func (d *DenyClient) BroadcastDKG(c context.Context, p net.Peer, in *drand.DKGPacket, opts ...net.CallOption) error {
	if !d.isAllowed(p) {
		d := make(chan bool)
		fmt.Printf("\nDENIAL BROADCAST DKG TO %s\n", p.Address())
		<-d
		return nil
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
