package core

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	gnet "net"
	"os"
	"path"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"

	"github.com/kabukky/httpscerts"
	"github.com/stretchr/testify/require"
)

// 1 second after end of dkg
var testBeaconOffset = 1
var testDkgTimeout = 2 * time.Second

func TestDrandDKGFresh(t *testing.T) {
	n := 4
	beaconPeriod := 1 * time.Second

	dt := NewDrandTest2(t, n, key.DefaultThreshold(n), beaconPeriod)
	defer dt.Cleanup()
	finalGroup := dt.RunDKG()
	time.Sleep(getSleepDuration())
	fmt.Println(" --- DKG FINISHED ---")
	// make the last node fail
	lastID := dt.nodes[n-1].addr
	dt.StopDrand(lastID, false)
	fmt.Printf("\n--- lastOne STOPPED %s --- \n\n", lastID)

	// move time to genesis
	// dt.MoveTime(offsetGenesis)
	now := dt.Now().Unix()
	beaconStart := finalGroup.GenesisTime
	diff := beaconStart - now
	dt.MoveTime(time.Duration(diff) * time.Second)
	// two = genesis + 1st round (happens at genesis)
	fmt.Println(" --- Test BEACON LENGTH --- ")
	dt.TestBeaconLength(2, false, dt.Ids(n-1, false)...)
	fmt.Printf("\n\n --- START LAST DRAND %s ---\n\n", lastID)
	// start last one
	dt.StartDrand(lastID, true, false)
	// leave some room to do the catchup
	time.Sleep(100 * time.Millisecond)
	fmt.Printf("\n\n --- STARTED BEACON DRAND %s ---\n\n", lastID)
	time.Sleep(2 * time.Second)
	dt.MoveTime(beaconPeriod)
	dt.TestBeaconLength(3, false, dt.Ids(n, false)...)
	dt.TestPublicBeacon(lastID, false)
}

func TestDrandDKGBroadcastDeny(t *testing.T) {
	n := 10
	thr := 6
	beaconPeriod := 1 * time.Second

	dt := NewDrandTest2(t, n, thr, beaconPeriod)
	defer dt.Cleanup()
	// close connection between a pair of nodes
	n1 := dt.nodes[1]
	n2 := dt.nodes[2]
	n1.drand.DenyBroadcastTo(n2.addr)
	n2.drand.DenyBroadcastTo(n1.addr)
	group1 := dt.RunDKG()
	dt.MoveToTime(group1.GenesisTime)
	dt.MoveTime(1 * time.Second)
	fmt.Printf("\n\n --- DKG FINISHED ---\n\n")
	time.Sleep(200 * time.Millisecond)
	_, err := dt.RunReshare(n, 0, thr, 1*time.Second, false, false)
	require.NoError(t, err)
	fmt.Println(" --- RESHARING FINISHED ---")
}

func TestDrandReshareForce(t *testing.T) {
	oldN := 4
	oldThr := 3
	timeout := 1 * time.Second
	beaconPeriod := 2 * time.Second

	dt := NewDrandTest2(t, oldN, oldThr, beaconPeriod)
	defer dt.Cleanup()
	group1 := dt.RunDKG()
	// make sure all nodes had enough time to run their go routines to start the
	// beacon handler - related to CI problems
	time.Sleep(getSleepDuration())
	dt.MoveToTime(group1.GenesisTime)
	dt.MoveTime(1 * time.Second)

	// run the resharing
	go dt.RunReshare(oldN, 0, oldThr, timeout, false, true)
	time.Sleep(500 * time.Millisecond)

	// force
	fmt.Printf("\n\n\nSTARTING RESHARING AGAIN\n\n\n")
	group3, err := dt.RunReshare(oldN, 0, oldThr, timeout, true, false)
	require.NoError(t, err)
	fmt.Printf("\n -- Move to Response phase !! -- \n")
	fmt.Println(group3)
}

