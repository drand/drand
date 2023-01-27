package beacon

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/boltdb"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/test"
	context2 "github.com/drand/drand/test/context"
)

func TestSchemeStore(t *testing.T) {
	dir := t.TempDir()
	ctx, sch, _ := context2.PrevSignatureMatersOnContext(t, context.Background())

	l := test.Logger(t)
	bstore, err := boltdb.NewBoltStore(ctx, l, dir, nil)
	require.NoError(t, err)

	genesisBeacon := chain.GenesisBeacon([]byte("genesis_signature"))
	err = bstore.Put(ctx, genesisBeacon)
	require.NoError(t, err)

	ss, err := NewSchemeStore(bstore, sch)
	require.NoError(t, err)

	newBeacon := &chain.Beacon{
		Round:       1,
		Signature:   []byte("signature_1"),
		PreviousSig: []byte("genesis_signature"),
	}
	err = ss.Put(ctx, newBeacon)
	require.NoError(t, err)

	beaconSaved, err := ss.Last(ctx)
	require.NoError(t, err)

	switch sch.Name {
	case crypto.DefaultSchemeID: // we're in chained mode it should keep the consistency between prev signature and signature
		if !bytes.Equal(beaconSaved.PreviousSig, genesisBeacon.Signature) {
			t.Errorf("previous signature on last beacon [%s] should be equal to previous beacon signature [%s]",
				beaconSaved.PreviousSig, genesisBeacon.PreviousSig)
		}
	default: // we're in unchained mode
		if beaconSaved.PreviousSig != nil {
			t.Errorf("previous signature should be nil")
		}
	}

	newBeacon = &chain.Beacon{
		Round:       2,
		Signature:   []byte("signature_2"),
		PreviousSig: nil,
	}

	err = ss.Put(ctx, newBeacon)
	switch sch.Name {
	case crypto.DefaultSchemeID: // we're in chained mode
		if err == nil {
			t.Errorf("new beacon should not be allow to be put on store")
		}
	default: // we're in unchained mode
		if err != nil {
			t.Errorf("new beacon should be allow to be put on store")
		}
	}
}
