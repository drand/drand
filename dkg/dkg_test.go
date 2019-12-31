package dkg

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/log"
	"github.com/dedis/drand/net"
	"github.com/dedis/drand/protobuf/crypto/dkg"
	"github.com/dedis/drand/protobuf/drand"
	"github.com/dedis/drand/test"
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
	for _, p := range oldPrivs[oldToRemove:] {
		newPrivs = append(newPrivs, p)
		newPubs = append(newPubs, p.Public)
	}
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
		c.Share = &key.Share{
			Share:   oldShares[i],
			Commits: dpub,
		}
		var err error
		handler, err := NewHandler(nets[i], &c, log.DefaultLogger)
		checkErr(err)
		dkgServer := testDKGServer{h: handler}
		listener := net.NewTCPGrpcListener(oldPrivs[i].Public.Addr, &dkgServer)

		oldNodes[oldPubs[i].Address()] = &node{
			priv:     oldPrivs[i],
			pub:      oldPubs[i],
			net:      nets[i],
			handler:  handler,
			listener: listener,
			newNode:  false,
		}
		keys[i] = oldPubs[i].Address()
	}
	newNodes := make(map[string]*node)
	for i := 0; i < newN; i++ {
		c := conf
		c.Key = newPrivs[i]
		if i < common {
			c.Share = &key.Share{
				Share:   oldShares[oldToRemove+i],
				Commits: dpub,
			}
		}
		var err error
		handler, err := NewHandler(nets[i], &c, log.DefaultLogger)
		checkErr(err)
		dkgServer := testDKGServer{h: handler}
		newNodes[newPubs[i].Address()] = &node{
			priv:     newPrivs[i],
			pub:      newPubs[i],
			net:      nets[oldToRemove+i],
			listener: net.NewTCPGrpcListener(newPrivs[i].Public.Addr, &dkgServer),
			handler:  handler,
			newNode:  true,
		}
		keys[oldToRemove+i] = newPubs[i].Address()
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
		go func(n *node) {
			h := n.handler
			shareCh := h.WaitShare()
			errCh := h.WaitError()
			exitCh := h.WaitExit()
			select {
			case <-shareCh:
				doneCh <- n.pub.Address()
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
		fmt.Printf("\n\nstart DKG id %s\n\n\n", id)
		n.handler.Start()
		fmt.Printf("\n\nstart DKG id DONE%s\n\n\n", id)
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
	keys, _ := dt.WaitFinish(alive)
	require.True(t, dt.CheckIncludedQUAL(keys))
}

func TestDKGResharingPartialWithTimeout(t *testing.T) {
	slog.Level = slog.LevelDebug
	oldN := 7
	oldT := key.DefaultThreshold(oldN)
	// reshare with one old node down and two new nodes = |card_old_group| + 1
	// first node is only there to reshare but wont be in the new group
	newN := oldN + 1
	newT := oldT + 1
	common := oldN - 1
	oldOffline := 1
	newOffline := newN - newT
	timeout := 1000 * time.Millisecond
	dt := NewDKGTestResharing(t, oldN, oldT, newN, newT, common, timeout)
	// serve the old nodes online
	for _, n := range dt.oldNodesA()[oldOffline:] {
		dt.ServeDKG(n.pub.Address())
		defer dt.StopDKG(n.pub.Address())
	}
	// serve the new nodes online
	for _, n := range dt.newNodesA()[newOffline:] {
		dt.ServeDKG(n.pub.Address())
		defer dt.StopDKG(n.pub.Address())
	}

	// start all nodes that are in the old group
	for _, id := range dt.oldGroup.Identities() {
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
	oldN := 5
	oldT := key.DefaultThreshold(oldN)
	oldPrivs := test.GenerateIDs(oldN)
	oldPubs := test.ListFromPrivates(oldPrivs)

	oldShares, dpub := test.SimulateDKG(t, key.KeyGroup, oldN, oldT)
	oldGroup := key.LoadGroup(oldPubs, &key.DistPublic{Coefficients: dpub}, oldT)

	newN := oldN + 1
	newT := oldT + 1

	newPrivs := test.GenerateIDs(newN)
	newPubs := test.ListFromPrivates(newPrivs)
	newGroup := key.NewGroup(newPubs, newT)

	require.Equal(t, len(newPrivs), newN)

	total := newN + oldN
	nets := testNets(total, false)
	handlers := make([]*Handler, total)
	listeners := make([]net.Listener, total)
	var err error

	// old nodes
	for i := 0; i < oldN; i++ {
		share := key.Share{Commits: dpub, Share: oldShares[i]}
		conf := &Config{
			Suite:    key.KeyGroup.(Suite),
			Key:      oldPrivs[i],
			OldNodes: oldGroup,
			NewNodes: newGroup,
			Share:    &share,
		}
		handlers[i], err = NewHandler(nets[i], conf, log.DefaultLogger)
		require.NoError(t, err)

		dkgServer := testDKGServer{h: handlers[i]}
		listeners[i] = net.NewTCPGrpcListener(oldPrivs[i].Public.Addr, &dkgServer)
		go listeners[i].Start()
	}
	// new nodes
	for i := oldN; i < total; i++ {
		newIdx := i - oldN
		conf := &Config{
			Suite:    key.KeyGroup.(Suite),
			Key:      newPrivs[newIdx],
			NewNodes: newGroup,
			OldNodes: oldGroup,
		}

		handlers[i], err = NewHandler(nets[i], conf, log.DefaultLogger)
		require.NoError(t, err)
		require.True(t, handlers[i].newNode)
		require.False(t, handlers[i].oldNode)
		require.Equal(t, handlers[i].nidx, newIdx)
		dkgServer := testDKGServer{h: handlers[i]}
		listeners[i] = net.NewTCPGrpcListener(newPrivs[newIdx].Public.Addr, &dkgServer)
		go listeners[i].Start()

	}

	defer func() {
		for i := range listeners {
			listeners[i].Stop()
		}
	}()

	finished := make(chan int, total)
	quitAll := make(chan bool)
	goDkg := func(idx int) {
		if idx < oldN {
			go handlers[idx].Start()
		}
		shareCh := handlers[idx].WaitShare()
		errCh := handlers[idx].WaitError()
		exitCh := handlers[idx].WaitExit()
		select {
		case <-shareCh:
			finished <- idx
		case <-exitCh:
			finished <- idx
		case err := <-errCh:
			require.NoError(t, err)
		case <-quitAll:
			return
		case <-time.After(3 * time.Second):
			fmt.Println("timeout")
			t.Fatal("not finished in time")
		}
	}

	for i := 0; i < total; i++ {
		go goDkg(i)
	}

	for i := 0; i < newN; i++ {
		<-finished
	}
	close(quitAll)
}

func TestDKGResharingPartial(t *testing.T) {
	slog.Level = slog.LevelDebug
	oldN := 5
	oldT := key.DefaultThreshold(oldN)
	oldPrivs := test.GenerateIDs(oldN)
	oldPubs := test.ListFromPrivates(oldPrivs)
	oldShares, dpub := test.SimulateDKG(t, key.KeyGroup, oldN, oldT)
	oldGroup := key.LoadGroup(oldPubs, &key.DistPublic{Coefficients: dpub}, oldT)

	newN := oldN + 1
	newT := oldT + 1
	// reshare with one old node down and two new nodes
	// first node is only there to reshare but wont be in the new group
	total := oldN + 2
	newDelta := test.GenerateIDs(2)
	newPrivs := make([]*key.Pair, 0, newN)
	// skip the first one
	for _, k := range oldPrivs[1:] {
		newPrivs = append(newPrivs, k)
	}
	// the two new keys are appended at the end
	newPrivs = append(newPrivs, newDelta[0])
	newPrivs = append(newPrivs, newDelta[1])

	require.Equal(t, len(newPrivs), newN)
	newOffset := newN - 2 // offset in newXXX of the new keys
	require.Equal(t, newPrivs[newOffset].Key.String(), newDelta[0].Key.String())
	require.Equal(t, newPrivs[newOffset+1].Key.String(), newDelta[1].Key.String())

	newPubs := test.ListFromPrivates(newPrivs)
	newGroup := key.NewGroup(newPubs, newT)

	nets := testNets(total, false)
	handlers := make([]*Handler, total)
	listeners := make([]net.Listener, total)
	var err error

	// old nodes
	for i := 0; i < oldN; i++ {
		share := key.Share{
			Share:   oldShares[i],
			Commits: dpub,
		}
		conf := &Config{
			Suite:    key.KeyGroup.(Suite),
			Key:      oldPrivs[i],
			OldNodes: oldGroup,
			NewNodes: newGroup,
			Share:    &share,
		}
		handlers[i], err = NewHandler(nets[i], conf, log.DefaultLogger)
		require.NoError(t, err)

		dkgServer := testDKGServer{h: handlers[i]}
		listeners[i] = net.NewTCPGrpcListener(oldPrivs[i].Public.Addr, &dkgServer)
		go listeners[i].Start()
	}
	// new nodes
	for i := oldN; i < total; i++ {
		newIdx := i - oldN + newOffset
		conf := &Config{
			Suite:    key.KeyGroup.(Suite),
			Key:      newPrivs[newIdx],
			NewNodes: newGroup,
			OldNodes: oldGroup,
		}
		handlers[i], err = NewHandler(nets[i], conf, log.DefaultLogger)
		require.NoError(t, err)
		require.True(t, handlers[i].newNode)
		require.False(t, handlers[i].oldNode)
		require.Equal(t, handlers[i].nidx, newIdx)
		dkgServer := testDKGServer{h: handlers[i]}
		listeners[i] = net.NewTCPGrpcListener(newPrivs[newIdx].Public.Addr, &dkgServer)
		go listeners[i].Start()

	}

	defer func() {
		for i := range listeners {
			listeners[i].Stop()
		}
	}()

	finished := make(chan int, total)
	quitAll := make(chan bool)
	goDkg := func(idx int) {
		if idx < oldN {
			go handlers[idx].Start()
		}
		errCh := handlers[idx].WaitError()
		shareCh := handlers[idx].WaitShare()
		exitCh := handlers[idx].WaitExit()
		select {
		case <-shareCh:
			finished <- idx
		case <-exitCh:
			finished <- idx
		case err := <-errCh:
			require.NoError(t, err)
		case <-quitAll:
			return
		case <-time.After(3 * time.Second):
			t.Fatal("not finished in time")
		}
	}

	for i := 0; i < total; i++ {
		go goDkg(i)
	}

	// XXX commented code tries to handle the case where old nodes are excluded
	// from the new group but don't return, while they should.
	//finisheds := make([]int, 0)
	//for i := 0; i < total-1; i++ {
	for i := 0; i < newN; i++ {
		<-finished
		//idx := <-finished
		//finisheds = append(finisheds, idx)
		//fmt.Printf("received finished signal %d/%d:%v\n", i+1, total, finisheds)
		/*if len(finisheds) == 6 {*/
		//fmt.Println("NewN = # responses per deal = ", newN, " => ", handlers[0].state.Certified())
		//fmt.Println(handlers[0].state.QUAL())
		//for ai, ag := range handlers[0].state.OldAggregators() {
		//fmt.Printf("%d: (len %d) : %v\n", ai, len(ag.Responses()), ag.Responses())
		//}
		/*}*/
	}
	close(quitAll)
}

func TestDKGResharingPartial2(t *testing.T) {
	slog.Level = slog.LevelDebug
	oldN := 5
	oldT := key.DefaultThreshold(oldN)
	oldPrivs := test.GenerateIDs(oldN)
	oldPubs := test.ListFromPrivates(oldPrivs)
	oldShares, dpub := test.SimulateDKG(t, key.KeyGroup, oldN, oldT)
	oldGroup := key.LoadGroup(oldPubs, &key.DistPublic{Coefficients: dpub}, oldT)

	newN := oldN + 2
	newT := oldT + 1
	// reshare with one old node down and three new nodes
	// first node is only there to reshare but wont be in the new group
	total := oldN + 3
	newDelta := test.GenerateIDs(3)
	newPrivs := make([]*key.Pair, 0, newN)
	// skip the first one
	for _, k := range oldPrivs[1:] {
		newPrivs = append(newPrivs, k)
	}
	// the new keys are appended at the end
	newPrivs = append(newPrivs, newDelta[0])
	newPrivs = append(newPrivs, newDelta[1])
	newPrivs = append(newPrivs, newDelta[2])

	require.Equal(t, len(newPrivs), newN)
	newOffset := newN - 3 // offset in newXXX of the new keys
	require.Equal(t, newPrivs[newOffset].Key.String(), newDelta[0].Key.String())
	require.Equal(t, newPrivs[newOffset+1].Key.String(), newDelta[1].Key.String())
	require.Equal(t, newPrivs[newOffset+2].Key.String(), newDelta[2].Key.String())

	newPubs := test.ListFromPrivates(newPrivs)
	newGroup := key.NewGroup(newPubs, newT)

	nets := testNets(total, false)
	handlers := make([]*Handler, total)
	listeners := make([]net.Listener, total)
	var err error

	// old nodes
	for i := 0; i < oldN; i++ {
		share := key.Share{
			Share:   oldShares[i],
			Commits: dpub,
		}
		conf := &Config{
			Suite:    key.KeyGroup.(Suite),
			Key:      oldPrivs[i],
			OldNodes: oldGroup,
			NewNodes: newGroup,
			Share:    &share,
		}
		handlers[i], err = NewHandler(nets[i], conf, log.DefaultLogger)
		require.NoError(t, err)

		dkgServer := testDKGServer{h: handlers[i]}
		listeners[i] = net.NewTCPGrpcListener(oldPrivs[i].Public.Addr, &dkgServer)
		go listeners[i].Start()
	}
	// new nodes
	for i := oldN; i < total; i++ {
		newIdx := i - oldN + newOffset
		conf := &Config{
			Suite:    key.KeyGroup.(Suite),
			Key:      newPrivs[newIdx],
			NewNodes: newGroup,
			OldNodes: oldGroup,
		}
		handlers[i], err = NewHandler(nets[i], conf, log.DefaultLogger)
		require.NoError(t, err)
		require.True(t, handlers[i].newNode)
		require.False(t, handlers[i].oldNode)
		require.Equal(t, handlers[i].nidx, newIdx)
		dkgServer := testDKGServer{h: handlers[i]}
		listeners[i] = net.NewTCPGrpcListener(newPrivs[newIdx].Public.Addr, &dkgServer)
		go listeners[i].Start()

	}

	defer func() {
		for i := range listeners {
			listeners[i].Stop()
		}
	}()

	finished := make(chan *key.Pair, total)
	goDkg := func(idx int) {
		if idx < oldN {
			go handlers[idx].Start()
		}
		errCh := handlers[idx].WaitError()
		shareCh := handlers[idx].WaitShare()
		exitCh := handlers[idx].WaitExit()
		kp := handlers[idx].private
		select {
		case <-shareCh:
			finished <- kp
		case <-exitCh:
			finished <- kp
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(3 * time.Second):
			fmt.Println("timeout")
			t.Fatal("not finished in time")
		}
	}

	for i := 0; i < total; i++ {
		go goDkg(i)
	}

	finisheds := make([]*key.Pair, 0)
	//for i := 0; i < total-1; i++ {
	for {
		finisheds = append(finisheds, <-finished)

		var allFound = true
		for _, kp := range newPrivs {
			var found bool
			pub := kp.Public
			for _, kp2 := range finisheds {
				if pub.Equal(kp2.Public) {
					found = true
					break
				}
			}
			if !found {
				allFound = false
				break
			}
		}

		if allFound {
			return
		}
		//idx := <-finished
		//finisheds = append(finisheds, idx)
		//fmt.Printf("received finished signal %d/%d:%v\n", i+1, total, finisheds)
		/*if len(finisheds) == 6 {*/
		//fmt.Println("NewN = # responses per deal = ", newN, " => ", handlers[0].state.Certified())
		//fmt.Println(handlers[0].state.QUAL())
		//for ai, ag := range handlers[0].state.OldAggregators() {
		//fmt.Printf("%d: (len %d) : %v\n", ai, len(ag.Responses()), ag.Responses())
		//}
		/*}*/
	}
}
