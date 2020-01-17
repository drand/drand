package beacon

import (
	"context"
	"fmt"
	"os"
	"path"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/util/random"
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

type BeaconTest struct {
	paths     []string
	n         int
	thr       int
	period    time.Duration
	group     *key.Group
	privates  []*key.Pair
	shares    []*key.Share
	dpublic   kyber.Point
	callbacks []func(*Beacon)
	clock     *clock.Mock
	handlers  []*Handler
	listeners []net.Listener
}

func NewBeaconTest(n, thr int, period time.Duration) *BeaconTest {
	paths := createBoltStores(prefixBeaconTest, n)
	shares, public := dkgShares(n, thr)
	privs, group := test.BatchIdentities(n)
	group.Threshold = thr
	group.Period = period

	return &BeaconTest{
		n:         n,
		thr:       thr,
		period:    period,
		paths:     paths,
		clock:     clock.NewMock(),
		privates:  privs,
		group:     group,
		shares:    shares,
		dpublic:   public,
		callbacks: make([]func(*Beacon), n),
		handlers:  make([]*Handler, n),
		listeners: make([]net.Listener, n),
	}
}

func (b *BeaconTest) CallbackFor(i int, fn func(*Beacon)) {
	b.callbacks[i] = fn
}

func (b *BeaconTest) ServeBeacon(i int) {
	seed := []byte("Sunshine in a bottle")
	store, err := NewBoltStore(b.paths[i], nil)
	if err != nil {
		panic(err)
	}
	if cb := b.callbacks[i]; cb != nil {
		store = NewCallbackStore(store, b.callbacks[i])
	}
	conf := &Config{
		Group:   b.group,
		Private: b.privates[i],
		Share:   b.shares[i],
		Seed:    seed,
		Scheme:  key.Scheme,
		Clock:   b.clock,
	}

	b.handlers[i], err = NewHandler(net.NewGrpcClient(), store, conf, log.DefaultLogger)
	checkErr(err)
	beaconServer := testBeaconServer{h: b.handlers[i]}
	b.listeners[i] = net.NewTCPGrpcListener(b.privates[i].Public.Addr, &beaconServer)
	go b.listeners[i].Start()
	time.Sleep(10 * time.Millisecond)
}

func (b *BeaconTest) StartBeacon(i int, catchup bool) {
	go b.handlers[i].Run(b.period, catchup)
}

func (b *BeaconTest) MovePeriod(p int) {
	for i := 0; i < p; i++ {
		b.clock.Add(b.period)
		// give each handlers time to perform their duty
		time.Sleep(time.Duration(b.n*50) * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
}

func (b *BeaconTest) StopBeacon(i int) {
	if l := b.listeners[i]; l != nil {
		l.Stop()
	}
	if h := b.handlers[i]; h != nil {
		h.Stop()
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

func TestBeaconSimple(t *testing.T) {
	n := 5
	thr := 5/2 + 1
	period := time.Duration(1000) * time.Millisecond

	bt := NewBeaconTest(n, thr, period)
	defer bt.CleanUp()

	var counter uint64
	myCallBack := func(b *Beacon) {
		// verify partial sig
		msg := Message(b.PreviousSig, b.Round)
		err := key.Scheme.VerifyRecovered(bt.dpublic, msg, b.Signature)
		require.NoError(t, err)
		// increase counter
		atomic.AddUint64(&counter, 1)
	}

	for i := 0; i < n; i++ {
		bt.CallbackFor(i, myCallBack)
		// first serve all beacons
		bt.ServeBeacon(i)
	}

	for i := 0; i < n; i++ {
		bt.StartBeacon(i, false)
	}

	// check 1 period
	bt.MovePeriod(1)
	// Travis ...
	time.Sleep(100 * time.Millisecond)
	v := int(atomic.LoadUint64(&counter))
	require.Equal(t, n, v)

	// check 2 period
	bt.MovePeriod(1)
	time.Sleep(100 * time.Millisecond)
	v = int(atomic.LoadUint64(&counter))
	require.Equal(t, n*2, v)
}

func TestBeaconThreshold(t *testing.T) {
	n := 5
	thr := 5/2 + 1
	period := time.Duration(1000) * time.Millisecond

	bt := NewBeaconTest(n, thr, period)
	defer bt.CleanUp()

	var counter uint64
	myCallBack := func(b *Beacon) {
		// verify partial sig
		msg := Message(b.PreviousSig, b.Round)
		err := key.Scheme.VerifyRecovered(bt.dpublic, msg, b.Signature)
		require.NoError(t, err)
		// increase counter
		atomic.AddUint64(&counter, 1)
	}

	for i := 0; i < n; i++ {
		bt.CallbackFor(i, myCallBack)
		// first serve all beacons
		bt.ServeBeacon(i)
	}

	for i := 0; i < n-1; i++ {
		bt.StartBeacon(i, false)
	}

	bt.MovePeriod(1)
	v := int(atomic.LoadUint64(&counter))
	require.Equal(t, n-1, v)

	bt.StartBeacon(n-1, true)

	bt.MovePeriod(1)
	v = int(atomic.LoadUint64(&counter))
	require.Equal(t, n*2-1, v)
}