func TestDrandDKGReshareTimeout(t *testing.T) {
	oldN := 3
	newN := 4
	oldThr := 2
	newThr := 3
	timeout := 1 * time.Second
	beaconPeriod := 2 * time.Second
	offline := 1

	dt := NewDrandTest2(t, oldN, oldThr, beaconPeriod)
	defer dt.Cleanup()
	group1 := dt.RunDKG()
	// make sure all nodes had enough time to run their go routines to start the
	// beacon handler - related to CI problems
	time.Sleep(getSleepDuration())
	dt.MoveToTime(group1.GenesisTime)
	// move to genesis time - so nodes start to make a round
	// dt.MoveTime(offsetGenesis)
	// two = genesis + 1st round (happens at genesis)
	dt.TestBeaconLength(2, false, dt.Ids(oldN, false)...)
	// so nodes think they are going forward with round 2
	dt.MoveTime(1 * time.Second)

	// + offline makes sure t
	toKeep := oldN - offline
	toAdd := newN - toKeep
	dt.SetupNewNodes(toAdd)

	fmt.Println("SETUP RESHARE DONE")
	// run the resharing
	var doneReshare = make(chan *key.Group)
	go func() {
		group, err := dt.RunReshare(toKeep, toAdd, newThr, timeout, false, false)
		require.NoError(t, err)
		doneReshare <- group
	}()
	time.Sleep(3 * time.Second)
	fmt.Printf("\n -- Move to Response phase !! -- \n")
	dt.MoveTime(timeout)
	// at this point in time, nodes should have gotten all deals and send back
	// their responses to all nodes
	time.Sleep(getSleepDuration())
	fmt.Printf("\n -- Move to Justif phase !! -- \n")
	dt.MoveTime(timeout)
	// at this time, all nodes received the responses of each other nodes but
	// there is one node missing so they expect justifications
	time.Sleep(getSleepDuration())
	fmt.Printf("\n -- Move to Finish phase !! -- \n")
	dt.MoveTime(timeout)
	time.Sleep(getSleepDuration())
	// at this time they received no justification from the missing node so he's
	// exlucded of the group and the dkg should finish
	// time.Sleep(10 * time.Second)
	var resharedGroup *key.Group
	select {
	case resharedGroup = <-doneReshare:
	case <-time.After(1 * time.Second):
		require.True(t, false)
	}
	fmt.Println(" RESHARED GROUP:", resharedGroup)
	target := resharedGroup.TransitionTime
	now := dt.Now().Unix()
	// get rounds from first node in the "old" group - since he's the leader for
	// the new group, he's alive
	lastBeacon := dt.TestPublicBeacon(dt.Ids(1, false)[0], false)
	// move to the transition time period by period - do not skip potential
	// periods as to emulate the normal time behavior
	for now < target-1 {
		dt.MoveTime(beaconPeriod)
		lastBeacon = dt.TestPublicBeacon(dt.Ids(1, false)[0], false)
		now = dt.Now().Unix()
	}
	// move to the transition time
	dt.MoveToTime(resharedGroup.TransitionTime)
	time.Sleep(getSleepDuration())
	// test that all nodes in the new group have generated a new beacon
	dt.TestBeaconLength(int(lastBeacon.Round+1), true, dt.Ids(newN, true)...)
}

