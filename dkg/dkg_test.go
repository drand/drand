package dkg

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	"github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/test"
	"github.com/nikkolasg/slog"
	"github.com/stretchr/testify/require"
)

// testDKGServer implements a barebone service to be plugged in a net.DefaultService
type testDKGServer struct {
	h *Handler
}

func (t *testDKGServer) Setup(c context.Context, in *dkg.DKGPacket) (*dkg.DKGResponse, error) {
	t.h.Process(c, in)
	return &dkg.DKGResponse{}, nil
}

func (t *testDKGServer) Reshare(c context.Context, in *dkg.ResharePacket) (*dkg.ReshareResponse, error) {
	t.h.Process(c, in.Packet)
	return &dkg.ReshareResponse{}, nil
}

// testNet implements the network interface that the dkg Handler expects
type testNet struct {
	fresh bool
	net.InternalClient
}

func (t *testNet) Send(p net.Peer, d *dkg.DKGPacket) error {
	var err error
	if t.fresh {
		_, err = t.InternalClient.Setup(p, d)
	} else {
		_, err = t.InternalClient.Reshare(p, &dkg.ResharePacket{Packet: d})
	}
	return err
}

func testNets(n int, fresh bool) []*testNet {
	nets := make([]*testNet, n, n)
	for i := 0; i < n; i++ {
		nets[i] = &testNet{fresh: fresh, InternalClient: net.NewGrpcClient()}
	}
	return nets
}

func TestDKGWithTimeout(t *testing.T) {
	slog.Level = slog.LevelDebug
	n := 7
	thr := key.DefaultThreshold(n)
	timeout := 500 * time.Millisecond
	privs := test.GenerateIDs(n)
	pubs := test.ListFromPrivates(privs)
	nets := testNets(n, true)
	handlers := make([]*Handler, n, n)
	listeners := make([]net.Listener, n, n)
	var err error
	group := key.NewGroup(pubs, thr)

	offline := n - thr
	alive := n - offline
	for i := 0; i < alive; i++ {
		conf := &Config{
			Suite:    key.G2.(Suite),
			Key:      privs[i],
			NewNodes: group,
			Timeout:  timeout,
		}

		handlers[i], err = NewHandler(nets[i], conf)
		require.NoError(t, err)
		dkgServer := testDKGServer{h: handlers[i]}
		listeners[i] = net.NewTCPGrpcListener(privs[i].Public.Addr, &net.DefaultService{D: &dkgServer})
		go listeners[i].Start()
	}
	defer func() {
		for i := 0; i < alive; i++ {
			listeners[i].Stop()
		}
	}()

	finished := make(chan int, n)
	goDkg := func(idx int) {
		if idx == 0 {
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
		case <-time.After(3 * time.Second):
			fmt.Println("timeout")
			t.Fatal("not finished in time")
		}
	}

	for i := 0; i < alive; i++ {
		go goDkg(i)
	}

	for i := 0; i < alive; i++ {
		select {
		case <-finished:
			continue
		case <-time.After(2 * time.Second):
		}
	}
}

func TestDKGFresh(t *testing.T) {
	n := 5
	slog.Level = slog.LevelDebug
	thr := key.DefaultThreshold(n)
	privs := test.GenerateIDs(n)
	pubs := test.ListFromPrivates(privs)
	nets := testNets(n, true)
	handlers := make([]*Handler, n, n)
	listeners := make([]net.Listener, n, n)
	var err error

	group := key.NewGroup(pubs, thr)
	for i := 0; i < n; i++ {
		conf := &Config{
			Suite:    key.G2.(Suite),
			Key:      privs[i],
			NewNodes: group,
		}

		handlers[i], err = NewHandler(nets[i], conf)
		require.NoError(t, err)
		dkgServer := testDKGServer{h: handlers[i]}
		listeners[i] = net.NewTCPGrpcListener(privs[i].Public.Addr, &net.DefaultService{D: &dkgServer})
		go listeners[i].Start()
	}
	defer func() {
		for i := 0; i < n; i++ {
			listeners[i].Stop()
		}
	}()

	finished := make(chan int, n)
	goDkg := func(idx int) {
		if idx == 0 {
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
		case <-time.After(3 * time.Second):
			fmt.Println("timeout")
			t.Fatal("not finished in time")
		}
	}

	for i := 0; i < n; i++ {
		go goDkg(i)
	}
	for i := 0; i < n; i++ {
		<-finished
	}
}

