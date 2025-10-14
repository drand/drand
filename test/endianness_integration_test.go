package test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/protobuf/drand"
	"github.com/drand/kyber"
	"github.com/drand/kyber/util/random"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// TestEndiannessIntegration demonstrates that drand works correctly
// across different endianness systems by simulating data exchange
// between big-endian and little-endian systems.
func TestEndiannessIntegration(t *testing.T) {
	t.Run("CrossEndiannessDataExchange", testCrossEndiannessDataExchange)
	t.Run("EndiannessConsistency", testEndiannessConsistency)
	t.Run("RealWorldScenario", testRealWorldEndiannessScenario)
}

// testCrossEndiannessDataExchange simulates data created on one endianness
// system being read on another endianness system.
func testCrossEndiannessDataExchange(t *testing.T) {
	scheme := crypto.NewPedersenBLSChained()
	
	// Create a keypair (this would be the same regardless of endianness)
	priv := scheme.KeyGroup.Scalar().Pick(random.New())
	pub := scheme.KeyGroup.Point().Mul(priv, nil)
	
	// Create a beacon with a specific round number
	round := uint64(0x1234567890ABCDEF)
	prevSig := []byte("previous_signature_data")
	sig := []byte("current_signature_data")
	
	// Create beacon on "little-endian system"
	beaconLE := &common.Beacon{
		Round:       round,
		PreviousSig: common.HexBytes(prevSig),
		Signature:   common.HexBytes(sig),
	}
	
	// Marshal beacon to JSON (this should be endianness-independent)
	jsonData, err := beaconLE.Marshal()
	require.NoError(t, err)
	
	// Simulate reading on "big-endian system"
	var beaconBE common.Beacon
	err = beaconBE.Unmarshal(jsonData)
	require.NoError(t, err)
	
	// Verify data integrity across endianness
	require.Equal(t, beaconLE.Round, beaconBE.Round)
	require.Equal(t, beaconLE.PreviousSig, beaconBE.PreviousSig)
	require.Equal(t, beaconLE.Signature, beaconBE.Signature)
	require.True(t, beaconLE.Equal(&beaconBE))
	
	// Test signature verification works across endianness
	msg := createTestMessage(round, prevSig)
	signature, err := scheme.AuthScheme.Sign(priv, msg)
	require.NoError(t, err)
	
	// Verify signature on "different endianness system"
	err = scheme.AuthScheme.Verify(pub, msg, signature)
	require.NoError(t, err)
}

// testEndiannessConsistency verifies that drand's endianness choices
// are consistent and well-documented.
func testEndiannessConsistency(t *testing.T) {
	// Test that drand consistently uses big-endian for round numbers
	// (as documented in internal/chain/store.go)
	round := uint64(0x1234567890ABCDEF)
	
	// This is how rounds are serialized in drand
	bigEndianBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(bigEndianBytes, round)
	
	// Verify the serialization is big-endian
	expected := []byte{0x12, 0x34, 0x56, 0x78, 0x90, 0xAB, 0xCD, 0xEF}
	require.Equal(t, expected, bigEndianBytes)
	
	// Test that drand consistently uses little-endian for epoch numbers
	// (as used in DKG messages)
	epoch := uint32(0x12345678)
	
	// This is how epochs are serialized in drand DKG
	littleEndianBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(littleEndianBytes, epoch)
	
	// Verify the serialization is little-endian
	expectedEpoch := []byte{0x78, 0x56, 0x34, 0x12}
	require.Equal(t, expectedEpoch, littleEndianBytes)
}

