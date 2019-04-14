package core

import (
	"bytes"
	"fmt"
	"io/ioutil"
	gnet "net"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/dedis/drand/beacon"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	"github.com/dedis/drand/protobuf/drand"
	"github.com/dedis/drand/test"
	"github.com/dedis/kyber/sign/bls"
	"github.com/kabukky/httpscerts"
	"github.com/nikkolasg/slog"
	"github.com/stretchr/testify/require"
)

func TestDrandDKGReshareTimeout(t *testing.T) {
	slog.Level = slog.LevelDebug
	oldN := 5 // 4 / 5
	newN := 6 // 5 / 6
	oldT := key.DefaultThreshold(oldN)
	newT := key.DefaultThreshold(newN)
	timeout := "3s"
	offline := 1 // can't do more anyway with a 2/3 + 1 threshold
	fmt.Printf("%d/%d -> %d/%d\n", oldT, oldN, newT, newN)

	// create n shares
	shares, dpub := test.SimulateDKG(t, key.G2, oldN, oldT)
	period := 1000 * time.Millisecond
	old := net.DefaultTimeout
	net.DefaultTimeout = 300 * time.Millisecond
	defer func() { net.DefaultTimeout = old }()

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
	newGroup := key.NewGroup(ids, newT)
	newGroup.Period = period
	newPath := path.Join(dir, "newgroup.toml")
	require.NoError(t, key.Save(newPath, newGroup, false))

	var wg sync.WaitGroup
	wg.Add(newN - 1 - offline)
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
			require.NoError(t, err)
			wg.Done()
		}(drand, i)
	}

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
	slog.Level = slog.LevelDebug

	oldN := 5
	newN := 6
	oldT := key.DefaultThreshold(oldN)
	newT := key.DefaultThreshold(newN)

	// create n shares
	shares, dpub := test.SimulateDKG(t, key.G2, oldN, oldT)
	period := 1000 * time.Millisecond
	old := net.DefaultTimeout
	net.DefaultTimeout = 300 * time.Millisecond
	defer func() { net.DefaultTimeout = old }()

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

