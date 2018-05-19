package core

import (
	"fmt"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/dedis/drand/beacon"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/test"
	"github.com/dedis/kyber/sign/bls"
	"github.com/nikkolasg/slog"
	"github.com/stretchr/testify/require"
)

func TestDrandDKG(t *testing.T) {
	slog.Level = slog.LevelDebug

	n := 5
	nbBeacons := 3
	//thr := key.DefaultThreshold(n)
	period := 700 * time.Millisecond

	drands := BatchNewDrand(n,
		WithBeaconPeriod(period))
	defer CloseAllDrands(drands)

	var wg sync.WaitGroup
	wg.Add(n - 1)
	for _, drand := range drands[1:] {
		go func(d *Drand) {
			err := d.WaitDKG()
			require.Nil(t, err)
			wg.Done()
		}(drand)
	}

	root := drands[0]
	err := root.StartDKG()
	require.Nil(t, err)
	wg.Wait()

	// check if share + dist public files are saved
	public, err := root.store.LoadDistPublic()
	require.Nil(t, err)
	require.NotNil(t, public)
	_, err = root.store.LoadShare()
	require.Nil(t, err)

	receivedChan := make(chan int, nbBeacons*n)
	expected := nbBeacons * n
	// launchBeacon will launch the beacon at the given index. Each time a new
	// beacon is ready from that node, it indicates it by sending the index on
	// the receivedChan channel.
	launchBeacon := func(i int) {
		curr := 0
		myCb := func(b *beacon.Beacon) {
			err := bls.Verify(key.Pairing, public.Key, beacon.Message(b.PreviousRand, b.Round), b.Randomness)
			require.NoError(t, err)
			curr++
			//slog.Printf(" --- TEST: new beacon generated (%d/%d) from node %d", curr, nbBeacons, i)
			receivedChan <- i
		}
		//fmt.Printf(" --- TEST: callback for node %d: %p\n", i, myCb)
		drands[i].opts.beaconCbs = append(drands[i].opts.beaconCbs, myCb)
		go drands[i].BeaconLoop()
	}

	for i := 0; i < n; i++ {
		launchBeacon(i)
	}

	done := make(chan bool)
	// keep track of how many do we have
	go func() {
		receivedIdx := make(map[int]int)
		for count := 0; count < expected; count++ {
			receivedIdx[<-receivedChan]++
		}
		var correct = true
		for i, count := range receivedIdx {
			if count != nbBeacons {
				fmt.Printf(" -- Node %d has only generated %d/%d beacons", i, count, nbBeacons)
				correct = false
				break
			}
		}
		done <- correct
	}()

	select {
	case correct := <-done:
		if !correct {
			t.Fatal()
		}
	case <-time.After(period * time.Duration(nbBeacons*2)):
		t.Fatal("not in time")
	}

	client := NewClient(root.opts.grpcOpts...)
	//fmt.Printf("testing client functionality with public key %x\n", public.Key)
	resp, err := client.LastPublic(root.priv.Public.Addr, public)
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func BatchNewDrand(n int, opts ...ConfigOption) []*Drand {
	privs, group := test.BatchIdentities(n)
	var err error
	drands := make([]*Drand, n, n)
	tmp := os.TempDir()
	for i := 0; i < n; i++ {
		s := test.NewKeyStore()
		s.SavePrivate(privs[i])
		// give each one their own private folder
		dbFolder := path.Join(tmp, fmt.Sprintf("db-%d", i))
		drands[i], err = NewDrand(s, group, NewConfig(append([]ConfigOption{WithDbFolder(dbFolder)}, opts...)...))
		if err != nil {
			panic(err)
		}
	}
	return drands
}

func CloseAllDrands(drands []*Drand) {
	for i := 0; i < len(drands); i++ {
		drands[i].Stop()
		os.RemoveAll(drands[i].opts.dbFolder)
	}
}
