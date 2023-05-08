package beacon

import (
	"bytes"
	"context"
	"testing"

	"github.com/drand/drand/common"
	"github.com/drand/drand/crypto"
	"github.com/drand/drand/internal/chain"
	"github.com/drand/drand/internal/chain/boltdb"
	context2 "github.com/drand/drand/internal/test/context"
	"github.com/drand/drand/internal/test/testlogger"
	"github.com/stretchr/testify/require"
)

func TestSchemeStore(t *testing.T) {
	dir := t.TempDir()
	ctx, sch, _ := context2.PrevSignatureMattersOnContext(t, context.Background())

	l := testlogger.New(t)
	bstore, err := boltdb.NewBoltStore(ctx, l, dir, nil)
	require.NoError(t, err)

	genesisBeacon := chain.GenesisBeacon([]byte("genesis_signature"))
	err = bstore.Put(ctx, genesisBeacon)
	require.NoError(t, err)

	ss, err := NewSchemeStore(ctx, bstore, sch)
	require.NoError(t, err)

	newBeacon := &common.Beacon{
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

	newBeacon = &common.Beacon{
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
