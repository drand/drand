package dkg

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/crypto/dkg"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"
	"github.com/nikkolasg/slog"
	"github.com/stretchr/testify/require"
)

// testDKGServer implements a barebone service to be plugged in a net.DefaultService
type testDKGServer struct {
	*net.EmptyServer
	h *Handler
}

func (t *testDKGServer) Setup(c context.Context, in *drand.SetupPacket) (*drand.Empty, error) {
	t.h.Process(c, in.Dkg)
	return &drand.Empty{}, nil
}

func (t *testDKGServer) Reshare(c context.Context, in *drand.ResharePacket) (*drand.Empty, error) {
	t.h.Process(c, in.Dkg)
	return &drand.Empty{}, nil
}

// testNet implements the network interface that the dkg Handler expects
type testNet struct {
	fresh bool
	net.ProtocolClient
}

func (t *testNet) Send(p net.Peer, d *dkg.Packet) error {
	var err error
	if t.fresh {
		_, err = t.ProtocolClient.Setup(p, &drand.SetupPacket{Dkg: d})
	} else {
		_, err = t.ProtocolClient.Reshare(p, &drand.ResharePacket{Dkg: d})
	}
	return err
}

func testNets(n int, fresh bool) []*testNet {
	nets := make([]*testNet, n, n)
	for i := 0; i < n; i++ {
		nets[i] = &testNet{fresh: fresh, ProtocolClient: net.NewGrpcClient()}
		nets[i].SetTimeout(1 * time.Second)
	}
	return nets
}

type node struct {
	newNode  bool
	handler  *Handler
	listener net.Listener
	net      *testNet
	priv     *key.Pair
	pub      *key.Identity
}

type DKGTest struct {
	total    int
	keys     []string
	clock    *clock.Mock
	timeout  time.Duration
	newGroup *key.Group
	newNodes map[string]*node
	oldGroup *key.Group
	oldNodes map[string]*node

	callbacks map[string]func(*Handler)
}

func NewDKGTest(t *testing.T, n, thr int, timeout time.Duration) *DKGTest {
	privs := test.GenerateIDs(n)
	pubs := test.ListFromPrivates(privs)
	newGroup := key.NewGroup(pubs, thr)
	newNodes := make(map[string]*node)
	nets := testNets(n, true)
	keys := make([]string, n)
	clock := clock.NewMock()
	conf := Config{
		Suite:    key.KeyGroup.(Suite),
		NewNodes: newGroup,
		Timeout:  timeout,
		Clock:    clock,
	}
	for i := 0; i < n; i++ {
		c := conf
		c.Key = privs[i]
		var err error
		nets[i].SetTimeout(timeout / 2)
		handler, err := NewHandler(nets[i], &c, log.DefaultLogger)
		checkErr(err)
		dkgServer := testDKGServer{h: handler}
		listener := net.NewTCPGrpcListener(privs[i].Public.Addr, &dkgServer)

		newNodes[privs[i].Public.Address()] = &node{
			newNode:  true,
			pub:      pubs[i],
			priv:     privs[i],
			net:      nets[i],
			listener: listener,
			handler:  handler,
		}
		keys[i] = pubs[i].Address()
	}

	return &DKGTest{
		keys:      keys,
		total:     n,
		timeout:   timeout,
		newGroup:  newGroup,
		newNodes:  newNodes,
		callbacks: make(map[string]func(*Handler), n),
		clock:     clock,
	}
}