func TestDrandResharePreempt(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping testing in CI environment")
	}

	oldN := 3
	newN := 3
	Thr := 2
	timeout := 1 * time.Second
	beaconPeriod := 2 * time.Second

	dt := NewDrandTest2(t, oldN, Thr, beaconPeriod)
	defer dt.Cleanup()
	group1 := dt.RunDKG()
	// make sure all nodes had enough time to run their go routines to start the
	// beacon handler - related to CI problems
	time.Sleep(getSleepDuration())
	dt.MoveToTime(group1.GenesisTime)
	// move to genesis time - so nodes start to make a round
	dt.TestBeaconLength(2, false, dt.Ids(oldN, false)...)
	// so nodes think they are going forward with round 2
	dt.MoveTime(1 * time.Second)

	// first, the leader is going to start running a failed reshare:
	oldNode := dt.group.Find(dt.nodes[0].drand.priv.Public)
	if oldNode == nil {
		panic("leader not found in old group")
	}
	// old root: oldNode.Index leater: leader.addr
	oldDone := make(chan error, 1)
	go func() {
		client, err := net.NewControlClient(dt.nodes[0].drand.opts.controlPort)
		require.NoError(t, err)
		_, err = client.InitReshareLeader(newN, Thr, timeout, 0, "unused secret", "", testBeaconOffset)
		// Done resharing
		if err == nil {
			panic("initial reshare should fail.")
		}
		oldDone <- err
	}()
	time.Sleep(100 * time.Millisecond)

	// run the resharing
	var doneReshare = make(chan *key.Group, 1)
	go func() {
		g, err := dt.RunReshare(oldN, 0, Thr, timeout, false, false)
		require.NoError(t, err)
		doneReshare <- g
	}()
	time.Sleep(time.Second)
	dt.MoveTime(time.Second)
	time.Sleep(time.Second)
	fmt.Printf("\n -- Move to Response phase !! -- \n")
	dt.MoveTime(timeout)
	// at this point in time, nodes should have gotten all deals and send back
	// their responses to all nodes
	time.Sleep(getSleepDuration())
	fmt.Printf("\n -- Move to Justif phase !! -- \n")
	dt.MoveTime(timeout)
	// at this time, all nodes received the responses of each other nodes but
	// there is one node missing so they expect justifications
	time.Sleep(getSleepDuration())
	fmt.Printf("\n -- Move to Finish phase !! -- \n")
	dt.MoveTime(timeout)
	time.Sleep(getSleepDuration())
	// at this time they received no justification from the missing node so he's
	// exlucded of the group and the dkg should finish
	select {
	case <-doneReshare:
	case <-time.After(1 * time.Second):
		panic("expect dkg to have finished within one second")
	}
	select {
	case <-oldDone:
	case <-time.After(1 * time.Second):
		panic("expect aborted dkg to fail")
	}
	dt.TestPublicBeacon(dt.Ids(1, false)[0], false)
}

// Check they all have same chain info
func TestDrandPublicChainInfo(t *testing.T) {
	n := 10
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	dt := NewDrandTest2(t, n, thr, p)
	defer dt.Cleanup()
	group := dt.RunDKG()
	ci := chain.NewChainInfo(group)
	cm := dt.nodes[0].drand.opts.certmanager
	client := NewGrpcClientFromCert(cm)
	for _, node := range dt.nodes {
		d := node.drand
		received, err := client.ChainInfo(d.priv.Public)
		require.NoError(t, err, fmt.Sprintf("addr %s", node.addr))
		require.True(t, ci.Equal(received))
	}
	for _, node := range dt.nodes {
		var found bool
		addr := node.addr
		public := node.drand.priv.Public
		for _, n := range group.Nodes {
			sameAddr := n.Address() == addr
			sameKey := n.Key.Equal(public.Key)
			sameTLS := n.IsTLS() == public.TLS
			if sameAddr && sameKey && sameTLS {
				found = true
				break
			}
		}
		require.True(t, found)
	}

	// rest := net.NewRestClientFromCertManager(cm)
	// restGroup, err := rest.Group(context.TODO(), dt.nodes[0].drand.priv.Public, &drand.GroupRequest{})
	// require.NoError(t, err)
	// received, err := key.GroupFromProto(restGroup)
	// require.NoError(t, err)
	// require.True(t, group.Equal(received))
}

// Test if the we can correctly fetch the rounds after a DKG using the
// PublicRand RPC call
func TestDrandPublicRand(t *testing.T) {
	n := 4
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	dt := NewDrandTest2(t, n, thr, p)
	defer dt.Cleanup()
	group := dt.RunDKG()
	time.Sleep(getSleepDuration())
	root := dt.nodes[0].drand
	rootID := root.priv.Public

	dt.MoveToTime(group.GenesisTime)
	// do a few periods
	for i := 0; i < 3; i++ {
		dt.MoveTime(group.Period)
	}

	cm := root.opts.certmanager
	client := net.NewGrpcClientFromCertManager(cm)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// get last round first
	resp, err := client.PublicRand(ctx, rootID, new(drand.PublicRandRequest))
	require.NoError(t, err)

	initRound := resp.Round + 1
	max := initRound + 4
	for i := initRound; i < max; i++ {
		dt.MoveTime(group.Period)
		req := new(drand.PublicRandRequest)
		req.Round = i
		resp, err := client.PublicRand(ctx, rootID, req)
		require.NoError(t, err)
		require.Equal(t, i, resp.Round)
		fmt.Println("REQUEST ROUND ", i, " GOT ROUND ", resp.Round)
	}
}

