package test

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/drand/kyber/util/random"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/testlogger"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/chain"
	"github.com/drand/drand/v2/internal/chain/boltdb"
	context2 "github.com/drand/drand/v2/internal/test/context"
)

// TestEndiannessIntegration tests endianness interoperability across
// multiple drand components.
func TestEndiannessIntegration(t *testing.T) {
	t.Run("RoundSerializationConsistency", testRoundSerializationConsistency)
	t.Run("BeaconStorageAndRetrieval", testBeaconStorageAndRetrieval)
	t.Run("SignatureVerificationAcrossRounds", testSignatureVerificationAcrossRounds)
}

// testRoundSerializationConsistency verifies round serialization consistency.
func testRoundSerializationConsistency(t *testing.T) {
	testCases := []struct {
		name        string
		round       uint64
		expectedHex string
		description string
	}{
		{
			name:        "zero",
			round:       0,
			expectedHex: "0000000000000000",
			description: "Zero round",
		},
		{
			name:        "small_round",
			round:       42,
			expectedHex: "000000000000002a",
			description: "Small round number",
		},
		{
			name:        "byte_boundary",
			round:       256,
			expectedHex: "0000000000000100",
			description: "Round at byte boundary",
		},
		{
			name:        "large_round",
			round:       0x1234567890ABCDEF,
			expectedHex: "1234567890abcdef",
			description: "Large round with distinct bytes",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bytes := chain.RoundToBytes(tc.round)
			require.Len(t, bytes, 8)

			actualHex := ""
			for _, b := range bytes {
				actualHex += string("0123456789abcdef"[b>>4])
				actualHex += string("0123456789abcdef"[b&0x0f])
			}
			require.Equal(t, tc.expectedHex, actualHex)

			recovered := chain.BytesToRound(bytes)
			require.Equal(t, tc.round, recovered)

			if tc.round > 0xFF {
				littleEndianBytes := make([]byte, 8)
				binary.LittleEndian.PutUint64(littleEndianBytes, tc.round)
				require.NotEqual(t, bytes, littleEndianBytes)
			}
		})
	}
}

// testBeaconStorageAndRetrieval tests beacon storage with big-endian round keys.
func testBeaconStorageAndRetrieval(t *testing.T) {
	ctx, _, _ := context2.PrevSignatureMattersOnContext(t, context.Background())
	dir := t.TempDir()
	l := testlogger.New(t)

	store, err := boltdb.NewBoltStore(ctx, l, dir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, store.Close())
	}()

	testRounds := []uint64{0, 1, 2, 3, 4, 5}

	beacons := make([]*common.Beacon, len(testRounds))
	for i, round := range testRounds {
		beacons[i] = &common.Beacon{
			Round:     round,
			Signature: common.HexBytes([]byte{byte(i), byte(i + 1), byte(i + 2)}),
		}
		if i > 0 {
			beacons[i].PreviousSig = beacons[i-1].Signature
		}
		require.NoError(t, store.Put(ctx, beacons[i]))
	}

	for i, round := range testRounds {
		retrieved, err := store.Get(ctx, round)
		require.NoError(t, err)
		require.Equal(t, beacons[i].Round, retrieved.Round)
		require.Equal(t, beacons[i].Signature, retrieved.Signature)

		roundBytes := chain.RoundToBytes(round)
		recoveredRound := chain.BytesToRound(roundBytes)
		require.Equal(t, round, recoveredRound)
	}

	last, err := store.Last(ctx)
	require.NoError(t, err)
	require.Equal(t, testRounds[len(testRounds)-1], last.Round)
}

// testSignatureVerificationAcrossRounds verifies BLS signatures work with
// big-endian digest convention across different rounds.
func testSignatureVerificationAcrossRounds(t *testing.T) {
	scheme := crypto.NewPedersenBLSChained()
	priv := scheme.KeyGroup.Scalar().Pick(random.New())
	pub := scheme.KeyGroup.Point().Mul(priv, nil)

	testRounds := []uint64{0, 1, 255, 256, 65535}
	prevSig := []byte("initial_previous_signature")

	for _, round := range testRounds {
		t.Run(fmt.Sprintf("round_%d", round), func(t *testing.T) {
			digest := createBeaconDigest(prevSig, round)

			sig, err := scheme.AuthScheme.Sign(priv, digest)
			require.NoError(t, err)

			err = scheme.AuthScheme.Verify(pub, digest, sig)
			require.NoError(t, err)

			sig2, err := scheme.AuthScheme.Sign(priv, digest)
			require.NoError(t, err)
			require.Equal(t, sig, sig2)

			prevSig = sig
		})
	}
}