func NewDKGTestResharing(t *testing.T, oldN, oldT, newN, newT, common int, timeout time.Duration) *DKGTest {
	oldPrivs := test.GenerateIDs(oldN)
	oldPubs := test.ListFromPrivates(oldPrivs)
	oldShares, dpub := test.SimulateDKG(t, key.KeyGroup, oldN, oldT)
	oldGroup := key.LoadGroup(oldPubs, &key.DistPublic{Coefficients: dpub}, oldT)

	newToAdd := newN - common
	oldToRemove := oldN - common
	totalDKGs := oldN + newToAdd
	nets := testNets(totalDKGs, false)

	addPrivs := test.GenerateIDs(newToAdd)
	//addPubs := test.ListFromPrivates(addPrivs)

	newPrivs := make([]*key.Pair, 0, newN)
	newPubs := make([]*key.Identity, 0, newN)
	// the old nodes that are also in the new group
	for _, p := range oldPrivs[oldToRemove:] {
		newPrivs = append(newPrivs, p)
		newPubs = append(newPubs, p.Public)
	}
	// the new nodes not present in the old group
	for _, p := range addPrivs {
		newPrivs = append(newPrivs, p)
		newPubs = append(newPubs, p.Public)
	}
	newGroup := key.NewGroup(newPubs, newT)

	oldNodes := make(map[string]*node)
	keys := make([]string, totalDKGs)

	clock := clock.NewMock()
	conf := Config{
		Suite:    key.KeyGroup.(Suite),
		NewNodes: newGroup,
		OldNodes: oldGroup,
		Timeout:  timeout,
		Clock:    clock,
	}
	for i := 0; i < oldToRemove; i++ {
		c := conf
		c.Key = oldPrivs[i]
		groupIndex, ok := oldGroup.Index(c.Key.Public)
		require.True(t, ok)
		c.Share = &key.Share{
			Share:   oldShares[groupIndex],
			Commits: dpub,
		}
		var err error
		handler, err := NewHandler(nets[i], &c, log.DefaultLogger)
		checkErr(err)
		dkgServer := testDKGServer{h: handler}
		listener := net.NewTCPGrpcListener(c.Key.Public.Address(), &dkgServer)

		oldNodes[c.Key.Public.Address()] = &node{
			priv:     c.Key,
			pub:      c.Key.Public,
			net:      nets[i],
			handler:  handler,
			listener: listener,
			newNode:  false,
		}
		keys[i] = c.Key.Public.Address()
	}
	newNodes := make(map[string]*node)
	for i := 0; i < newN; i++ {
		c := conf
		c.Key = newPrivs[i]
		nnet := nets[oldToRemove+i]
		if i < common {
			groupIndex, ok := oldGroup.Index(c.Key.Public)
			require.True(t, ok)
			c.Share = &key.Share{
				Share:   oldShares[groupIndex],
				Commits: dpub,
			}
		}
		var err error
		handler, err := NewHandler(nnet, &c, log.DefaultLogger)
		checkErr(err)
		dkgServer := testDKGServer{h: handler}
		newNodes[c.Key.Public.Address()] = &node{
			priv:     c.Key,
			pub:      c.Key.Public,
			net:      nnet,
			listener: net.NewTCPGrpcListener(c.Key.Public.Address(), &dkgServer),
			handler:  handler,
			newNode:  true,
		}
		keys[oldToRemove+i] = c.Key.Public.Address()
	}
	return &DKGTest{
		total:     totalDKGs,
		keys:      keys,
		newGroup:  newGroup,
		newNodes:  newNodes,
		oldGroup:  oldGroup,
		oldNodes:  oldNodes,
		timeout:   timeout,
		callbacks: make(map[string]func(*Handler)),
		clock:     clock,
	}
}

func (d *DKGTest) tryBoth(id string, fn func(n *node), silent ...bool) {
	if n, ok := d.oldNodes[id]; ok {
		fn(n)
	} else if n, ok := d.newNodes[id]; ok {
		fn(n)
	} else {
		if len(silent) == 0 || !silent[0] {
			panic("that should not happen")
		}
	}
}

func (d *DKGTest) ServeDKG(id string) {
	d.tryBoth(id, func(n *node) {
		fmt.Printf("\t-| ServeDKG %s\n", n.pub.Address())
		go n.listener.Start()
		if cb := d.callbacks[id]; cb != nil {
			go cb(n.handler)
		}
		time.Sleep(10 * time.Millisecond)
	})
}

func (d *DKGTest) CallbackFor(id string, fn func(s *Share, e error, exit bool)) {
	d.callbacks[id] = func(h *Handler) {
		shareCh := h.WaitShare()
		errCh := h.WaitError()
		exitCh := h.WaitExit()
		select {
		case s := <-shareCh:
			fn(&s, nil, false)
		case err := <-errCh:
			fn(nil, err, false)
		case <-exitCh:
			fn(nil, nil, true)
		}
	}
}

func (d *DKGTest) WaitFinishFor(id string) {
	d.tryBoth(id, func(n *node) {
		if n.newNode {
			<-n.handler.WaitShare()
		} else {
			<-n.handler.WaitExit()
		}
	})
}