func TestDKGResharingPartialWithTimeout(t *testing.T) {
	slog.Level = slog.LevelDebug
	oldN := 7
	oldT := key.DefaultThreshold(oldN)
	oldPrivs := test.GenerateIDs(oldN)
	oldPubs := test.ListFromPrivates(oldPrivs)
	oldShares, dpub := test.SimulateDKG(t, key.G2, oldN, oldT)
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
			Suite:    key.G2.(Suite),
			Key:      oldPrivs[i],
			OldNodes: oldGroup,
			NewNodes: newGroup,
			Share:    &share,
			Timeout:  timeout,
		}
		handlers[i], err = NewHandler(nets[i], conf)
		require.NoError(t, err)

		dkgServer := testDKGServer{h: handlers[i]}
		listeners[i] = net.NewTCPGrpcListener(oldPrivs[i].Public.Addr, &net.DefaultService{D: &dkgServer})
		go listeners[i].Start()
	}
	// new nodes
	for i := oldN; i < total; i++ {
		newIdx := i - oldN + newOffset
		conf := &Config{
			Suite:    key.G2.(Suite),
			Key:      newPrivs[newIdx],
			NewNodes: newGroup,
			OldNodes: oldGroup,
			Timeout:  timeout,
		}
		handlers[i], err = NewHandler(nets[i], conf)
		require.NoError(t, err)
		require.True(t, handlers[i].newNode)
		require.False(t, handlers[i].oldNode)
		require.Equal(t, handlers[i].nidx, newIdx)
		dkgServer := testDKGServer{h: handlers[i]}
		listeners[i] = net.NewTCPGrpcListener(newPrivs[newIdx].Public.Addr, &net.DefaultService{D: &dkgServer})
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
	oldShares, dpub := test.SimulateDKG(t, key.G2, oldN, oldT)
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
			Suite:    key.G2.(Suite),
			Key:      oldPrivs[i],
			OldNodes: oldGroup,
			NewNodes: newGroup,
			Share:    &share,
		}
		handlers[i], err = NewHandler(nets[i], conf)
		require.NoError(t, err)

		dkgServer := testDKGServer{h: handlers[i]}
		listeners[i] = net.NewTCPGrpcListener(oldPrivs[i].Public.Addr, &net.DefaultService{D: &dkgServer})
		go listeners[i].Start()
	}
	// new nodes
	for i := oldN; i < total; i++ {
		newIdx := i - oldN + newOffset
		conf := &Config{
			Suite:    key.G2.(Suite),
			Key:      newPrivs[newIdx],
			NewNodes: newGroup,
			OldNodes: oldGroup,
		}
		handlers[i], err = NewHandler(nets[i], conf)
		require.NoError(t, err)
		require.True(t, handlers[i].newNode)
		require.False(t, handlers[i].oldNode)
		require.Equal(t, handlers[i].nidx, newIdx)
		dkgServer := testDKGServer{h: handlers[i]}
		listeners[i] = net.NewTCPGrpcListener(newPrivs[newIdx].Public.Addr, &net.DefaultService{D: &dkgServer})
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
}

func TestDKGResharingNewNode(t *testing.T) {
	slog.Level = slog.LevelDebug
	oldN := 5
	oldT := key.DefaultThreshold(oldN)
	oldPrivs := test.GenerateIDs(oldN)
	oldPubs := test.ListFromPrivates(oldPrivs)

	oldShares, dpub := test.SimulateDKG(t, key.G2, oldN, oldT)
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
			Suite:    key.G2.(Suite),
			Key:      oldPrivs[i],
			OldNodes: oldGroup,
			NewNodes: newGroup,
			Share:    &share,
		}
		handlers[i], err = NewHandler(nets[i], conf)
		require.NoError(t, err)

		dkgServer := testDKGServer{h: handlers[i]}
		listeners[i] = net.NewTCPGrpcListener(oldPrivs[i].Public.Addr, &net.DefaultService{D: &dkgServer})
		go listeners[i].Start()
	}
	// new nodes
	for i := oldN; i < total; i++ {
		newIdx := i - oldN
		conf := &Config{
			Suite:    key.G2.(Suite),
			Key:      newPrivs[newIdx],
			NewNodes: newGroup,
			OldNodes: oldGroup,
		}

		handlers[i], err = NewHandler(nets[i], conf)
		require.NoError(t, err)
		require.True(t, handlers[i].newNode)
		require.False(t, handlers[i].oldNode)
		require.Equal(t, handlers[i].nidx, newIdx)
		dkgServer := testDKGServer{h: handlers[i]}
		listeners[i] = net.NewTCPGrpcListener(newPrivs[newIdx].Public.Addr, &net.DefaultService{D: &dkgServer})
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
}

func TestDKGResharingPartial2(t *testing.T) {
	slog.Level = slog.LevelDebug
	oldN := 5
	oldT := key.DefaultThreshold(oldN)
	oldPrivs := test.GenerateIDs(oldN)
	oldPubs := test.ListFromPrivates(oldPrivs)
	oldShares, dpub := test.SimulateDKG(t, key.G2, oldN, oldT)
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
			Suite:    key.G2.(Suite),
			Key:      oldPrivs[i],
			OldNodes: oldGroup,
			NewNodes: newGroup,
			Share:    &share,
		}
		handlers[i], err = NewHandler(nets[i], conf)
		require.NoError(t, err)

		dkgServer := testDKGServer{h: handlers[i]}
		listeners[i] = net.NewTCPGrpcListener(oldPrivs[i].Public.Addr, &net.DefaultService{D: &dkgServer})
		go listeners[i].Start()
	}
	// new nodes
	for i := oldN; i < total; i++ {
		newIdx := i - oldN + newOffset
		conf := &Config{
			Suite:    key.G2.(Suite),
			Key:      newPrivs[newIdx],
			NewNodes: newGroup,
			OldNodes: oldGroup,
		}
		handlers[i], err = NewHandler(nets[i], conf)
		require.NoError(t, err)
		require.True(t, handlers[i].newNode)
		require.False(t, handlers[i].oldNode)
		require.Equal(t, handlers[i].nidx, newIdx)
		dkgServer := testDKGServer{h: handlers[i]}
		listeners[i] = net.NewTCPGrpcListener(newPrivs[newIdx].Public.Addr, &net.DefaultService{D: &dkgServer})
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
			fmt.Println(" YOUOUUOUOUOUOUOU")
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
