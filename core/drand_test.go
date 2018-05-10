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
	//thr := key.DefaultThreshold(n)
	period := 400 * time.Millisecond

	beaconCh := make(chan *beacon.Beacon, 1)
	cb := func(b *beacon.Beacon) {
		beaconCh <- b
	}
	drands := BatchNewDrand(n,
		WithBeaconPeriod(period),
		WithBeaconCallback(cb))
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

	go root.BeaconLoop()
	select {
	case b := <-beaconCh:
		err := bls.Verify(key.Pairing, public.Key, beacon.Message(b.PreviousSig, b.Timestamp), b.Signature)
		require.NoError(t, err)
	case <-time.After(1000 * time.Millisecond):
		t.Fatal("fail")
	}
}

/*func TestDrandDKGReverse(t *testing.T) {*/
////slog.Level = slog.LevelDebug
//n := 5
//_, drands := BatchDrands(n)
//defer CloseAllDrands(drands)

//var wg sync.WaitGroup
//wg.Add(n)
//for i := n - 1; i >= 0; i-- {
//go func(j int, d *Drand) {
//var err error
//if j == 0 {
//err = d.StartDKG()
//} else {
//err = d.RunDKG()
//}
//require.Nil(t, err)
//wg.Done()
//}(i, drands[i])
//}
//wg.Wait()
//root := drands[0]
//// check if share + dist public files are saved
//_, err := root.store.LoadDistPublic()
//require.Nil(t, err)
//_, err = root.store.LoadShare()
//require.Nil(t, err)
//}

//func TestDrandTBLS(t *testing.T) {
//n := 5
//_, drands := BatchDrands(n)
////defer CloseAllDrands(drands)
////slog.Level = slog.LevelDebug

//// do the dkg
//var wg sync.WaitGroup
//wg.Add(n - 1)
//for _, drand := range drands[1:] {
//go func(d *Drand) {
//err := d.RunDKG()
//require.Nil(t, err)
//wg.Done()
//}(drand)
//}
//root := drands[0]
//err := root.StartDKG()
//require.Nil(t, err)
//wg.Wait()
//time.Sleep(50 * time.Millisecond)
//// do a round of tbls
//wg = sync.WaitGroup{}
//wg.Add(n - 1)
//var wait sync.WaitGroup
//wait.Add(n - 1)
//for _, drand := range drands[1:] {
//go func(d *Drand) {
//wait.Done()
//d.Loop()
//wg.Done()
//}(drand)
//}
//wait.Wait()

//seed := []byte("beaconing is so good")
//period := 80 * time.Millisecond
//// launch the beacon
//// XXX
//// can't stop a ticker so can't stop this function
//go root.RandomBeacon(seed, period)

//// sleep a while
//time.Sleep(3 * period)
//// finish everyone
//for _, drand := range drands {
//drand.Stop()
//}
//wg.Wait()
//testStore := root.store.(*TestStore)
//require.True(t, len(testStore.Signatures) >= 1)
//_, err = root.store.LoadShare()
//require.Nil(t, err)
//public, err := root.store.LoadDistPublic()
//require.Nil(t, err)
//for _, bs := range testStore.Signatures {
//msg := bs.Request.Message()
//require.Nil(t, bls.Verify(pairing, public.Key, msg, bs.RawSig()))
//}

//}

//func TestDrandTBLSReverse(t *testing.T) {
//n := 5
//_, drands := BatchDrands(n)
////defer CloseAllDrands(drands)
//slog.Level = slog.LevelDebug

//root := drands[0]
//sigs := make(chan *BeaconSignature, 1)
//root.store.(*TestStore).CbSignatures = func(b *BeaconSignature) {
//sigs <- b
//}
//// do the dkg
//var wg sync.WaitGroup
//// wait for all of them to finish
//wg.Add(n)
//for i := n - 1; i >= 0; i-- {
//go func(j int, d *Drand) {
//var err error
//if j == 0 {
//err = d.StartDKG()
//} else {
//err = d.RunDKG()
//}
//require.Nil(t, err)
//wg.Done()
//}(i, drands[i])
//}
////fmt.Println("DKG WAIT <<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
//wg.Wait()
//time.Sleep(50 * time.Millisecond)
////fmt.Println("DKG DONE <<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")

//// start beacon rounds
//// and waits for them to finish with wg
//wg = sync.WaitGroup{}
//wg.Add(n - 1)
//// wait the start of the n-1 nodes
//var wait sync.WaitGroup
//wait.Add(n - 1)
//for _, drand := range drands[1:] {
//go func(d *Drand) {
//wait.Done()
//d.Loop()
//wg.Done()
//}(drand)
//}
//// wait that everyone is alive
//wait.Wait()

//var err error
//seed := []byte("beaconing is so good")
//period := 80 * time.Millisecond

//// launch the beacon
//// XXX
//// can't stop a ticker so can't stop this function
//go root.RandomBeacon(seed, period)
//<-sigs
//// finish everyone
//for _, drand := range drands {
//drand.Stop()
//}
//wg.Wait()
//testStore := root.store.(*TestStore)
//require.True(t, len(testStore.Signatures) >= 1)
//_, err = root.store.LoadShare()
//require.Nil(t, err)
//public, err := root.store.LoadDistPublic()
//require.Nil(t, err)
//for _, bs := range testStore.Signatures {
//msg := bs.Request.Message()
//require.Nil(t, bls.Verify(pairing, public.Key, msg, bs.RawSig()))
//}

/*}*/

func BatchNewDrand(n int, opts ...DrandOptions) []*Drand {
	privs, group := test.BatchIdentities(n)
	var err error
	drands := make([]*Drand, n, n)
	tmp := os.TempDir()
	for i := 0; i < n; i++ {
		s := test.NewKeyStore()
		s.SavePrivate(privs[i])
		// give each one their own private folder
		dbFolder := path.Join(tmp, fmt.Sprintf("db-%d", i))
		drands[i], err = NewDrand(s, group, append([]DrandOptions{WithDbFolder(dbFolder)}, opts...)...)
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