// wait for newNodes to finish
func (d *DKGTest) WaitFinish(min int, timeouta ...time.Duration) ([]string, bool) {
	timeouted := make(chan bool, 1)
	exit := make(chan bool, 1)
	timeout := 60 * time.Second
	if len(timeouta) > 0 {
		timeout = timeouta[0]
	}
	doneCh := make(chan string, d.total)
	for _, n := range d.newNodes {
		go func(nd *node) {
			h := nd.handler
			shareCh := h.WaitShare()
			errCh := h.WaitError()
			exitCh := h.WaitExit()
			select {
			case sh := <-shareCh:
				if sh.Commits == nil {
					panic("nil share")
				}
				doneCh <- nd.pub.Address()
			case <-exitCh:
			case err := <-errCh:
				checkErr(err)
			case <-d.clock.After(timeout):
				timeouted <- true
			case <-exit:
				return
			}
		}(n)
	}
	var ids []string
	for {
		select {
		case id := <-doneCh:
			ids = append(ids, id)
			//fmt.Printf(" \n\nDKG %d FINISHED \n\n", <-doneCh)
			if len(ids) == min {
				close(exit)
				return ids, false
			}
		case <-timeouted:
			return nil, true
		}
	}
}

// only care about QUAL from new nodes's point of view
func (d *DKGTest) CheckIncludedQUALFrom(from, dkg string) bool {
	n, ok := d.newNodes[from]
	if !ok {
		panic("that should not happen")
	}
	group := n.handler.QualifiedGroup()
	pub := n.pub
	return group.Contains(pub)
}

// check consistency amongt qual members
func (d *DKGTest) CheckIncludedQUAL(ids []string) bool {
	for _, id1 := range ids {
		for _, id2 := range ids {
			if !d.CheckIncludedQUALFrom(id1, id2) {
				return false
			}
		}
	}
	return true
}

func (d *DKGTest) StartDKG(id string) {
	d.tryBoth(id, func(n *node) {
		fmt.Printf(" -- Test - StartDKG for %s\n", n.pub.Address())
		n.handler.Start()
		time.Sleep(5 * time.Millisecond)
	})
}

func (d *DKGTest) newNodesA() []*node {
	a := make([]*node, 0, len(d.newNodes))
	for _, nn := range d.newNodes {
		a = append(a, nn)
	}
	return a
}

func (d *DKGTest) oldNodesA() []*node {
	a := make([]*node, 0, len(d.oldNodes))
	for _, nn := range d.oldNodes {
		a = append(a, nn)
	}
	return a
}

func (d *DKGTest) StopDKG(id string) {
	d.tryBoth(id, func(n *node) {
		n.listener.Stop()
	})
}

func (d *DKGTest) MoveTime(t time.Duration) {
	d.clock.Add(t)
	time.Sleep(10 * time.Millisecond)
}
func checkErr(e error) {
	if e != nil {
		panic(e)
	}
}

func TestDKGFresh(t *testing.T) {
	n := 5
	thr := key.DefaultThreshold(n)
	timeout := 2 * time.Second
	dt := NewDKGTest(t, n, thr, timeout)

	for _, k := range dt.keys {
		dt.ServeDKG(k)
	}

	dt.StartDKG(dt.keys[0])
	keys, _ := dt.WaitFinish(n)
	require.True(t, dt.CheckIncludedQUAL(keys))
}

func TestDKGWithTimeout(t *testing.T) {
	n := 7
	thr := key.DefaultThreshold(n)
	timeout := 1 * time.Second
	offline := n - thr
	alive := n - offline
	dt := NewDKGTest(t, n, thr, timeout)
	for _, k := range dt.keys[:alive] {
		dt.ServeDKG(k)
	}
	dt.StartDKG(dt.keys[0])
	// wait for all messages to come back and forth but less than timeout
	time.Sleep(700 * time.Millisecond)
	// trigger timeout immediatly
	dt.MoveTime(timeout * 2)
	keys, timeouted := dt.WaitFinish(alive)
	fmt.Printf("\n\n -- DKGTest Keys: %v\n\n", dt.keys)
	fmt.Printf("\n\n -- SERVING DKG ADDRESSES: %v\n\n", dt.keys[:alive])
	fmt.Printf("\n\n -- WaitFinish returned: %v\n\n", keys)
	// dkg should have finished,
	require.False(t, timeouted)
	require.True(t, dt.CheckIncludedQUAL(keys))
}

