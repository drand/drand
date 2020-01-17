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

	"github.com/benbjohnson/clock"
	"github.com/drand/drand/beacon"
	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"
	"github.com/kabukky/httpscerts"
	"github.com/stretchr/testify/require"
)

func TestDrandDKGReshareTimeout(t *testing.T) {
	oldN := 5 // 4 / 5
	newN := 6 // 5 / 6
	oldT := key.DefaultThreshold(oldN)
	newT := key.DefaultThreshold(newN)
	timeout := "3s"
	offline := 1 // can't do more anyway with a 2/3 + 1 threshold
	fmt.Printf("%d/%d -> %d/%d\n", oldT, oldN, newT, newN)

	// create n shares
	shares, dpub := test.SimulateDKG(t, key.KeyGroup, oldN, oldT)
	period := 1000 * time.Millisecond

	// instantiating all drands already
	drands, _, dir := BatchNewDrand(newN, false,
		WithCallOption(grpc.FailFast(true)))
	defer CloseAllDrands(drands)
	defer os.RemoveAll(dir)

	// listing all new ids
	ids := make([]*key.Identity, newN)
	for i, d := range drands {
		ids[i] = d.priv.Public
		drands[i].idx = i
	}

	// creating old group from subset of ids
	oldGroup := key.LoadGroup(ids[:oldN], &key.DistPublic{Coefficients: dpub}, oldT)
	oldGroup.Period = period
	oldPath := path.Join(dir, "oldgroup.toml")
	require.NoError(t, key.Save(oldPath, oldGroup, false))

	for i := range drands[:oldN] {
		// so old drand nodes "think" it already ran a dkg a first time.
		drands[i].group = oldGroup
		drands[i].dkgDone = true
	}

	// creating the new group from the whole set of keys
	shuffledIds := make([]*key.Identity, len(ids))
	copy(shuffledIds, ids)
	// shuffle with random swaps
	for i := 0; i < len(ids)*3; i++ {
		i1 := rand.Intn(len(ids))
		i2 := rand.Intn(len(ids))
		shuffledIds[i1], shuffledIds[i2] = shuffledIds[i2], shuffledIds[i1]
	}
	newGroup := key.NewGroup(shuffledIds, newT)
	newGroup.Period = period
	newPath := path.Join(dir, "newgroup.toml")
	require.NoError(t, key.Save(newPath, newGroup, false))

	fmt.Printf("oldGroup: %v\n", oldGroup.String())
	fmt.Printf("newGroup: %v\n", newGroup.String())
	fmt.Printf("offline nodes: ")
	for _, d := range drands[1 : 1+offline] {
		fmt.Printf("%s -", d.priv.Public.Addr)
	}
	fmt.Println()
	var wg sync.WaitGroup
	wg.Add(newN - 1 - offline)
	// skip the offline ones and the "first" (as leader)
	for i, drand := range drands[1+offline:] {
		go func(d *Drand, j int) {
			if d.idx < oldN {
				// simulate share material for old node
				ks := &key.Share{
					Share:   shares[d.idx],
					Commits: dpub,
				}
				d.share = ks
			}

			// instruct to be ready for a reshare
			client, err := net.NewControlClient(d.opts.controlPort)
			require.NoError(t, err)
			_, err = client.InitReshare(oldPath, newPath, false, timeout)
			fmt.Printf("drand %s: %v\n", d.priv.Public.Addr, err)
			require.NoError(t, err)
			wg.Done()
		}(drand, i)
	}
	// let a bit of time so everybody has performed the initreshare
	time.Sleep(100 * time.Millisecond)
	dkgDone := make(chan bool, 1)
	go func() {
		ks := key.Share{
			Share:   shares[0],
			Commits: dpub,
		}
		root := drands[0]
		root.share = &ks
		//err := root.StartDKG(c)
		client, err := net.NewControlClient(root.opts.controlPort)
		require.NoError(t, err)
		_, err = client.InitReshare(oldPath, newPath, true, timeout)
		require.NoError(t, err)
		dkgDone <- true
	}()

	tt, _ := time.ParseDuration(timeout)
	var timeoutDone bool
	select {
	case <-dkgDone:
		require.True(t, timeoutDone)
	case <-time.After(tt - 500*time.Millisecond):
		timeoutDone = true
	}

	wg.Wait()

}

