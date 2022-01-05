package beacon

import (
	"bytes"
	"os"
	"testing"

	"github.com/drand/drand/common/scheme"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/boltdb"
	"github.com/stretchr/testify/require"
)

func TestSchemeStore(t *testing.T) {
	sch, _ := scheme.ReadSchemeByEnv()

	dir, err := os.MkdirTemp("", "*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	bstore, err := boltdb.NewBoltStore(dir, nil)
	require.NoError(t, err)

	genesisBeacon := chain.GenesisBeacon(&chain.Info{GroupHash: []byte("genesis_signature")})
	err = bstore.Put(genesisBeacon)
	require.NoError(t, err)

	ss := newSchemeStore(bstore, sch)

	newBeacon := &chain.Beacon{
		Round:       1,
		Signature:   []byte("signature_1"),
		PreviousSig: []byte("genesis_signature"),
	}
	err = ss.Put(newBeacon)
	require.NoError(t, err)

	beaconSaved, err := ss.Last()
	require.NoError(t, err)

	// test if store sets to nil prev signature depending on scheme
	// with chained scheme, it should keep the consistency between prev signature and signature
	if sch.DecouplePrevSig && beaconSaved.PreviousSig != nil {
		t.Errorf("previous signature should be nil")
	} else if !sch.DecouplePrevSig && !bytes.Equal(beaconSaved.PreviousSig, genesisBeacon.Signature) {
		t.Errorf("previous signature on last beacon [%s] should be equal to previos beacon signature [%s]", beaconSaved.PreviousSig, genesisBeacon.PreviousSig)
	}

	newBeacon = &chain.Beacon{
		Round:       2,
		Signature:   []byte("signature_2"),
		PreviousSig: nil,
	}

	err = ss.Put(newBeacon)

	// test if store checks consistency between signature and prev signature depending on the scheme
	if sch.DecouplePrevSig && err != nil {
		t.Errorf("new beacon should be allow to be put on store")
	} else if !sch.DecouplePrevSig && err == nil {
		t.Errorf("new beacon should not be allow to be put on store")
	}
}
