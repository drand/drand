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
		nets[i].SetTimeout(5 * time.Second)
	}
	return nets
}

type DKGTest struct {
	n         int
	thr       int
	clock     *clock.Mock
	timeout   time.Duration
	group     *key.Group
	privates  []*key.Pair
	publics   []*key.Identity
	handlers  []*Handler
	listeners []net.Listener
	nets      []*testNet
	callbacks []func(*Handler)
}

func NewDKGTest(n, thr int, timeout time.Duration) *DKGTest {
	privs := test.GenerateIDs(n)
	pubs := test.ListFromPrivates(privs)
	group := key.NewGroup(pubs, thr)
	return &DKGTest{
		n:         n,
		thr:       thr,
		timeout:   timeout,
		group:     group,
		privates:  privs,
		publics:   pubs,
		nets:      testNets(n, true),
		handlers:  make([]*Handler, n),
		listeners: make([]net.Listener, n),
		callbacks: make([]func(*Handler), n),
		clock:     clock.NewMock(),
	}
}

func (d *DKGTest) ServeDKG(i int) {
	conf := &Config{
		Suite:    key.KeyGroup.(Suite),
		Key:      d.privates[i],
		NewNodes: d.group,
		Timeout:  d.timeout,
		Clock:    d.clock,
	}
	var err error
	d.handlers[i], err = NewHandler(d.nets[i], conf, log.DefaultLogger)
	checkErr(err)
	dkgServer := testDKGServer{h: d.handlers[i]}
	d.listeners[i] = net.NewTCPGrpcListener(d.privates[i].Public.Addr, &dkgServer)
	go d.listeners[i].Start()
	if cb := d.callbacks[i]; cb != nil {
		go cb(d.handlers[i])
	}
	time.Sleep(10 * time.Millisecond)
}

func (d *DKGTest) CallbackFor(i int, fn func(s *Share, e error, exit bool)) {
	d.callbacks[i] = func(h *Handler) {
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

func (d *DKGTest) WaitFinishFor(i int) {
	if d.handlers[i] == nil {
		panic("not supposed ot happen")
	}
	<-d.handlers[i].WaitShare()
}
func (d *DKGTest) WaitFinish(min int) []int {
	doneCh := make(chan int, d.n)
	for i := 0; i < d.n; i++ {
		go func(i int) {
			h := d.handlers[i]
			if h == nil {
				return
			}
			shareCh := h.WaitShare()
			errCh := h.WaitError()
			exitCh := h.WaitExit()
			select {
			case <-shareCh:
				doneCh <- i
			case <-exitCh:
			case err := <-errCh:
				checkErr(err)
			}
		}(i)
	}
	var indexes []int
	for {
		indexes = append(indexes, <-doneCh)
		//fmt.Printf(" \n\nDKG %d FINISHED \n\n", <-doneCh)
		if len(indexes) == min {
			return indexes
		}
	}
}

func (d *DKGTest) CheckIncludedQUALFrom(from, dkg int) bool {
	h := d.handlers[from]
	if h == nil {
		panic("that should not happen")
	}
	group := h.QualifiedGroup()
	pub := d.publics[dkg]
	return group.Contains(pub)
}

func (d *DKGTest) CheckIncludedQUAL(indexes []int) bool {
	for idx := range indexes {
		for idx2 := range indexes {
			if !d.CheckIncludedQUALFrom(idx, idx2) {
				return false
			}
		}
	}
	return true
}

func (d *DKGTest) StartDKG(i int) {
	go d.handlers[i].Start()
	time.Sleep(5 * time.Millisecond)
}

func (d *DKGTest) StopDKG(i int) {
	if l := d.listeners[i]; l != nil {
		l.Stop()
	}
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
	dt := NewDKGTest(n, thr, timeout)

	for i := 0; i < n; i++ {
		dt.ServeDKG(i)
	}

	dt.StartDKG(0)
	indexes := dt.WaitFinish(n)
	require.True(t, dt.CheckIncludedQUAL(indexes))
}

func TestDKGWithTimeout2(t *testing.T) {
	n := 7
	thr := key.DefaultThreshold(n)
	timeout := 1 * time.Second
	offline := n - thr
	alive := n - offline
	dt := NewDKGTest(n, thr, timeout)
	for i := 0; i < alive; i++ {
		dt.ServeDKG(i)
	}
	dt.StartDKG(0)
	// wait for all messages to come back and forth but less than timeout
	time.Sleep(500 * time.Millisecond)
	// trigger timeout immediatly
	dt.MoveTime(timeout * 2)
	indexes := dt.WaitFinish(alive)
	require.True(t, dt.CheckIncludedQUAL(indexes))
}

func TestDKGResharingPartialWithTimeout(t *testing.T) {
	slog.Level = slog.LevelDebug
	oldN := 7
	oldT := key.DefaultThreshold(oldN)
	oldPrivs := test.GenerateIDs(oldN)
	oldPubs := test.ListFromPrivates(oldPrivs)
	oldShares, dpub := test.SimulateDKG(t, key.KeyGroup, oldN, oldT)
	oldGroup := key.LoadGroup(oldPubs, &key.DistPublic{Coefficients: dpub}, oldT)

	timeout := 500 * time.Millisecond

	// reshare with one old node down and two new nodes = |card_old_group| + 1
	// first node is only there to reshare but wont be in the new group
	newN := oldN + 1
	newT := oldT + 1
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

	require.Equal(t, newN, len(newPrivs))
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
			Timeout:  timeout,
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
			Timeout:  timeout,
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
		case <-time.After(3 * time.Second):
			fmt.Println("timeout in the test")
			require.True(t, false)
		}
	}

	// the first dealer does not start - it should succeed but only *after* the
	// timeout has occured. Thus we start a timeout ourselves, and check that we
	// don't have any responses before the timeout occurs
	timeoutDone := make(chan bool, 1)
	go func() {
		time.Sleep(timeout)
		timeoutDone <- true
	}()
	time.Sleep(100 * time.Millisecond)
	// the index 0 is the dealer that is out of the new group, so let's say he
	// does not participate. Then we can add dealers that are in the new group.
	// These should not have a valid share at the end
	offlineInNewGroup := 1
	for i := 1 + offlineInNewGroup; i < total; i++ {
		go goDkg(i)
	}

	// XXX for nodes that don't participate in the new group, i.e. old nodes
	// quitting the group, they still dont know when the protocol finished ->
	// need some love
	finisheds := make([]int, 0)
	var timeoutBefore bool
	for len(finisheds) < (newN - offlineInNewGroup) {
		select {
		case idx := <-finished:
			require.True(t, timeoutBefore)
			if newGroup.Contains(handlers[idx].private.Public) {
				// only look at the expected ones, the share holders
				finisheds = append(finisheds, idx)
			}
		case <-timeoutDone:
			timeoutBefore = true
		}

	}
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
