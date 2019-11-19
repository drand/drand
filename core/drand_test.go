package core

import (
	"bytes"
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

	"github.com/dedis/drand/beacon"
	"github.com/dedis/drand/entropy"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	"github.com/dedis/drand/protobuf/drand"
	"github.com/dedis/drand/test"
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

func TestDrandDKGFresh(t *testing.T) {
	n := 5
	nbRound := 4
	period := 1000 * time.Millisecond

	emptyReader := ""
	defaultUserOnly := false
	defaultEntropyReader := entropy.NewEntropyReader(emptyReader, defaultUserOnly)

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

	var distributedPublic *key.DistPublic
	var publicSet = make(chan bool)
	getPublic := func() *key.DistPublic {
		<-publicSet
		return distributedPublic
	}

	type receivingStruct struct {
		Index  int
		Beacon *beacon.Beacon
	}

	// storing beacons from all nodes indexed per round
	genBeacons := make(map[uint64][]*receivingStruct)
	var l sync.Mutex
	// this is just to signal we received a new beacon
	newBeacon := make(chan int, n*nbRound)
	// function printing the map
	displayInfo := func(prelude string) {
		l.Lock()
		defer l.Unlock()
		if false {
			fmt.Printf("\n --- %s ++ Beacon Map ---\n", prelude)
			for round, beacons := range genBeacons {
				fmt.Printf("\tround %d = ", round)
				for _, b := range beacons {
					fmt.Printf("(%d) %s -", b.Index, drands[b.Index].priv.Public.Addr)
				}
				fmt.Printf("\n")
			}
			fmt.Printf("\n --- ---------- ---\n")
		}
	}
	// setupDrand setups the callbacks related to beacon generation. When a beacon
	// is ready from that node, it saves the beacon and the node index
	// into the genBeacons map.
	setupDrand := func(i int) {
		//addr := drands[i].priv.Public.Address()
		myCb := func(b *beacon.Beacon) {
			msg := beacon.Message(b.PreviousSig, b.Round)
			err := key.Scheme.VerifyRecovered(getPublic().Key(), msg, b.Signature)
			if err != nil {
				fmt.Printf("Beacon error callback: %s\n", b.Signature)
			}
			require.NoError(t, err)
			l.Lock()
			genBeacons[b.Round] = append(genBeacons[b.Round], &receivingStruct{
				Index:  i,
				Beacon: b,
			})
			l.Unlock()
			//fmt.Printf("\n /\\/\\ New Beacon Round %d from node %d: %s /\\/\\\n", b.Round, i, addr)
			//displayInfo()
			newBeacon <- i
		}
		drands[i].opts.beaconCbs = append(drands[i].opts.beaconCbs, myCb)
	}

	for i := 0; i < n-1; i++ {
		setupDrand(i)
	}

	var wg sync.WaitGroup
	wg.Add(n - 1)
	for _, drand := range drands[1:] {
		go func(d *Drand) {
			// instruct to be ready for a reshare
			client, err := net.NewControlClient(d.opts.controlPort)
			require.NoError(t, err)
			_, err = client.InitDKG(groupPath, false, "", defaultEntropyReader)
			require.NoError(t, err)
			//err = d.WaitDKG()
			//require.Nil(t, err)
			wg.Done()
		}(drand)
	}

	root := drands[0]
	controlClient, err := net.NewControlClient(root.opts.controlPort)
	require.NoError(t, err)
	_, err = controlClient.InitDKG(groupPath, true, "", defaultEntropyReader)
	require.NoError(t, err)

	//err = root.WaitDKG()
	//require.Nil(t, err)
	wg.Wait()

	// check if share + dist public files are saved
	distributedPublic, err = root.store.LoadDistPublic()
	require.Nil(t, err)
	require.NotNil(t, distributedPublic)
	close(publicSet)
	_, err = root.store.LoadShare()
	require.Nil(t, err)

	// make the last node fail
	// XXX The node still replies to early beacon packet
	lastOne := drands[n-1]
	lastOne.Stop()
	pinger, err := net.NewControlClient(lastOne.opts.controlPort)
	require.NoError(t, err)
	var counter = 1
	for range time.Tick(100 * time.Millisecond) {
		if err := pinger.Ping(); err != nil {
			break
		}
		counter++
		if counter > 5 {
			require.False(t, true, "last drand should be off by now")
		}
	}
	fmt.Printf("\n\n\n\nStopping last drand %s\n Leader is %s\n\n\n\n\n\n\n\n", lastOne.priv.Public.Addr, root.priv.Public.Addr)

	//expected := nbRound * n
	done := make(chan bool)
	// test how many beacons are generated per each handler, except the last
	// handler that will start later
	countGenBeacons := func(rounds, beaconPerRound int, doneCh chan bool) {
		displayInfo("STARTING BEACON")
		for {
			<-newBeacon
			l.Lock()
			// do we have enough rounds made
			if len(genBeacons) < rounds {
				l.Unlock()
				displayInfo("NOTENOUGH ROUNDS")
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
					displayInfo("NOTENOUGH fullrounds")
					continue
				}
			}
			l.Unlock()
			displayInfo("FULL MAPPING")
			l.Lock()
			// let's check if they are all equal
			for _, beacons := range genBeacons {
				original := beacons[0].Beacon
				for _, beacon := range beacons[1:] {
					if !bytes.Equal(beacon.Beacon.Signature, original.Signature) {
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
	// nbRound - 1 because in the first round, the leader is
	// "too fast"
	// n-1 since we have one node down
	go countGenBeacons(nbRound-1, n-1, done)

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

	//fmt.Printf("\n\n\n\n\n HELLLOOOOO FINISHED FIRST PASS \n\n\n\n\n\n\n\n\n")

	drands[n-1], err = LoadDrand(drands[n-1].store, drands[n-1].opts)
	require.NoError(t, err)
	lastDrand := drands[n-1]
	fmt.Printf("\n\n#1\n\n")
	defer CloseAllDrands(drands[n-1:])
	setupDrand(n - 1)
	lastDrand.StartBeacon(true)
	go countGenBeacons(nbRound, n, done)
	checkSuccess()

	//fmt.Printf("\n\n\n\n\n HELLLOOOOO FINISHED SECOND PASS \n\n\n\n\n\n\n\n\n")

	client := net.NewGrpcClientFromCertManager(root.opts.certmanager, root.opts.grpcOpts...)
	resp, err := client.PublicRand(test.NewTLSPeer(root.priv.Public.Addr), &drand.PublicRandRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestDrandPublicGroup(t *testing.T) {

	n := 5
	thr := key.DefaultThreshold(n)

	// create n shares
	_, dpub := test.SimulateDKG(t, key.KeyGroup, n, thr)
	period := 1000 * time.Millisecond

	// instantiating all drands already
	drands, _, dir := BatchNewDrand(n, true)
	defer CloseAllDrands(drands)
	defer os.RemoveAll(dir)

	// listing all new ids
	ids := make([]*key.Identity, n)
	for i, d := range drands {
		ids[i] = d.priv.Public
		drands[i].idx = i
	}

	// creating old group from subset of ids
	group1 := key.LoadGroup(ids, &key.DistPublic{Coefficients: dpub}, thr)
	group1.Period = period
	path := path.Join(dir, "group.toml")
	require.NoError(t, key.Save(path, group1, false))

	gtoml := group1.TOML().(*key.GroupTOML)

	for i := range drands {
		// so old drand nodes "think" it has already ran a dkg
		drands[i].group = group1
		drands[i].dkgDone = true
	}

	client := NewGrpcClient()
	rest := net.NewRestClient()
	for _, d := range drands {
		groupResp, err := client.Group(d.priv.Public.Address(), d.priv.Public.TLS)
		require.NoError(t, err)

		require.Equal(t, uint32(group1.Period), groupResp.Period*1e6)
		require.Equal(t, uint32(group1.Threshold), groupResp.Threshold)
		require.Equal(t, gtoml.PublicKey.Coefficients, groupResp.Distkey)
		require.Len(t, groupResp.Nodes, len(group1.Nodes))
		for i, n := range group1.Nodes {
			n2 := groupResp.Nodes[i]
			require.Equal(t, n.Addr, n2.Address)
			require.Equal(t, gtoml.Nodes[i].Key, n2.Key)
			require.Equal(t, n.TLS, n2.TLS)
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
