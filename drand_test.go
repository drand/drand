package main

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dedis/drand/bls"
	"github.com/nikkolasg/slog"
	"github.com/stretchr/testify/require"
)

func TestDrandDKG(t *testing.T) {
	slog.Level = slog.LevelDebug
	n := 5
	_, drands := BatchDrands(n)
	defer CloseAllDrands(drands)

	var wg sync.WaitGroup
	wg.Add(n - 1)
	for _, drand := range drands[1:] {
		go func(d *Drand) {
			err := d.RunDKG()
			require.Nil(t, err)
			wg.Done()
		}(drand)
	}
	root := drands[0]
	err := root.StartDKG()
	require.Nil(t, err)
	wg.Wait()

	// check if share + dist public files are saved
	_, err = root.store.LoadDistPublic()
	require.Nil(t, err)
	_, err = root.store.LoadShare()
	require.Nil(t, err)
}

func TestDrandDKGReverse(t *testing.T) {
	slog.Level = slog.LevelDebug
	n := 5
	_, drands := BatchDrands(n)
	defer CloseAllDrands(drands)

	var wg sync.WaitGroup
	wg.Add(n - 1)
	for i := n - 1; i >= 0; i-- {
		go func(j int, d *Drand) {
			var err error
			if j == 0 {
				err = d.StartDKG()
			} else {
				err = d.RunDKG()
			}
			require.Nil(t, err)
			wg.Done()
		}(i, drands[i])
	}
	wg.Wait()
	root := drands[0]
	// check if share + dist public files are saved
	_, err := root.store.LoadDistPublic()
	require.Nil(t, err)
	_, err = root.store.LoadShare()
	require.Nil(t, err)
}

func TestDrandTBLS(t *testing.T) {
	n := 5
	_, drands := BatchDrands(n)
	//defer CloseAllDrands(drands)
	slog.Level = slog.LevelDebug

	// do the dkg
	var wg sync.WaitGroup
	wg.Add(n - 1)
	for _, drand := range drands[1:] {
		go func(d *Drand) {
			err := d.RunDKG()
			require.Nil(t, err)
			fmt.Println(" !!!!!!!!!!!!!!! dkg", d.r.addr, " FINISHED")
			wg.Done()
		}(drand)
	}
	root := drands[0]
	err := root.StartDKG()
	require.Nil(t, err)
	fmt.Println("DKG WAIT <<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
	wg.Wait()
	time.Sleep(50 * time.Millisecond)
	fmt.Println("DKG DONE <<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
	// do a round of tbls
	wg = sync.WaitGroup{}
	wg.Add(n - 1)
	var wait sync.WaitGroup
	wait.Add(n - 1)
	for _, drand := range drands[1:] {
		go func(d *Drand) {
			wait.Done()
			d.Loop()
			wg.Done()
		}(drand)
	}
	wait.Wait()

	seed := []byte("beaconing is so good")
	period := 80 * time.Millisecond
	// launch the beacon
	// XXX
	// can't stop a ticker so can't stop this function
	go root.RandomBeacon(seed, period)

	// sleep a while
	time.Sleep(3 * period)
	// finish everyone
	for _, drand := range drands {
		drand.Stop()
	}
	wg.Wait()
	testStore := root.store.(*TestStore)
	require.True(t, len(testStore.Signatures) >= 1)
	_, err = root.store.LoadShare()
	require.Nil(t, err)
	public, err := root.store.LoadDistPublic()
	require.Nil(t, err)
	for _, bs := range testStore.Signatures {
		msg := bs.Request.Message()
		require.Nil(t, bls.Verify(pairing, public.Key, msg, bs.RawSig()))
	}

}

func TestDrandTBLSReverse(t *testing.T) {
	n := 5
	_, drands := BatchDrands(n)
	//defer CloseAllDrands(drands)
	slog.Level = slog.LevelDebug

	// do the dkg
	var wg sync.WaitGroup
	wg.Add(n)
	for i := n - 1; i >= 0; i-- {
		go func(j int, d *Drand) {
			var err error
			if j == 0 {
				err = d.StartDKG()
			} else {
				err = d.RunDKG()
			}
			require.Nil(t, err)
			wg.Done()
		}(i, drands[i])
	}
	fmt.Println("DKG WAIT <<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
	wg.Wait()
	time.Sleep(50 * time.Millisecond)
	fmt.Println("DKG DONE <<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
	// do a round of tbls
	wg = sync.WaitGroup{}
	wg.Add(n - 1)
	var wait sync.WaitGroup
	wait.Add(n - 1)
	for _, drand := range drands[1:] {
		go func(d *Drand) {
			wait.Done()
			d.Loop()
			wg.Done()
		}(drand)
	}
	wait.Wait()

	var err error
	root := drands[0]
	seed := []byte("beaconing is so good")
	period := 80 * time.Millisecond
	// launch the beacon
	// XXX
	// can't stop a ticker so can't stop this function
	go root.RandomBeacon(seed, period)

	// sleep a while
	time.Sleep(3 * period)
	// finish everyone
	for _, drand := range drands {
		drand.Stop()
	}
	wg.Wait()
	testStore := root.store.(*TestStore)
	require.True(t, len(testStore.Signatures) >= 1)
	_, err = root.store.LoadShare()
	require.Nil(t, err)
	public, err := root.store.LoadDistPublic()
	require.Nil(t, err)
	for _, bs := range testStore.Signatures {
		msg := bs.Request.Message()
		require.Nil(t, bls.Verify(pairing, public.Key, msg, bs.RawSig()))
	}

}
