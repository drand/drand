package core

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	gnet "net"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/BurntSushi/toml"
	"github.com/benbjohnson/clock"
	"github.com/drand/drand/beacon"
	"github.com/drand/drand/entropy"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"
	"github.com/drand/kyber"
	"github.com/kabukky/httpscerts"
	"github.com/stretchr/testify/require"
)

func getSleepDuration() time.Duration {
	if os.Getenv("TRAVIS_BRANCH") != "" {
		return time.Duration(1500) * time.Millisecond
	}
	return time.Duration(300) * time.Millisecond
}

func TestDrandDKGReshareTimeout(t *testing.T) {
	oldN := 5 // 4 / 5
	newN := 6 // 5 / 6
	oldThr := key.DefaultThreshold(oldN)
	newThr := key.DefaultThreshold(newN)
	timeoutStr := "200ms"
	timeout, _ := time.ParseDuration(timeoutStr)
	period := 300 * time.Millisecond
	offline := 1 // can't do more anyway with a 2/3 + 1 threshold

	dt := NewDrandTest(t, oldN, oldThr, period)
	defer dt.Cleanup()
	dt.RunDKG()
	pubShare := func(s *key.Share) kyber.Point {
		return key.KeyGroup.Point().Mul(s.Share.V, nil)
	}
	for _, drand := range dt.drands {
		pk := drand.priv.Public
		idx, ok := dt.group.Index(pk)
		require.True(t, ok)
		fmt.Printf("idx: %d : pubkey %s\n\t - pub share: %s\n\n", idx, pk.Key.String(), pubShare(drand.share).String())
	}

	dt.SetupReshare(oldN-offline, newN-oldN, newThr)

	fmt.Println("SETUP RESHARE DONE")
	// run the resharing
	var doneReshare = make(chan bool, 1)
	go func() {
		dt.RunReshare(oldN-offline, newN-oldN, timeoutStr)
		doneReshare <- true
	}()
	checkDone := func() bool {
		select {
		case <-doneReshare:
			return true
		default:
			return false
		}
	}
	// check it is not done yet
	time.Sleep(getSleepDuration())
	require.False(t, checkDone())

	// advance time to the timeout
	dt.MoveTime(timeout)
	// give time to finish for the go routines and such
	time.Sleep(getSleepDuration())
	require.True(t, checkDone())
}

type SyncClock struct {
	*clock.Mock
	*sync.Mutex
}

func (s *SyncClock) Add(d time.Duration) {
	//s.Lock()
	//defer s.Unlock()
	s.Mock.Add(d)
}

type DrandTest struct {
	t            *testing.T
	n            int
	thr          int
	dir          string
	newDir       string
	drands       map[string]*Drand
	newDrands    map[string]*Drand
	group        *key.Group
	newGroup     *key.Group
	groupPath    string
	newGroupPath string
	period       time.Duration
	ids          []string
	newIds       []string
	shares       map[string]*key.Share
	clocks       map[string]*SyncClock
	certPaths    []string
	newCertPaths []string
}

func (d *DrandTest) Cleanup() {
	os.RemoveAll(d.dir)
	os.RemoveAll(d.newDir)
}

func (d *DrandTest) GetBeacon(id string, round int) (*beacon.Beacon, error) {
	dd, ok := d.drands[id]
	require.True(d.t, ok)

	return dd.beaconStore.Get(uint64(round))
}

// returns new ids generated
func (d *DrandTest) SetupReshare(keepOld, addNew, newThr int) []string {
	newN := keepOld + addNew
	ids := make([]*key.Identity, 0, newN)
	newAddr := make([]string, addNew)
	newDrands, _, newDir, newCertPaths := BatchNewDrand(addNew, false,
		WithCallOption(grpc.FailFast(true)),
	)
	d.newDir = newDir
	d.newDrands = make(map[string]*Drand)
	// add old participants
	for _, id := range d.ids[:keepOld] {
		drand := d.drands[id]
		ids = append(ids, drand.priv.Public)
		for _, cp := range newCertPaths {
			drand.opts.certmanager.Add(cp)
		}

	}
	// add new participants
	for i, drand := range newDrands {
		ids = append(ids, drand.priv.Public)
		newAddr[i] = drand.priv.Public.Address()
		d.newDrands[drand.priv.Public.Address()] = drand
		d.setClock(newAddr[i])
		for _, cp := range d.certPaths {
			drand.opts.certmanager.Add(cp)
		}
	}

	d.newIds = newAddr

	//

	shuffledIds := make([]*key.Identity, len(ids))
	copy(shuffledIds, ids)
	// shuffle with random swaps
	for i := 0; i < len(ids)*3; i++ {
		i1 := rand.Intn(len(ids))
		i2 := rand.Intn(len(ids))
		shuffledIds[i1], shuffledIds[i2] = shuffledIds[i2], shuffledIds[i1]
	}

	d.newGroup = key.NewGroup(shuffledIds, newThr)
	d.newGroup.Period = d.period
	fmt.Println("RESHARE GROUP:\n", d.newGroup.String())
	d.newGroupPath = path.Join(newDir, "newgroup.toml")
	require.NoError(d.t, key.Save(d.newGroupPath, d.newGroup, false))
	return newAddr
}