func TestDrandDKGFresh(t *testing.T) {
	slog.Level = slog.LevelDebug

	n := 5
	nbRound := 3
	period := 1000 * time.Millisecond
	old := net.DefaultTimeout
	net.DefaultTimeout = 300 * time.Millisecond
	defer func() { net.DefaultTimeout = old }()

	drands, group, dir := BatchNewDrand(n, false,
		WithCallOption(grpc.FailFast(true)))
	defer CloseAllDrands(drands[:n-1])
	defer os.RemoveAll(dir)

	group.Period = period
	groupPath := path.Join(dir, "dkggroup.toml")
	require.NoError(t, key.Save(groupPath, group, false))

	ids := make([]*key.Identity, n)
	for i, d := range drands {
		ids[i] = d.priv.Public
	}

	var wg sync.WaitGroup
	wg.Add(n - 1)
	for _, drand := range drands[1:] {
		go func(d *Drand) {
			// instruct to be ready for a reshare
			client, err := net.NewControlClient(d.opts.controlPort)
			require.NoError(t, err)
			_, err = client.InitDKG(groupPath, false, "")
			require.NoError(t, err)
			//err = d.WaitDKG()
			//require.Nil(t, err)
			wg.Done()
		}(drand)
	}

	root := drands[0]
	controlClient, err := net.NewControlClient(root.opts.controlPort)
	require.NoError(t, err)
	_, err = controlClient.InitDKG(groupPath, true, "")
	require.NoError(t, err)

	//err = root.WaitDKG()
	//require.Nil(t, err)
	wg.Wait()

	// check if share + dist public files are saved
	public, err := root.store.LoadDistPublic()
	require.Nil(t, err)
	require.NotNil(t, public)
	_, err = root.store.LoadShare()
	require.Nil(t, err)

	// make the last node fail
	drands[n-1].Stop()

	type receiveStruct struct {
		I int
		B *beacon.Beacon
	}
	// storing beacons from all nodes indexed per round
	genBeacons := make(map[uint64][]*beacon.Beacon)
	var l sync.Mutex
	// this is just to signal we received a new beacon
	newBeacon := make(chan int, n*nbRound)
	// launchDrand will launch the beacon at the given index. Each time a new
	// beacon is ready from that node, it saves the beacon and the node index
	// into the map
	launchDrand := func(i int) {
		myCb := func(b *beacon.Beacon) {
			msg := beacon.Message(b.PreviousRand, b.Round)
			err := bls.Verify(key.Pairing, public.Key(), msg, b.Randomness)
			if err != nil {
				fmt.Printf("Beacon error callback: %s\n", b.Randomness)
			}
			require.NoError(t, err)
			l.Lock()
			genBeacons[b.Round] = append(genBeacons[b.Round], b)
			l.Unlock()
			newBeacon <- i
		}
		drands[i].opts.beaconCbs = append(drands[i].opts.beaconCbs, myCb)
		//fmt.Printf(" --- Launch drand %s\n", drands[i].priv.Public.Address())
		go drands[i].BeaconLoop()
	}

	for i := 0; i < n-1; i++ {
		launchDrand(i)
	}

	/* displayInfo := func() {*/
	//l.Lock()
	//defer l.Unlock()
	//for round, beacons := range genBeacons {
	//fmt.Printf("round %d = %d beacons.", round, len(beacons))
	//}
	//fmt.Printf("\n")
	/*}*/
	//expected := nbRound * n
	done := make(chan bool)
	// test how many beacons are generated per each handler, except the last
	// handler that will start later
	countGenBeacons := func(rounds, beaconPerRound int, doneCh chan bool) {
		for {
			<-newBeacon
			l.Lock()
			// do we have enough rounds made
			if len(genBeacons) < rounds {
				l.Unlock()
				continue
			} else {
				// do we have enough beacons for enough rounds
				// we want at least <rounds> rounds with at least
				// <beaconPerRound> beacons in each
				fullRounds := 0
				for _, beacons := range genBeacons {
					if len(beacons) >= beaconPerRound {
						fullRounds++
					}
				}
				if fullRounds < rounds {
					l.Unlock()
					continue
				}
			}
			l.Unlock()
			//displayInfo()
			l.Lock()
			// let's check if they are all equal
			for _, beacons := range genBeacons {
				original := beacons[0]
				for _, beacon := range beacons[1:] {
					if !bytes.Equal(beacon.Randomness, original.Randomness) {
						// randomness is not equal we return false
						l.Unlock()
						doneCh <- false
						return
					}
				}
			}
			l.Unlock()
			doneCh <- true
			return
		}
	}
	go countGenBeacons(nbRound, n-1, done)

	checkSuccess := func() {
		select {
		case success := <-done:
			if !success {
				t.Fatal("Not all equal")
			}
			// erase the map
			l.Lock()
			for i := range genBeacons {
				delete(genBeacons, i)
			}
			l.Unlock()
			//case <-time.After(period * time.Duration(nbRound*60)):
			//t.Fatal("not in time")
		}
	}

	checkSuccess()

	lastDrand := drands[n-1]
	drands[n-1], err = LoadDrand(lastDrand.store, lastDrand.opts)
	require.NoError(t, err)
	defer CloseAllDrands(drands[n-1:])
	// trick the late drand into thinking it already has some beacon
	// only need that trick for the test, it's easier
	require.NoError(t, drands[n-1].beaconStore.Put(&beacon.Beacon{
		Round:        56,
		PreviousRand: []byte{0x01, 0x02, 0x03},
		Randomness:   []byte("best randomness ever"),
	}))
	// ugly trick to not get the callback triggered because it gets triggered in
	// a goroutine, and the callback are set before by launchDrand.
	time.Sleep(100 * time.Millisecond)
	// the logic should make the drand catchup automatically
	launchDrand(n - 1)
	go countGenBeacons(nbRound, n, done)
	checkSuccess()

	client := net.NewGrpcClientFromCertManager(root.opts.certmanager, root.opts.grpcOpts...)
	resp, err := client.Public(test.NewTLSPeer(root.priv.Public.Addr), &drand.PublicRandRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

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

func CloseAllDrands(drands []*Drand) {
	for i := 0; i < len(drands); i++ {
		drands[i].Stop()
		os.RemoveAll(drands[i].opts.dbFolder)
	}
}