func TestDKGResharingPartialWithTimeout(t *testing.T) {
	slog.Level = slog.LevelDebug
	oldN := 7
	oldT := key.DefaultThreshold(oldN)
	// reshare with all but threshold of old nodes down
	newN := oldN + 1
	newT := oldT + 1
	common := oldT
	oldOffline := oldN - oldT
	newOffline := newN - newT
	timeout := 1000 * time.Millisecond
	dt := NewDKGTestResharing(t, oldN, oldT, newN, newT, common, timeout)
	fmt.Printf("Old Group:\n%s\n", dt.oldGroup.String())
	fmt.Printf("New Group:\n%s\n", dt.newGroup.String())
	// serve the old nodes online that wont be present in the new group
	for _, n := range dt.oldNodesA()[oldOffline:] {
		dt.ServeDKG(n.pub.Address())
		fmt.Printf(" -- Test - ServerDKG for %s\n", n.pub.Address())
		defer dt.StopDKG(n.pub.Address())
	}
	// serve the new (and old) nodes online - those who'll be present in the new
	// group
	for _, n := range dt.newNodesA()[newOffline:] {
		dt.ServeDKG(n.pub.Address())
		defer dt.StopDKG(n.pub.Address())
	}

	// start all nodes that are in the old group
	for _, id := range dt.oldGroup.Identities() {
		//for _, n := range dt.oldNodesA()[oldOffline:] {
		//go dt.StartDKG(n.pub.Address())
		go dt.StartDKG(id.Address())
	}

	// nobody should be finished before timeout
	go func() {
		// wait enough so the WaitFinish call starts already
		time.Sleep(100 * time.Millisecond)
		dt.MoveTime(timeout / 2)
	}()
	_, timeouted := dt.WaitFinish(newN-newOffline, timeout/2)
	require.True(t, timeouted)

	// every new online  should have finished after timeout
	go func() {
		time.Sleep(100 * time.Millisecond)
		dt.MoveTime(timeout)
		// time for the messages to pass through
		time.Sleep(100 * time.Millisecond)
	}()
	fmt.Println("BEFORE wait finishing timeouted #2")
	finished, to := dt.WaitFinish(newN - newOffline)
	fmt.Println("AFTER wait finishing timeouted #2")
	require.False(t, to)
	require.True(t, dt.CheckIncludedQUAL(finished))

	// XXX for nodes that don't participate in the new group, i.e. old nodes
	// quitting the group, they still dont know when the protocol finished ->
	// need some love
}

func TestDKGResharingNewNode(t *testing.T) {
	slog.Level = slog.LevelDebug
	oldN := 7
	oldT := key.DefaultThreshold(oldN)
	// reshare with one old node down and two new nodes = |card_old_group| + 1
	// first node is only there to reshare but wont be in the new group
	newN := oldN + 1
	newT := oldT + 1
	common := 0
	timeout := 1000 * time.Millisecond
	dt := NewDKGTestResharing(t, oldN, oldT, newN, newT, common, timeout)
	// serve the old nodes online
	for _, n := range dt.oldNodesA() {
		dt.ServeDKG(n.pub.Address())
		defer dt.StopDKG(n.pub.Address())
	}
	// serve the new nodes online
	for _, n := range dt.newNodesA() {
		dt.ServeDKG(n.pub.Address())
		defer dt.StopDKG(n.pub.Address())
	}

	// start all nodes that are in the old group
	for _, id := range dt.oldGroup.Identities() {
		go dt.StartDKG(id.Address())
	}

	finished, to := dt.WaitFinish(newN)
	require.False(t, to)
	require.True(t, dt.CheckIncludedQUAL(finished))

	// XXX for nodes that don't participate in the new group, i.e. old nodes
	// quitting the group, they still dont know when the protocol finished ->
	// need some love
}

func TestDKGResharingPartial(t *testing.T) {
	slog.Level = slog.LevelDebug
	oldN := 7
	oldT := key.DefaultThreshold(oldN)
	// reshare with one old node down and two new nodes = |card_old_group| + 1
	// first node is only there to reshare but wont be in the new group
	newN := oldN + 1
	newT := oldT + 1
	common := 0
	timeout := 1000 * time.Millisecond
	dt := NewDKGTestResharing(t, oldN, oldT, newN, newT, common, timeout)
	// serve the old nodes online
	for _, n := range dt.oldNodesA() {
		dt.ServeDKG(n.pub.Address())
		defer dt.StopDKG(n.pub.Address())
	}
	// serve the new nodes online
	for _, n := range dt.newNodesA() {
		dt.ServeDKG(n.pub.Address())
		defer dt.StopDKG(n.pub.Address())
	}

	// start all nodes that are in the old group
	for _, id := range dt.oldGroup.Identities() {
		go dt.StartDKG(id.Address())
	}

	finished, to := dt.WaitFinish(newN)
	require.False(t, to)
	require.True(t, dt.CheckIncludedQUAL(finished))
}

func TestDKGEntropy(t *testing.T) {
	// XXX TODO
}
