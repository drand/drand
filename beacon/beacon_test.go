package beacon

import (
	"context"
	"fmt"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	//"github.com/benbjohnson/clock"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/util/random"
	clock "github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
)

// TODO make beacon tests not dependant on key.Scheme

// testBeaconServer implements a barebone service to be plugged in a net.DefaultService
type testBeaconServer struct {
	*net.EmptyServer
	h *Handler
}

func (t *testBeaconServer) NewBeacon(c context.Context, in *drand.BeaconRequest) (*drand.BeaconResponse, error) {
	return t.h.ProcessBeacon(c, in)
}

func (t *testBeaconServer) SyncChain(req *drand.SyncRequest, p drand.Protocol_SyncChainServer) error {
	return t.h.SyncChain(req, p)
}

func dkgShares(n, t int) ([]*key.Share, kyber.Point) {
	var priPoly *share.PriPoly
	var pubPoly *share.PubPoly
	var err error
	for i := 0; i < n; i++ {
		pri := share.NewPriPoly(key.KeyGroup, t, key.KeyGroup.Scalar().Pick(random.New()), random.New())
		pub := pri.Commit(key.KeyGroup.Point().Base())
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
	secret, err := share.RecoverSecret(key.KeyGroup, shares, t, n)
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
		sigs[i], err = key.Scheme.Sign(shares[i], msg)
		if err != nil {
			panic(err)
		}
		dkgShares[i] = &key.Share{
			Share:   shares[i],
			Commits: commits,
		}
	}
	sig, err := key.Scheme.Recover(pubPoly, msg, sigs, t, n)
	if err != nil {
		panic(err)
	}
	if err := key.Scheme.VerifyRecovered(pubPoly.Commit(), msg, sig); err != nil {
		panic(err)
	}
	//fmt.Println(pubPoly.Commit())
	return dkgShares, pubPoly.Commit()
}

const prefixBeaconTest = "beaconTest"

type node struct {
	private  *key.Pair
	shares   *key.Share
	callback func(*Beacon)
	handler  *Handler
	listener net.Listener
	clock    clock.FakeClock
	started  bool
}

type BeaconTest struct {
	paths   []string
	n       int
	thr     int
	genesis int64
	shares  []*key.Share
	period  time.Duration
	group   *key.Group
	privs   []*key.Pair
	dpublic kyber.Point
	nodes   map[int]*node
	time    clock.FakeClock
}

func NewBeaconTest(n, thr int, period time.Duration, genesisTime int64) *BeaconTest {
	paths := createBoltStores(prefixBeaconTest, n)
	shares, public := dkgShares(n, thr)
	privs, group := test.BatchIdentities(n)
	group.Threshold = thr
	group.Period = period
	group.GenesisTime = genesisTime

	bt := &BeaconTest{
		n:       n,
		privs:   privs,
		thr:     thr,
		period:  period,
		paths:   paths,
		shares:  shares,
		group:   group,
		dpublic: public,
		nodes:   make(map[int]*node),
		time:    clock.NewFakeClock(),
	}

	for i := 0; i < n; i++ {
		bt.CreateNode(i)
	}
	return bt
}

func (b *BeaconTest) CreateNode(i int) {
	for _, p := range b.privs {
		idx, _ := b.group.Index(p.Public)
		if idx != i {
			continue
		}
		node := &node{}
		if n, ok := b.nodes[idx]; ok {
			node = n
		}
		store, err := NewBoltStore(b.paths[idx], nil)
		if err != nil {
			panic(err)
		}
		store = NewCallbackStore(store, func(b *Beacon) {
			if node.callback != nil {
				node.callback(b)
			}
		})
		node.clock = clock.NewFakeClock()
		node.clock.Advance(b.time.Since(node.clock.Now()))
		conf := &Config{
			Group:   b.group,
			Private: p,
			Share:   b.shares[idx],
			Scheme:  key.Scheme,
			Clock:   node.clock,
		}

		node.handler, err = NewHandler(net.NewGrpcClient(), store, conf, log.NewLogger(log.LogDebug))
		checkErr(err)
		beaconServer := testBeaconServer{h: node.handler}
		node.listener = net.NewTCPGrpcListener(p.Public.Addr, &beaconServer)
		b.nodes[idx] = node
	}
}

func (b *BeaconTest) CallbackFor(i int, fn func(*Beacon)) {
	b.nodes[i].callback = fn
}

func (b *BeaconTest) ServeBeacon(i int) {
	go b.nodes[i].listener.Start()
}

func (b *BeaconTest) StartBeacons(n int) {
	for i := 0; i < n; i++ {
		b.StartBeacon(i, false)
	}
	// give time for go routines to kick off
	time.Sleep(100 * time.Millisecond)
}
func (b *BeaconTest) StartBeacon(i int, catchup bool) {
	b.nodes[i].started = true
	if catchup {
		go b.nodes[i].handler.Catchup()
	} else {
		go b.nodes[i].handler.Start()
	}
}

func (b *BeaconTest) MoveTime(t time.Duration) {
	for i, n := range b.nodes {
		before := n.clock.Now().Unix()
		n.clock.Advance(t)
		fmt.Printf(" - %d increasing time of node %d - %p - before: %d - current: %d\n", time.Now().Unix(), i, n.clock, before, n.clock.Now().Unix())
	}
	b.time.Advance(t)
	// give each handlers time to perform their duty
	time.Sleep(time.Duration(b.n*100) * time.Millisecond)
	//time.Sleep(100 * time.Millisecond)
}

func (b *BeaconTest) StopBeacon(i int) {
	if n, ok := b.nodes[i]; ok {
		n.listener.Stop()
		n.handler.Stop()
		n.started = false
	}
}

