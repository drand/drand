package test

import (
	"context"
	"crypto/sha256"
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

// TestBigEndianRoundSerialization verifies RoundToBytes and BytesToRound
// produce consistent big-endian byte sequences.
func TestBigEndianRoundSerialization(t *testing.T) {
	testRounds := []struct {
		name  string
		round uint64
		bytes []byte
	}{
		{
			name:  "zero",
			round: 0,
			bytes: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name:  "one",
			round: 1,
			bytes: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
		},
		{
			name:  "small_value",
			round: 12345,
			bytes: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x30, 0x39},
		},
		{
			name:  "large_value",
			round: 0x1234567890ABCDEF,
			bytes: []byte{0x12, 0x34, 0x56, 0x78, 0x90, 0xAB, 0xCD, 0xEF},
		},
		{
			name:  "max_value",
			round: 0xFFFFFFFFFFFFFFFF,
			bytes: []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
		},
	}

	for _, tc := range testRounds {
		t.Run(tc.name, func(t *testing.T) {
			bytes := chain.RoundToBytes(tc.round)
			require.Equal(t, tc.bytes, bytes)

			round := chain.BytesToRound(tc.bytes)
			require.Equal(t, tc.round, round)

			round2 := chain.BytesToRound(chain.RoundToBytes(tc.round))
			require.Equal(t, tc.round, round2)
		})
	}
}

// TestBeaconStorageCrossEndianness verifies RoundToBytes produces expected
// big-endian byte sequences for various round values.
func TestBeaconStorageCrossEndianness(t *testing.T) {
	testRounds := []struct {
		round        uint64
		bigEndianHex string
		description  string
	}{
		{
			round:        0,
			bigEndianHex: "0000000000000000",
			description:  "zero",
		},
		{
			round:        1,
			bigEndianHex: "0000000000000001",
			description:  "one",
		},
		{
			round:        255, // 0xFF
			bigEndianHex: "00000000000000ff",
			description:  "single_byte_max",
		},
		{
			round:        256, // 0x0100
			bigEndianHex: "0000000000000100",
			description:  "two_bytes_min",
		},
		{
			round:        65535, // 0xFFFF
			bigEndianHex: "000000000000ffff",
			description:  "two_bytes_max",
		},
		{
			round:        65536, // 0x00010000
			bigEndianHex: "0000000000010000",
			description:  "three_bytes_min",
		},
		{
			round:        0x1234567890ABCDEF,
			bigEndianHex: "1234567890abcdef",
			description:  "large_value_distinct_bytes",
		},
	}

	for _, tc := range testRounds {
		t.Run(tc.description, func(t *testing.T) {
			roundBytes := chain.RoundToBytes(tc.round)
			expectedBytes := make([]byte, 8)
			for i := 0; i < 8; i++ {
				var b byte
				fmt.Sscanf(tc.bigEndianHex[i*2:i*2+2], "%02x", &b)
				expectedBytes[i] = b
			}
			require.Equal(t, expectedBytes, roundBytes)

			recoveredRound := chain.BytesToRound(roundBytes)
			require.Equal(t, tc.round, recoveredRound)

			if tc.round != 0 && tc.round != 0xFFFFFFFFFFFFFFFF {
				littleEndianBytes := make([]byte, 8)
				binary.LittleEndian.PutUint64(littleEndianBytes, tc.round)
				if tc.round > 0xFF {
					require.NotEqual(t, roundBytes, littleEndianBytes)
				}
			}
		})
	}
}