func TestDrandDKGReshare(t *testing.T) {

	oldN := 5
	newN := 6
	oldT := key.DefaultThreshold(oldN)
	newT := key.DefaultThreshold(newN)

	// create n shares
	shares, dpub := test.SimulateDKG(t, key.KeyGroup, oldN, oldT)
	period := 1000 * time.Millisecond

	// instantiating all drands already
	drands, _, dir := BatchNewDrand(newN, false,
		WithCallOption(grpc.FailFast(true)))
	defer CloseAllDrands(drands)
	defer os.RemoveAll(dir)

	// listing all new ids
	ids := make([]*key.Identity, newN)
	for i, d := range drands {
		ids[i] = d.priv.Public
		drands[i].idx = i
	}

	// creating old group from subset of ids
	oldGroup := key.LoadGroup(ids[:oldN], &key.DistPublic{Coefficients: dpub}, oldT)
	oldGroup.Period = period
	oldPath := path.Join(dir, "oldgroup.toml")
	require.NoError(t, key.Save(oldPath, oldGroup, false))

	for i := range drands[:oldN] {
		// so old drand nodes "think" it has already ran a dkg
		drands[i].group = oldGroup
		drands[i].dkgDone = true
	}

	newGroup := key.NewGroup(ids, newT)
	newGroup.Period = period
	newPath := path.Join(dir, "newgroup.toml")
	require.NoError(t, key.Save(newPath, newGroup, false))

	var wg sync.WaitGroup
	wg.Add(newN - 1)
	for i, drand := range drands[1:] {
		go func(d *Drand, j int) {
			if d.idx < oldN {
				// simulate share material for old node
				ks := &key.Share{
					Share:   shares[d.idx],
					Commits: dpub,
				}
				d.share = ks
			}

			// instruct to be ready for a reshare
			client, err := net.NewControlClient(d.opts.controlPort)
			require.NoError(t, err)
			_, err = client.InitReshare(oldPath, newPath, false, "")
			require.NoError(t, err)
			wg.Done()
		}(drand, i)
	}

	ks := key.Share{
		Share:   shares[0],
		Commits: dpub,
	}
	root := drands[0]
	root.share = &ks
	//err := root.StartDKG(c)
	client, err := net.NewControlClient(root.opts.controlPort)
	require.NoError(t, err)
	_, err = client.InitReshare(oldPath, newPath, true, "")
	require.NoError(t, err)
	//err = root.WaitDKG()
	//require.NoError(t, err)
	wg.Wait()

}

type DrandTest struct {
	t         *testing.T
	n         int
	thr       int
	dir       string
	drands    map[string]*Drand
	group     *key.Group
	groupPath string
	period    time.Duration
	ids       []string
	shares    map[string]*key.Share
	clock     *clock.Mock
}

func (d *DrandTest) Cleanup() {
	os.RemoveAll(d.dir)
}

func (d *DrandTest) GetBeacon(id string, round int) (*beacon.Beacon, error) {
	dd, ok := d.drands[id]
	require.True(d.t, ok)

	return dd.beaconStore.Get(uint64(round))
}

func NewDrandTest(t *testing.T, n, thr int, period time.Duration) *DrandTest {
	drands, group, dir := BatchNewDrand(n, false,
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
		clock:     clock.NewMock(),
		shares:    make(map[string]*key.Share),
	}
}

func (d *DrandTest) RunDKG() {
	var wg sync.WaitGroup
	wg.Add(d.n - 1)
	for _, id := range d.ids[1:] {
		go func(dd *Drand) {
			d.setCallbacks(dd)
			client, err := net.NewControlClient(dd.opts.controlPort)
			require.NoError(d.t, err)
			_, err = client.InitDKG(d.groupPath, false, "")
			require.NoError(d.t, err)
			wg.Done()
		}(d.drands[id])
	}

	root := d.drands[d.ids[0]]
	d.setCallbacks(root)
	controlClient, err := net.NewControlClient(root.opts.controlPort)
	require.NoError(d.t, err)
	_, err = controlClient.InitDKG(d.groupPath, true, "")
	require.NoError(d.t, err)
	wg.Wait()
}

func (d *DrandTest) setCallbacks(dr *Drand) {
	dr.opts.dkgCallback = func(s *key.Share) {
		d.shares[dr.priv.Public.Address()] = s
	}
	dr.opts.clock = d.clock
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
	d.setCallbacks(dr)
	dr.StartBeacon(catchup)
}

func (d *DrandTest) MoveTime(p time.Duration) {
	d.clock.Add(p)
	d.clock.Add(50 * time.Millisecond)
	sleepTime := 20 * d.n
	time.Sleep(time.Duration(sleepTime) * time.Millisecond)
}

func (d *DrandTest) TestBeaconLength(length int, ids ...string) {
	for _, id := range ids {
		drand, ok := d.drands[id]
		require.True(d.t, ok)
		require.Less(d.t, drand.beaconStore.Len(), length)
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
	n := 10
	p := 200 * time.Millisecond
	dt := NewDrandTest(t, n, 7, p)
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
func BatchNewDrand(n int, insecure bool, opts ...ConfigOption) ([]*Drand, *key.Group, string) {
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
		drands[i], err = NewDrand(s, NewConfig(confOptions...))
		if err != nil {
			panic(err)
		}
	}
	return drands, group, dir
}

// CloseAllDrands closes all drands
func CloseAllDrands(drands []*Drand) {
	for i := 0; i < len(drands); i++ {
		drands[i].Stop()
		//os.RemoveAll(drands[i].opts.dbFolder)
	}
}