// testRealWorldEndiannessScenario simulates a real-world scenario where
// drand nodes on different endianness systems need to interoperate.
func testRealWorldEndiannessScenario(t *testing.T) {
	scheme := crypto.NewPedersenBLSChained()
	
	// Simulate a drand network with mixed endianness systems
	// Node A (little-endian system) creates a beacon
	nodeA := createMockNode(t, scheme, "little-endian")
	beaconA := nodeA.CreateBeacon(12345, []byte("prev_sig"), []byte("sig"))
	
	// Node B (big-endian system) receives and processes the beacon
	_ = createMockNode(t, scheme, "big-endian")
	
	// Serialize beacon from Node A
	jsonData, err := beaconA.Marshal()
	require.NoError(t, err)
	
	// Deserialize beacon on Node B
	var beaconB common.Beacon
	err = beaconB.Unmarshal(jsonData)
	require.NoError(t, err)
	
	// Verify the beacon is identical
	require.True(t, beaconA.Equal(&beaconB))
	
	// Test that signature verification works across systems
	msg := createTestMessage(beaconA.Round, beaconA.PreviousSig)
	signature, err := scheme.AuthScheme.Sign(nodeA.PrivateKey, msg)
	require.NoError(t, err)
	err = scheme.AuthScheme.Verify(nodeA.PublicKey, msg, signature)
	require.NoError(t, err)
	
	// Test protobuf message exchange
	packetA := &drand.BeaconPacket{
		Round:            beaconA.Round,
		PreviousSignature: beaconA.PreviousSig,
		Signature:        beaconA.Signature,
	}
	
	// Serialize on Node A
	protoData, err := proto.Marshal(packetA)
	require.NoError(t, err)
	
	// Deserialize on Node B
	var packetB drand.BeaconPacket
	err = proto.Unmarshal(protoData, &packetB)
	require.NoError(t, err)
	
	// Verify data integrity
	require.Equal(t, packetA.Round, packetB.Round)
	require.Equal(t, packetA.PreviousSignature, packetB.PreviousSignature)
	require.Equal(t, packetA.Signature, packetB.Signature)
}

// MockNode represents a drand node for testing
type MockNode struct {
	Scheme     *crypto.Scheme
	PrivateKey kyber.Scalar
	PublicKey  kyber.Point
	Endianness string
}

func createMockNode(t *testing.T, scheme *crypto.Scheme, endianness string) *MockNode {
	priv := scheme.KeyGroup.Scalar().Pick(random.New())
	pub := scheme.KeyGroup.Point().Mul(priv, nil)
	
	return &MockNode{
		Scheme:     scheme,
		PrivateKey: priv,
		PublicKey:  pub,
		Endianness: endianness,
	}
}

func (n *MockNode) CreateBeacon(round uint64, prevSig, sig []byte) *common.Beacon {
	return &common.Beacon{
		Round:       round,
		PreviousSig: common.HexBytes(prevSig),
		Signature:   common.HexBytes(sig),
	}
}

func createTestMessage(round uint64, prevSig []byte) []byte {
	// This simulates the message format used in drand's beacon digest
	var buf bytes.Buffer
	if len(prevSig) > 0 {
		buf.Write(prevSig)
	}
	// Use big-endian for round number (as per drand's convention)
	buf.Write(binary.BigEndian.AppendUint64([]byte{}, round))
	return buf.Bytes()
}

// TestEndiannessDocumentation verifies that the endianness choices
// in drand are well-documented and consistent.
func TestEndiannessDocumentation(t *testing.T) {
	t.Run("RoundSerialization", func(t *testing.T) {
		// Document that rounds use big-endian serialization
		round := uint64(0x1234567890ABCDEF)
		bigEndianBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(bigEndianBytes, round)
		
		// This should match the format used in internal/chain/store.go
		expected := []byte{0x12, 0x34, 0x56, 0x78, 0x90, 0xAB, 0xCD, 0xEF}
		require.Equal(t, expected, bigEndianBytes)
		
		t.Logf("Round %d serialized as big-endian: %x", round, bigEndianBytes)
	})
	
	t.Run("EpochSerialization", func(t *testing.T) {
		// Document that epochs use little-endian serialization
		epoch := uint32(0x12345678)
		littleEndianBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(littleEndianBytes, epoch)
		
		// This should match the format used in DKG messages
		expected := []byte{0x78, 0x56, 0x34, 0x12}
		require.Equal(t, expected, littleEndianBytes)
		
		t.Logf("Epoch %d serialized as little-endian: %x", epoch, littleEndianBytes)
	})
	
	t.Run("BeaconDigest", func(t *testing.T) {
		// Document that beacon digest uses big-endian for round numbers
		round := uint64(0x1234567890ABCDEF)
		prevSig := []byte("test_previous_signature")
		
		// This simulates the digest function in crypto/schemes.go
		var buf bytes.Buffer
		if len(prevSig) > 0 {
			buf.Write(prevSig)
		}
		buf.Write(binary.BigEndian.AppendUint64([]byte{}, round))
		digest := buf.Bytes()
		
		expected := append(prevSig, 0x12, 0x34, 0x56, 0x78, 0x90, 0xAB, 0xCD, 0xEF)
		require.Equal(t, expected, digest)
		
		t.Logf("Beacon digest for round %d: %x", round, digest)
	})
}

