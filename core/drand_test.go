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
	client := NewClient(root.opts.grpcOpts...)
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
