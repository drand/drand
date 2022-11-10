package beacon

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/boltdb"
	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/log"
)

func TestSchemeStore(t *testing.T) {
	sch, _ := scheme.ReadSchemeByEnv()

	dir := t.TempDir()
	ctx := context.Background()

	logLevel := log.LogInfo
	debugEnv, isDebug := os.LookupEnv("DRAND_TEST_LOGS")
	if isDebug && debugEnv == "DEBUG" {
		t.Log("Enabling LogDebug logs")
		logLevel = log.LogDebug
	}

	l := log.NewLogger(nil, logLevel)
	bstore, err := boltdb.NewBoltStore(l, dir, nil)
	require.NoError(t, err)

	genesisBeacon := chain.GenesisBeacon(&chain.Info{GenesisSeed: []byte("genesis_signature")})
	err = bstore.Put(ctx, genesisBeacon)
	require.NoError(t, err)

	ss := NewSchemeStore(bstore, sch)

	newBeacon := &chain.Beacon{
		Round:       1,
		Signature:   []byte("signature_1"),
		PreviousSig: []byte("genesis_signature"),
	}
	err = ss.Put(ctx, newBeacon)
	require.NoError(t, err)

	beaconSaved, err := ss.Last(ctx)
	require.NoError(t, err)

	// test if store sets to nil prev signature depending on scheme
	// with chained scheme, it should keep the consistency between prev signature and signature
	if sch.DecouplePrevSig && beaconSaved.PreviousSig != nil {
		t.Errorf("previous signature should be nil")
	} else if !sch.DecouplePrevSig && !bytes.Equal(beaconSaved.PreviousSig, genesisBeacon.Signature) {
		t.Errorf("previous signature on last beacon [%s] should be equal to previous beacon signature [%s]",
			beaconSaved.PreviousSig, genesisBeacon.PreviousSig)
	}

	newBeacon = &chain.Beacon{
		Round:       2,
		Signature:   []byte("signature_2"),
		PreviousSig: nil,
	}

	err = ss.Put(ctx, newBeacon)

	// test if store checks consistency between signature and prev signature depending on the scheme
	if sch.DecouplePrevSig && err != nil {
		t.Errorf("new beacon should be allow to be put on store")
	} else if !sch.DecouplePrevSig && err == nil {
		t.Errorf("new beacon should not be allow to be put on store")
	}
}