func (b *BeaconTest) StopAll() {
	for i := 0; i < b.n; i++ {
		b.StopBeacon(i)
	}
}

func (b *BeaconTest) CleanUp() {
	deleteBoltStores(prefixBeaconTest)
	b.StopAll()
}

func checkErr(e error) {
	if e != nil {
		panic(e)
	}
}

func createBoltStores(prefix string, n int) []string {
	paths := make([]string, n, n)
	for i := 0; i < n; i++ {
		paths[i] = path.Join(prefix, fmt.Sprintf("drand-%d", i))
		if err := os.MkdirAll(paths[i], 0755); err != nil {
			panic(err)
		}
	}
	return paths
}

func deleteBoltStores(prefix string) {
	os.RemoveAll(prefix)
}

func checkWait(counter *sync.WaitGroup) {
	var doneCh = make(chan bool, 1)
	go func() {
		counter.Wait()
		doneCh <- true
	}()
	select {
	case <-doneCh:
		break
	case <-time.After(1 * time.Second):
		panic("outdated beacon time")
	}
}
func TestBeaconSimple(t *testing.T) {
	n := 3
	thr := n/2 + 1
	period := 2 * time.Second

	//var genesisTime int64 = clock.NewMock().Now().Unix() + 2
	var genesisTime int64 = clock.NewFakeClock().Now().Unix() + 2

	bt := NewBeaconTest(n, thr, period, genesisTime)
	defer bt.CleanUp()

	var counter = &sync.WaitGroup{}
	counter.Add(n)
	myCallBack := func(b *Beacon) {
		// verify partial sig
		msg := Message(b.PreviousSig, b.PreviousRound, b.Round)
		err := key.Scheme.VerifyRecovered(bt.dpublic, msg, b.Signature)
		require.NoError(t, err)
		counter.Done()
	}

	for i := 0; i < n; i++ {
		bt.CallbackFor(i, myCallBack)
		// first serve all beacons
		bt.ServeBeacon(i)
	}

	bt.StartBeacons(n)
	// move clock before genesis time
	bt.MoveTime(1 * time.Second)
	for i := 0; i < n; i++ {
		bt.nodes[i].handler.Lock()
		started := bt.nodes[i].handler.started
		bt.nodes[i].handler.Unlock()
		require.False(t, started, "handler %d has started?", i)
		//fmt.Printf(" + before genesis - node %d has clock time %d\n", bt.handlers[i].index, bt.handlers[i].conf.Clock.Now().Unix())
	}
	//fmt.Println(" --------- moving to genesis ---------------")
	// move clock to genesis time
	bt.MoveTime(1 * time.Second)
	//time.Sleep(100 * time.Millisecond)
	for i := 0; i < n; i++ {
		bt.nodes[i].handler.Lock()
		started := bt.nodes[i].handler.started
		bt.nodes[i].handler.Unlock()
		require.True(t, started, "handler %d hasnot started", i)
		//fmt.Printf(" + after genesis - node %d has clock time %d\n", bt.handlers[i].index, bt.handlers[i].conf.Clock.Now().Unix())
	}

	// check 1 period
	checkWait(counter)
	// check 2 period
	counter.Add(n)
	bt.MoveTime(2 * time.Second)
	checkWait(counter)
}

func TestBeaconThreshold(t *testing.T) {
	n := 3
	thr := n/2 + 1
	period := 2 * time.Second

	//var genesisTime int64 = clock.NewMock().Now().Unix() + 2
	var genesisTime int64 = clock.NewFakeClock().Now().Unix() + 2

	bt := NewBeaconTest(n, thr, period, genesisTime)
	defer bt.CleanUp()
	var currentRound uint64 = 0
	var counter = &sync.WaitGroup{}
	myCallBack := func(i int) func(*Beacon) {
		return func(b *Beacon) {
			fmt.Printf(" - test: callback called for node %d - round %d\n", i, b.Round)
			// verify partial sig
			msg := Message(b.PreviousSig, b.PreviousRound, b.Round)
			err := key.Scheme.VerifyRecovered(bt.dpublic, msg, b.Signature)
			require.NoError(t, err)
			// callbacks are called for syncing up as well so we only decrease
			// waitgroup when it's the current round
			if b.Round == currentRound {
				counter.Done()
			}
		}
	}

	makeRounds := func(r int, howMany int) {
		func() {
			for i := 0; i < r; i++ {
				counter.Add(howMany)
				bt.MoveTime(period)
				checkWait(counter)
				time.Sleep(100 * time.Millisecond)
				currentRound++
			}
		}()

	}
	// open connections for all but one
	for i := 0; i < n-1; i++ {
		bt.CallbackFor(i, myCallBack(i))
		bt.ServeBeacon(i)
	}

	// start all but one
	bt.StartBeacons(n - 1)

	currentRound++
	// make a few round
	makeRounds(2, n-1)

	// launch the last one
	bt.CallbackFor(n-1, myCallBack(n-1))
	bt.ServeBeacon(n - 1)
	bt.StartBeacon(n-1, true)
	// wait a bit for syncing to take place
	time.Sleep(100 * time.Millisecond)
	makeRounds(2, n)

	// stop last one again - so it will force a sync not from genesis
	bt.StopBeacon(n - 1)
	// make a few round
	makeRounds(2, n-1)
	// start the node again
	bt.CreateNode(n - 1)
	bt.ServeBeacon(n - 1)
	bt.StartBeacon(n-1, true)
	// let time for syncing
	time.Sleep(100 * time.Millisecond)
	// expect lastnode to have catch up
	makeRounds(3, n)
}
