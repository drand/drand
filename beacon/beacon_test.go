package beacon

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	dkg_proto "github.com/dedis/drand/protobuf/dkg"
	"github.com/dedis/drand/protobuf/drand"
	"github.com/dedis/drand/test"
	"github.com/dedis/kyber"
	"github.com/dedis/kyber/share"
	"github.com/dedis/kyber/sign/bls"
	"github.com/dedis/kyber/sign/tbls"
	"github.com/dedis/kyber/util/random"
	"github.com/nikkolasg/slog"
	"github.com/stretchr/testify/require"
)

// testService implements a barebone service to be plugged in a net.Gateway
type testService struct {
	*Handler
}

func (t *testService) Public(context.Context, *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	return &drand.PublicRandResponse{}, nil
}
func (t *testService) Private(context.Context, *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	return &drand.PrivateRandResponse{}, nil
}
func (t *testService) Setup(c context.Context, in *dkg_proto.DKGPacket) (*dkg_proto.DKGResponse, error) {
	return &dkg_proto.DKGResponse{}, nil
}

func (t *testService) NewBeacon(c context.Context, in *drand.BeaconRequest) (*drand.BeaconResponse, error) {
	return t.Handler.ProcessBeacon(c, in)
}

func dkgShares(n, t int) ([]*key.Share, kyber.Point) {
	var priPoly *share.PriPoly
	var pubPoly *share.PubPoly
	var err error
	for i := 0; i < n; i++ {
		pri := share.NewPriPoly(key.G2, t, key.G2.Scalar().Pick(random.New()), random.New())
		pub := pri.Commit(key.G2.Point().Base())
		if priPoly == nil {
			priPoly = pri
			pubPoly = pub
			continue
		}
		priPoly, err = priPoly.Add(pri)
		if err != nil {
			panic(err)
		}
		pubPoly, err = pubPoly.Add(pub)
		if err != nil {
			panic(err)
		}
	}
	shares := priPoly.Shares(n)
	secret, err := share.RecoverSecret(key.G2, shares, t, n)
	if err != nil {
		panic(err)
	}
	if !secret.Equal(priPoly.Secret()) {
		panic("secret not equal")
	}
	msg := []byte("Hello world")
	sigs := make([][]byte, n, n)
	_, commits := pubPoly.Info()
	dkgShares := make([]*key.Share, n, n)
	for i := 0; i < n; i++ {
		sigs[i], err = tbls.Sign(key.Pairing, shares[i], msg)
		if err != nil {
			panic(err)
		}
		dkgShares[i] = &key.Share{
			Share:   shares[i],
			Commits: commits,
		}
	}
	sig, err := tbls.Recover(key.Pairing, pubPoly, msg, sigs, t, n)
	if err != nil {
		panic(err)
	}
	if err := bls.Verify(key.Pairing, pubPoly.Commit(), msg, sig); err != nil {
		panic(err)
	}
	//fmt.Println(pubPoly.Commit())
	return dkgShares, pubPoly.Commit()
}

func TestBeacon(t *testing.T) {
	slog.Level = slog.LevelDebug
	n := 5
	thr := 5/2 + 1
	// how many generated beacons should we wait from each beacon handler
	nbBeacons := 3

	tmp := path.Join(os.TempDir(), "drandtest")
	paths := make([]string, n, n)
	for i := 0; i < n; i++ {
		paths[i] = path.Join(tmp, fmt.Sprintf("drand-%d", i))
		require.NoError(t, os.MkdirAll(paths[i], 0755))
	}
	defer func() {
		for i := 0; i < n; i++ {
			os.RemoveAll(paths[i])
		}
	}()

	shares, public := dkgShares(n, thr)
	privs, group := test.BatchIdentities(n)

	listeners := make([]net.Listener, n)
	handlers := make([]*Handler, n)
	receivedChan := make(chan int, nbBeacons*n)

	seed := []byte("Sunshine in a bottle")
	period := time.Duration(400) * time.Millisecond
	// launchBeacon will launch the beacon at the given index. Each time a new
	// beacon is ready from that node, it indicates it by sending the index on
	// the receivedChan channel.
	launchBeacon := func(i int) {
		myCb := func(b *Beacon) {
			err := bls.Verify(key.Pairing, public, Message(b.PreviousRand, b.Round), b.Randomness)
			require.NoError(t, err)
			receivedChan <- i
		}
		store, err := NewBoltStore(paths[i], nil)
		require.NoError(t, err)
		store = NewCallbackStore(store, myCb)
		handlers[i] = NewHandler(net.NewGrpcClient(), privs[i], shares[i], group, store)
		listeners[i] = net.NewTCPGrpcListener(privs[i].Public.Addr, &testService{handlers[i]})
		go listeners[i].Start()
		go handlers[i].Loop(seed, period)
	}

	for i := 0; i < n; i++ {
		launchBeacon(i)
	}

	defer func() {
		for i := 0; i < n; i++ {
			handlers[i].Stop()
			listeners[i].Stop()
		}
	}()

	//expected := nbBeacons * n
	done := make(chan bool)
	// keep track of how many do we have
	go func() {
		receivedIdx := make(map[int]int)
		for {
			receivedIdx[<-receivedChan]++
			var continueRcv = false
			for i := 0; i < n; i++ {
				rcvd := receivedIdx[i]
				if rcvd < nbBeacons {
					continueRcv = true
					break
				}
			}
			if !continueRcv {
				done <- true
				return
			}
		}
	}()

	select {
	case <-done:
		fmt.Println("youpui")
	case <-time.After(period * time.Duration(nbBeacons*2)):
		t.Fatal("not in time")
	}
}