func (d *DrandTest) RunReshare(oldRun, newRun int, timeout string) {
	var clientCounter = &sync.WaitGroup{}
	runreshare := func(dr *Drand, leader bool) {
		// instruct to be ready for a reshare
		client, err := net.NewControlClient(dr.opts.controlPort)
		require.NoError(d.t, err)
		_, err = client.InitReshare(d.groupPath, d.newGroupPath, leader, timeout)
		require.NoError(d.t, err)
		fmt.Printf("\n\nDKG TEST: drand %s DONE RESHARING (%v)\n", dr.priv.Public.Address(), leader)
		clientCounter.Done()
	}

	// take list of old nodes present in new groups
	var oldNodes []string
	for _, id := range d.ids {
		drand := d.drands[id]
		if d.newGroup.Contains(drand.priv.Public) {
			oldNodes = append(oldNodes, drand.priv.Public.Address())
		}
	}

	var allIds []string
	// run the old ones
	// exclude leader
	clientCounter.Add(oldRun - 1)
	for _, id := range oldNodes[1:oldRun] {
		fmt.Println("Launching reshare on old", id)
		go runreshare(d.drands[id], false)
		allIds = append(allIds, id)
	}
	// stop the rest
	for _, id := range oldNodes[oldRun:] {
		d.drands[id].Stop()
	}

	// run the new ones
	clientCounter.Add(newRun)
	for _, id := range d.newIds[:newRun] {
		fmt.Println("Launching reshare on new", id)
		go runreshare(d.newDrands[id], false)
		allIds = append(allIds, id)
	}
	allIds = append(allIds, oldNodes[0])
	d.setDKGCallback(allIds)
	// run leader
	fmt.Println("Launching reshare on (old) root", d.ids[0])
	clientCounter.Add(1)
	go runreshare(d.drands[oldNodes[0]], true)
	// wait for the return of the clients
	checkWait(clientCounter)
	fmt.Printf("\n\n -- TEST FINISHED ALL RESHARE DKG --\n\n")
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
	case <-time.After(11 * time.Second):
		panic("outdated beacon time")
	}
}
func NewDrandTest(t *testing.T, n, thr int, period time.Duration) *DrandTest {
	drands, group, dir, certPaths := BatchNewDrand(n, false,
		WithCallOption(grpc.FailFast(true)),
	)
	group.Period = period
	groupPath := path.Join(dir, "dkggroup.toml")
	require.NoError(t, key.Save(groupPath, group, false))
	ids := make([]string, n)
	mDrands := make(map[string]*Drand, n)
	for i, d := range drands {
		ids[i] = d.priv.Public.Address()
		mDrands[ids[i]] = d
	}
	return &DrandTest{
		t:         t,
		n:         n,
		thr:       thr,
		drands:    mDrands,
		group:     group,
		groupPath: groupPath,
		period:    period,
		ids:       ids,
		shares:    make(map[string]*key.Share),
		clocks:    make(map[string]*SyncClock),
		certPaths: certPaths,
	}
}

func (d *DrandTest) RunDKG() {
	// XXX make it optional
	uo := false
	er := entropy.NewEntropyReader("")

	var wg sync.WaitGroup
	wg.Add(d.n - 1)
	d.setClock(d.ids...)
	d.setDKGCallback(d.ids)
	for _, id := range d.ids[1:] {
		go func(dd *Drand) {
			client, err := net.NewControlClient(dd.opts.controlPort)
			require.NoError(d.t, err)
			_, err = client.InitDKG(d.groupPath, false, "", er, uo)
			require.NoError(d.t, err)
			wg.Done()
			fmt.Printf("\n\n\n TESTDKG NON-ROOT %s FINISHED\n\n\n", dd.priv.Public.Address())
		}(d.drands[id])
	}

	root := d.drands[d.ids[0]]
	controlClient, err := net.NewControlClient(root.opts.controlPort)
	require.NoError(d.t, err)
	_, err = controlClient.InitDKG(d.groupPath, true, "", er, uo)
	require.NoError(d.t, err)
	wg.Wait()
	fmt.Printf("\n\n\n TESTDKG ROOT %s FINISHED\n\n\n", d.ids[0])
	resp, err := controlClient.GroupFile()
	require.NoError(d.t, err)
	group := new(key.Group)
	groupToml := new(key.GroupTOML)
	_, err = toml.Decode(resp.GetGroupToml(), groupToml)
	require.NoError(d.t, err)
	require.NoError(d.t, group.FromTOML(groupToml))
	d.group = group
	require.Equal(d.t, d.thr, d.group.Threshold)
	for _, drand := range d.drands {
		require.True(d.t, d.group.Contains(drand.priv.Public))
	}
	require.Len(d.t, d.group.PublicKey.Coefficients, d.thr)
	require.NoError(d.t, key.Save(d.groupPath, d.group, false))
}