// Test if the we can correctly fetch the rounds after a DKG using the
// PublicRandStream RPC call
// It also test the follow method call (it avoid redoing an expensive and long
// setup on CI to test both behaviors).
func TestDrandPublicStream(t *testing.T) {
	n := 4
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	dt := NewDrandTest2(t, n, thr, p)
	defer dt.Cleanup()
	group := dt.RunDKG()
	time.Sleep(getSleepDuration())
	root := dt.nodes[0]
	rootID := root.drand.priv.Public

	dt.MoveToTime(group.GenesisTime)
	// do a few periods
	for i := 0; i < 3; i++ {
		dt.MoveTime(group.Period)
	}

	cm := root.drand.opts.certmanager
	client := net.NewGrpcClientFromCertManager(cm)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// get last round first
	resp, err := client.PublicRand(ctx, rootID, new(drand.PublicRandRequest))
	require.NoError(t, err)

	//  run streaming and expect responses
	req := &drand.PublicRandRequest{Round: resp.GetRound()}
	respCh, err := client.PublicRandStream(ctx, root.drand.priv.Public, req)
	require.NoError(t, err)
	// expect first round now since node already has it
	select {
	case beacon := <-respCh:
		require.Equal(t, beacon.GetRound(), resp.GetRound())
	case <-time.After(100 * time.Millisecond):
		require.True(t, false, "too late")
	}
	nTry := 4
	// we expect the next one now
	initRound := resp.Round + 1
	maxRound := initRound + uint64(nTry)
	fmt.Println("Streaming for future rounds starting from", initRound)
	for round := initRound; round < maxRound; round++ {
		// move time to next period
		dt.MoveTime(group.Period)
		select {
		case beacon := <-respCh:
			require.Equal(t, beacon.GetRound(), round)
		case <-time.After(1 * time.Second):
			require.True(t, false, "too late for streaming, round %d didn't reply in time", round)
		}
	}
	// try fetching with round 0 -> get latest
	respCh, err = client.PublicRandStream(ctx, root.drand.priv.Public, new(drand.PublicRandRequest))
	require.NoError(t, err)
	select {
	case <-respCh:
		require.False(t, true, "shouldn't get an entry if time doesn't go by")
	case <-time.After(50 * time.Millisecond):
		// correct
	}

	dt.MoveTime(group.Period)
	select {
	case resp := <-respCh:
		require.Equal(t, maxRound, resp.GetRound())
	case <-time.After(50 * time.Millisecond):
		// correct
	}
}
func TestDrandFollowChain(tt *testing.T) {
	n := 4
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	dt := NewDrandTest2(tt, n, thr, p)
	defer dt.Cleanup()
	group := dt.RunDKG()
	time.Sleep(getSleepDuration())
	root := dt.nodes[0]
	rootID := root.drand.priv.Public

	dt.MoveToTime(group.GenesisTime)
	// do a few periods
	for i := 0; i < 10; i++ {
		dt.MoveTime(group.Period)
	}

	cm := root.drand.opts.certmanager
	client := net.NewGrpcClientFromCertManager(cm)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// get last round first
	resp, err := client.PublicRand(ctx, rootID, new(drand.PublicRandRequest))
	require.NoError(tt, err)

	// TEST setup a new node and fetch history

	newNode := dt.SetupNewNodes(1)[0]
	newClient, err := net.NewControlClient(newNode.drand.opts.controlPort)
	require.NoError(tt, err)
	addrToFollow := []string{rootID.Address()}
	hash := fmt.Sprintf("%x", chain.NewChainInfo(group).Hash())
	tls := true
	fn := func(upTo, exp uint64) {
		ctx, cancel = context.WithCancel(context.Background())
		progress, errCh, err := newClient.StartFollowChain(ctx, hash, addrToFollow, tls, upTo)
		require.NoError(tt, err)
		var goon = true
		for goon {
			select {
			case p, ok := <-progress:
				if ok && p.Current == exp {
					// success
					fmt.Printf("\n\nSUCCESSSSSS\n\n")
					goon = false
					break
				}
			case e := <-errCh:
				if e == io.EOF {
					break
				}
				require.NoError(tt, e)
			case <-time.After(1 * time.Second):
				tt.FailNow()
			}
		}
		// cancel the operation
		cancel()

		// check if the beacon is in the database
		store, err := newNode.drand.createBoltStore()
		require.NoError(tt, err)
		defer store.Close()
		lastB, err := store.Last()
		require.NoError(tt, err)
		require.Equal(tt, exp, lastB.Round, "found %d vs expected %d", lastB.Round, exp)
	}
	fn(resp.GetRound()-2, resp.GetRound()-2)
	time.Sleep(200 * time.Millisecond)
	fn(0, resp.GetRound())
}

