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
	time.Sleep(100 * time.Millisecond)
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
	public, err := root.store.LoadShare()
	require.Nil(t, err)
	for _, bs := range testStore.Signatures {
		msg := message(bs.Request.PreviousSig, bs.Request.Timestamp)
		require.Nil(t, bls.Verify(pairing, public.Commits[0], msg, bs.RawSig()))
	}

}