func (d *DrandTest) tryBoth(id string, fn func(d *Drand)) {
	if dr, ok := d.drands[id]; ok {
		fn(dr)
	} else if dr, ok = d.newDrands[id]; ok {
		fn(dr)
	} else {
		panic("that should not happen")
	}
}

func (d *DrandTest) setClock(ids ...string) {
	for _, id := range ids {
		d.tryBoth(id, func(dr *Drand) {
			c := &SyncClock{
				Mock:  clock.NewMock(),
				Mutex: new(sync.Mutex),
			}
			addr := dr.priv.Public.Address()
			d.clocks[addr] = c
			dr.opts.clock = c
			dr.opts.dkgCallback = func(s *key.Share) {
				d.shares[addr] = s
				fmt.Printf("\n\n\n  --- DKG %s FINISHED ---\n\n\n", addr)
			}
		})
	}
}

// first wait for all dkg callbacks to trigger, then update the clock of every
// ids
func (d *DrandTest) setDKGCallback(ids []string) {
	var wg sync.WaitGroup
	wg.Add(len(ids))
	for _, id := range ids {
		d.tryBoth(id, func(dr *Drand) {
			dr.opts.dkgCallback = func(s *key.Share) {
				d.shares[dr.priv.Public.Address()] = s
				fmt.Printf("\n\n %s DKG DONE \n\n", dr.priv.Public.Address())
				wg.Done()
			}
		})
	}
	go func() {
		wg.Wait()
		// TRAVIS: let all peers go into sleep mode before increasing
		// their clock
		time.Sleep(100 * time.Millisecond)
		for _, id := range ids {
			d.clocks[id].Add(syncTime)
		}
	}()
}

func (d *DrandTest) GetDrand(id string) *Drand {
	return d.drands[id]
}

func (d *DrandTest) StopDrand(id string) {
	dr := d.drands[id]
	dr.Stop()
	pinger, err := net.NewControlClient(dr.opts.controlPort)
	require.NoError(d.t, err)
	var counter = 1
	for range time.Tick(100 * time.Millisecond) {
		if err := pinger.Ping(); err != nil {
			break
		}
		counter++
		require.LessOrEqual(d.t, counter, 5)
	}
}

func (d *DrandTest) StartDrand(id string, catchup bool) {
	dr, ok := d.drands[id]
	require.True(d.t, ok)
	var err error
	dr, err = LoadDrand(dr.store, dr.opts)
	require.NoError(d.t, err)
	d.drands[id] = dr
	d.setClock(id)
	dr.StartBeacon(catchup)
}

func (d *DrandTest) MoveTime(p time.Duration) {
	for _, c := range d.clocks {
		c.Add(p)
	}
	time.Sleep(getSleepDuration())
}

func (d *DrandTest) TestBeaconLength(max int, ids ...string) {
	for _, id := range ids {
		drand, ok := d.drands[id]
		require.True(d.t, ok)
		require.LessOrEqual(d.t, drand.beaconStore.Len(), max)
	}

}

func (d *DrandTest) TestPublicBeacon(id string) {
	dr := d.GetDrand(id)
	client := net.NewGrpcClientFromCertManager(dr.opts.certmanager, dr.opts.grpcOpts...)
	resp, err := client.PublicRand(test.NewTLSPeer(dr.priv.Public.Addr), &drand.PublicRandRequest{})
	require.NoError(d.t, err)
	require.NotNil(d.t, resp)
}

func TestDrandDKGFresh(t *testing.T) {
	n := 5
	p := 200 * time.Millisecond
	dt := NewDrandTest(t, n, key.DefaultThreshold(n), p)
	defer dt.Cleanup()
	dt.RunDKG()
	// make the last node fail
	// XXX The node still replies to early beacon packet
	lastID := dt.ids[n-1]
	lastOne := dt.GetDrand(lastID)
	lastOne.Stop()
	// test everyone has two beacon except the one we stopped
	dt.MoveTime(p)
	dt.TestBeaconLength(2, dt.ids[:n-1]...)

	// start last one
	dt.StartDrand(lastID, true)
	dt.MoveTime(p)
	dt.TestBeaconLength(3, dt.ids[:n-1]...)
	// 2 because the first beacon is ran automatically by everyone, can't stop
	// it before at the moment
	dt.TestBeaconLength(2, lastID)
	dt.TestPublicBeacon(dt.ids[0])
}