// Test if the we can correctly fetch the rounds through the local proxy
func TestDrandPublicStreamProxy(t *testing.T) {
	n := 4
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second
	dt := NewDrandTest2(t, n, thr, p)
	defer dt.Cleanup()
	group := dt.RunDKG()
	time.Sleep(getSleepDuration())
	root := dt.nodes[0]

	dt.MoveToTime(group.GenesisTime)
	// do a few periods
	for i := 0; i < 3; i++ {
		dt.MoveTime(group.Period)
	}

	client := &drandProxy{root.drand}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// get last round first
	resp, err := client.Get(ctx, 0)
	require.NoError(t, err)

	//  run streaming and expect responses
	rc := client.Watch(ctx)
	// expect first round now since node already has it
	dt.MoveTime(group.Period)
	beacon, ok := <-rc
	if !ok {
		t.Fatal("expected beacon")
	}
	require.Equal(t, beacon.Round(), resp.Round()+1)
	nTry := 4
	// we expect the next one now
	initRound := resp.Round() + 2
	maxRound := initRound + uint64(nTry)
	for round := initRound; round < maxRound; round++ {
		// move time to next period
		dt.MoveTime(group.Period)
		beacon, ok = <-rc
		require.True(t, ok)
		require.Equal(t, round, beacon.Round())
	}
}

// BatchNewDrand returns n drands, using TLS or not, with the given
// options. It returns the list of Drand structures, the group created,
// the folder where db, certificates, etc are stored. It is the folder
// to delete at the end of the test. As well, it returns a public grpc
// client that can reach any drand node.
func BatchNewDrand(n int, insecure bool, opts ...ConfigOption) (drands []*Drand, group *key.Group, dir string, certPaths []string) {
	var privs []*key.Pair
	if insecure {
		privs, group = test.BatchIdentities(n)
	} else {
		privs, group = test.BatchTLSIdentities(n)
	}
	ports := test.Ports(n)
	var err error
	drands = make([]*Drand, n)
	tmp := os.TempDir()
	ioDir, err := ioutil.TempDir(tmp, "drand")
	if err != nil {
		panic(err)
	}
	dir = ioDir

	certPaths = make([]string, n)
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
		confOptions := []ConfigOption{WithDBFolder(dbFolder)}
		if !insecure {
			confOptions = append(confOptions,
				WithTLS(certPaths[i], keyPaths[i]),
				WithTrustedCerts(certPaths...))
		} else {
			confOptions = append(confOptions, WithInsecure())
		}
		confOptions = append(confOptions,
			WithControlPort(ports[i]),
			WithLogLevel(log.LogDebug))
		// add options in last so it overwrites the default
		confOptions = append(confOptions, opts...)
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
		drands[i].Stop(context.Background())
	}
}

func getSleepDuration() time.Duration {
	if os.Getenv("CIRCLE_CI") != "" {
		fmt.Println("--- Sleeping on CIRCLECI")
		return time.Duration(600) * time.Millisecond
	}
	return time.Duration(500) * time.Millisecond
}
