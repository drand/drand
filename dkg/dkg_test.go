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
	"github.com/dedis/kyber"
	"github.com/dedis/kyber/share"
	sdkg "github.com/dedis/kyber/share/dkg/pedersen"
	"github.com/dedis/kyber/util/random"
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

func (t *testDKGServer) Reshare(c context.Context, in *dkg.DKGPacket) (*dkg.DKGResponse, error) {
	t.h.Process(c, in)
	return &dkg.DKGResponse{}, nil
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
		_, err = t.InternalClient.Reshare(p, d)
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
	nets := testNets(n, true)
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
		select {
		case <-shareCh:
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
	oldShares, dpub := simulateDKG(t, key.G2, oldN, oldT)
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
		shareCh := handlers[idx].WaitShare()
		errCh := handlers[idx].WaitError()
		select {
		case <-shareCh:
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

// returns a list of private shares along with the list of public coefficients
// of the public polynomial
func simulateDKG(test *testing.T, g kyber.Group, n, t int) ([]*share.PriShare, []kyber.Point) {
	// Run an n-fold Pedersen VSS (= DKG)
	priPolys := make([]*share.PriPoly, n)
	priShares := make([][]*share.PriShare, n)
	pubPolys := make([]*share.PubPoly, n)
	pubShares := make([][]*share.PubShare, n)
	for i := 0; i < n; i++ {
		priPolys[i] = share.NewPriPoly(g, t, nil, random.New())
		priShares[i] = priPolys[i].Shares(n)
		pubPolys[i] = priPolys[i].Commit(nil)
		pubShares[i] = pubPolys[i].Shares(n)
	}

	// Verify VSS shares
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			sij := priShares[i][j]
			// s_ij * G
			sijG := g.Point().Base().Mul(sij.V, nil)
			require.True(test, sijG.Equal(pubShares[i][j].V))
		}
	}

	// Create private DKG shares
	dkgShares := make([]*share.PriShare, n)
	for i := 0; i < n; i++ {
		acc := g.Scalar().Zero()
		for j := 0; j < n; j++ { // assuming all participants are in the qualified set
			acc = g.Scalar().Add(acc, priShares[j][i].V)
		}
		dkgShares[i] = &share.PriShare{i, acc}
	}

	// Create public DKG commitments (= verification vector)
	dkgCommits := make([]kyber.Point, t)
	for k := 0; k < t; k++ {
		acc := g.Point().Null()
		for i := 0; i < n; i++ { // assuming all participants are in the qualified set
			_, coeff := pubPolys[i].Info()
			acc = g.Point().Add(acc, coeff[k])
		}
		dkgCommits[k] = acc
	}

	// Check that the private DKG shares verify against the public DKG commits
	dkgPubPoly := share.NewPubPoly(g, nil, dkgCommits)
	for i := 0; i < n; i++ {
		require.True(test, dkgPubPoly.Check(dkgShares[i]))
	}
	return dkgShares, dkgCommits
}

func TestDKGResharingNewNode(t *testing.T) {
	slog.Level = slog.LevelDebug
	oldN := 5
	oldT := key.DefaultThreshold(oldN)
	oldPrivs := test.GenerateIDs(oldN)
	oldPubs := test.ListFromPrivates(oldPrivs)

	oldShares, dpub := simulateDKG(t, key.G2, oldN, oldT)
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
		select {
		case <-shareCh:
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