// Check they all have same public group file after dkg
func TestDrandPublicGroup(t *testing.T) {
	n := 10
	thr := key.DefaultThreshold(n)
	p := 200 * time.Millisecond
	dt := NewDrandTest(t, n, thr, p)
	defer dt.Cleanup()
	dt.RunDKG()

	//client := NewGrpcClient()
	cm := dt.drands[dt.ids[0]].opts.certmanager
	client := NewGrpcClientFromCert(cm)
	rest := net.NewRestClientFromCertManager(cm)
	var group *drand.GroupResponse
	for i, id := range dt.ids {
		d := dt.drands[id]
		groupResp, err := client.Group(d.priv.Public.Address(), d.priv.Public.TLS)
		require.NoError(t, err, fmt.Sprintf("idx %d: addr %s", i, id))
		if group == nil {
			group = groupResp
		}
		require.Equal(t, uint32(group.Period), groupResp.Period)
		require.Equal(t, uint32(group.Threshold), groupResp.Threshold)
		require.Equal(t, group.Distkey, groupResp.Distkey)
		require.Len(t, groupResp.Nodes, len(group.Nodes))

		nodes := groupResp.GetNodes()
		for addr, d := range dt.drands {
			var found bool
			for _, n := range nodes {
				sameAddr := n.GetAddress() == addr
				sameKey := n.GetKey() == key.PointToString(d.priv.Public.Key)
				sameTLS := n.GetTLS() == d.priv.Public.TLS
				if sameAddr && sameKey && sameTLS {
					found = true
					break
				}
			}
			require.True(t, found)
		}
		restGroup, err := rest.Group(d.priv.Public, &drand.GroupRequest{})
		require.NoError(t, err)
		require.Equal(t, groupResp, restGroup)
	}
}

// BatchNewDrand returns n drands, using TLS or not, with the given
// options. It returns the list of Drand structures, the group created,
// the folder where db, certificates, etc are stored. It is the folder
// to delete at the end of the test. As well, it returns a public grpc
// client that can reach any drand node.
func BatchNewDrand(n int, insecure bool, opts ...ConfigOption) ([]*Drand, *key.Group, string, []string) {
	var privs []*key.Pair
	var group *key.Group
	if insecure {
		privs, group = test.BatchIdentities(n)
	} else {
		privs, group = test.BatchTLSIdentities(n)
	}
	ports := test.Ports(n)
	var err error
	drands := make([]*Drand, n, n)
	tmp := os.TempDir()
	dir, err := ioutil.TempDir(tmp, "drand")
	if err != nil {
		panic(err)
	}

	certPaths := make([]string, n)
	keyPaths := make([]string, n)
	if !insecure {
		for i := 0; i < n; i++ {
			certPath := path.Join(dir, fmt.Sprintf("server-%d.crt", i))
			keyPath := path.Join(dir, fmt.Sprintf("server-%d.key", i))
			if httpscerts.Check(certPath, keyPath) != nil {

				h, _, err := gnet.SplitHostPort(privs[i].Public.Address())
				if err != nil {
					panic(err)
				}
				if err := httpscerts.Generate(certPath, keyPath, h); err != nil {
					panic(err)
				}
			}
			certPaths[i] = certPath
			keyPaths[i] = keyPath
		}
	}

	for i := 0; i < n; i++ {
		s := test.NewKeyStore()
		s.SaveKeyPair(privs[i])
		// give each one their own private folder
		dbFolder := path.Join(dir, fmt.Sprintf("db-%d", i))
		confOptions := append([]ConfigOption{WithDbFolder(dbFolder)}, opts...)
		if !insecure {
			confOptions = append(confOptions, WithTLS(certPaths[i], keyPaths[i]))
			confOptions = append(confOptions, WithTrustedCerts(certPaths...))
		} else {
			confOptions = append(confOptions, WithInsecure())
		}
		confOptions = append(confOptions, WithControlPort(ports[i]))
		confOptions = append(confOptions, WithLogLevel(log.LogDebug))
		drands[i], err = NewDrand(s, NewConfig(confOptions...))
		if err != nil {
			panic(err)
		}
	}
	return drands, group, dir, certPaths
}

// CloseAllDrands closes all drands
func CloseAllDrands(drands []*Drand) {
	for i := 0; i < len(drands); i++ {
		drands[i].Stop()
		//os.RemoveAll(drands[i].opts.dbFolder)
	}
}