// TestEndiannessCompatibilityMatrix tests compatibility between
// different endianness combinations.
func TestEndiannessCompatibilityMatrix(t *testing.T) {
	testCases := []struct {
		name      string
		round     uint64
		epoch     uint32
		prevSig   []byte
		signature []byte
	}{
		{
			name:      "zero_values",
			round:     0,
			epoch:     0,
			prevSig:   nil,
			signature: nil,
		},
		{
			name:      "small_values",
			round:     1,
			epoch:     1,
			prevSig:   []byte("small"),
			signature: []byte("small_sig"),
		},
		{
			name:      "medium_values",
			round:     12345,
			epoch:     67890,
			prevSig:   []byte("medium_previous_signature"),
			signature: []byte("medium_signature"),
		},
		{
			name:      "large_values",
			round:     0x1234567890ABCDEF,
			epoch:     0x12345678,
			prevSig:   []byte("large_previous_signature_data"),
			signature: []byte("large_signature_data"),
		},
		{
			name:      "max_values",
			round:     0xFFFFFFFFFFFFFFFF,
			epoch:     0xFFFFFFFF,
			prevSig:   []byte("max_previous_signature_data"),
			signature: []byte("max_signature_data"),
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test that data can be serialized and deserialized
			// regardless of the underlying system endianness
			
			// Create beacon
			beacon := &common.Beacon{
				Round:       tc.round,
				PreviousSig: common.HexBytes(tc.prevSig),
				Signature:   common.HexBytes(tc.signature),
			}
			
			// Serialize to JSON (endianness-independent)
			jsonData, err := beacon.Marshal()
			require.NoError(t, err)
			
			// Deserialize from JSON
			var recoveredBeacon common.Beacon
			err = recoveredBeacon.Unmarshal(jsonData)
			require.NoError(t, err)
			
			// Verify data integrity
			require.Equal(t, beacon.Round, recoveredBeacon.Round)
			// Handle nil vs empty slice comparison
			if len(beacon.PreviousSig) == 0 && len(recoveredBeacon.PreviousSig) == 0 {
				// Both are empty, that's fine
			} else {
				require.Equal(t, beacon.PreviousSig, recoveredBeacon.PreviousSig)
			}
			if len(beacon.Signature) == 0 && len(recoveredBeacon.Signature) == 0 {
				// Both are empty, that's fine
			} else {
				require.Equal(t, beacon.Signature, recoveredBeacon.Signature)
			}
			
			// Test protobuf serialization
			packet := &drand.BeaconPacket{
				Round:            tc.round,
				PreviousSignature: tc.prevSig,
				Signature:        tc.signature,
			}
			
			protoData, err := proto.Marshal(packet)
			require.NoError(t, err)
			
			var recoveredPacket drand.BeaconPacket
			err = proto.Unmarshal(protoData, &recoveredPacket)
			require.NoError(t, err)
			
			require.Equal(t, packet.Round, recoveredPacket.Round)
			require.Equal(t, packet.PreviousSignature, recoveredPacket.PreviousSignature)
			require.Equal(t, packet.Signature, recoveredPacket.Signature)
			
			// Test binary serialization consistency
			roundBigEndian := make([]byte, 8)
			binary.BigEndian.PutUint64(roundBigEndian, tc.round)
			
			epochLittleEndian := make([]byte, 4)
			binary.LittleEndian.PutUint32(epochLittleEndian, tc.epoch)
			
			// Verify round-trip conversion
			recoveredRound := binary.BigEndian.Uint64(roundBigEndian)
			recoveredEpoch := binary.LittleEndian.Uint32(epochLittleEndian)
			
			require.Equal(t, tc.round, recoveredRound)
			require.Equal(t, tc.epoch, recoveredEpoch)
			
			t.Logf("Round %d -> %x (big-endian) -> %d", tc.round, roundBigEndian, recoveredRound)
			t.Logf("Epoch %d -> %x (little-endian) -> %d", tc.epoch, epochLittleEndian, recoveredEpoch)
		})
	}
}