// TestSignatureVerificationWithBigEndianDigest verifies BLS signatures work
// with digests computed using drand's big-endian round serialization.
func TestSignatureVerificationWithBigEndianDigest(t *testing.T) {
	scheme := crypto.NewPedersenBLSChained()
	priv := scheme.KeyGroup.Scalar().Pick(random.New())
	pub := scheme.KeyGroup.Point().Mul(priv, nil)

	testRounds := []uint64{
		0,
		1,
		0x00000000000000FF,
		0x000000000000FF00,
		0x1234567890ABCDEF,
		0xFFFFFFFFFFFFFFFF,
	}

	for _, round := range testRounds {
		t.Run(fmt.Sprintf("round_%d", round), func(t *testing.T) {
			prevSig := []byte("previous_signature")
			digest := createBeaconDigest(prevSig, round)

			sig, err := scheme.AuthScheme.Sign(priv, digest)
			require.NoError(t, err)

			err = scheme.AuthScheme.Verify(pub, digest, sig)
			require.NoError(t, err)

			sig2, err := scheme.AuthScheme.Sign(priv, digest)
			require.NoError(t, err)
			require.Equal(t, sig, sig2)
		})
	}
}

// TestBeaconDigestBigEndianConvention verifies the beacon digest uses
// big-endian serialization for round numbers.
func TestBeaconDigestBigEndianConvention(t *testing.T) {
	round := uint64(0x1234567890ABCDEF)
	prevSig := []byte("test_previous_signature")

	digest1 := createBeaconDigest(prevSig, round)
	require.Len(t, digest1, 32)

	expectedRoundBytes := []byte{0x12, 0x34, 0x56, 0x78, 0x90, 0xAB, 0xCD, 0xEF}
	roundBytes := chain.RoundToBytes(round)
	require.Equal(t, expectedRoundBytes, roundBytes)

	littleEndianRoundBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(littleEndianRoundBytes, round)
	require.NotEqual(t, expectedRoundBytes, littleEndianRoundBytes)

	digest2 := createBeaconDigest(prevSig, round)
	require.Equal(t, digest1, digest2)

	digest3 := createBeaconDigest(prevSig, round+1)
	require.NotEqual(t, digest1, digest3)
}

// createBeaconDigest creates a beacon digest using drand's convention.
// Matches crypto/schemes.go: SHA256(prevSig || big-endian(round))
func createBeaconDigest(prevSig []byte, round uint64) []byte {
	hash := sha256.New()
	if len(prevSig) > 0 {
		hash.Write(prevSig)
	}
	roundBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(roundBytes, round)
	hash.Write(roundBytes)
	return hash.Sum(nil)
}

// TestCrossEndiannessBeaconExchange verifies beacons can be stored and
// retrieved correctly using big-endian round keys.
func TestCrossEndiannessBeaconExchange(t *testing.T) {
	ctx, _, _ := context2.PrevSignatureMattersOnContext(t, context.Background())
	dir := t.TempDir()
	l := testlogger.New(t)

	store, err := boltdb.NewBoltStore(ctx, l, dir)
	require.NoError(t, err)

	genesisBeacon := &common.Beacon{
		Round:     0,
		Signature: common.HexBytes([]byte("genesis_sig")),
	}
	require.NoError(t, store.Put(ctx, genesisBeacon))

	round := uint64(2)
	prevBeacon := &common.Beacon{
		Round:       1,
		PreviousSig: genesisBeacon.Signature,
		Signature:   common.HexBytes([]byte("prev_sig")),
	}
	require.NoError(t, store.Put(ctx, prevBeacon))

	beacon := &common.Beacon{
		Round:       round,
		PreviousSig: prevBeacon.Signature,
		Signature:   common.HexBytes([]byte("signature")),
	}
	require.NoError(t, store.Put(ctx, beacon))
	require.NoError(t, store.Close())

	store2, err := boltdb.NewBoltStore(ctx, l, dir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, store2.Close())
	}()

	retrieved, err := store2.Get(ctx, round)
	require.NoError(t, err)
	require.True(t, beacon.Equal(retrieved))
	require.Equal(t, round, retrieved.Round)

	roundBytes := chain.RoundToBytes(round)
	expectedBytes := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02}
	require.Equal(t, expectedBytes, roundBytes)
}
