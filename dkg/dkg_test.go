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
	sdkg "github.com/dedis/kyber/share/dkg/pedersen"
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

func TestDKGFresh(t *testing.T) {
	slog.Level = slog.LevelDebug
	n := 5
	thr := key.DefaultThreshold(n)
	privs := test.GenerateIDs(n)
	pubs := test.ListFromPrivates(privs)
<<<<<<< HEAD
	nets := testNets(n, true)
=======
	nets := testNets(n)
	conf := &Config{
		Suite: key.G2.(sdkg.Suite),
		Group: key.NewGroup(pubs, thr, &key.DistPublic{}),
	}
	conf.Group.Threshold = thr
>>>>>>> interface
	handlers := make([]*Handler, n, n)
	listeners := make([]net.Listener, n, n)
	var err error

	group := key.NewGroup(pubs, thr)
	for i := 0; i < n; i++ {
		conf := &Config{
			Suite:     key.G2.(Suite),
			Key:       privs[i],
			NewNodes:  group,
			Threshold: thr,
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

func TestDKGResharingPartial(t *testing.T) {
	slog.Level = slog.LevelDebug
	oldN := 5
	oldT := key.DefaultThreshold(oldN)
	oldPrivs := test.GenerateIDs(oldN)
	oldPubs := test.ListFromPrivates(oldPrivs)
	oldShares, dpub := test.SimulateDKG(t, key.G2, oldN, oldT)
	oldGroup := key.LoadGroup(oldPubs, &key.DistPublic{dpub}, oldT)

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
			Suite:     key.G2.(Suite),
			Key:       oldPrivs[i],
			OldNodes:  oldGroup,
			NewNodes:  newGroup,
			Share:     &share,
			Threshold: newT,
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
		dkgConf := sdkg.NewReshareConfig(key.G2.(sdkg.Suite),
			newPrivs[newIdx].Key,
			oldGroup.Points(),
			newGroup.Points(),
			nil,
			dpub)
		dkgConf.Threshold = newT
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
			fmt.Println("timeout")
			t.Fatal("not finished in time")
		}
	}

	for i := 0; i < total; i++ {
		go goDkg(i)
	}

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
	oldGroup := key.LoadGroup(oldPubs, &key.DistPublic{dpub}, oldT)

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
			Suite:     key.G2.(Suite),
			Key:       oldPrivs[i],
			OldNodes:  oldGroup,
			NewNodes:  newGroup,
			Share:     &share,
			Threshold: newT,
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
			Suite:     key.G2.(Suite),
			Key:       newPrivs[newIdx],
			NewNodes:  newGroup,
			OldNodes:  oldGroup,
			Threshold: newT,
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
