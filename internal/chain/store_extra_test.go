package chain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBytesToRound(t *testing.T) {
	require.Equal(t, uint64(0), BytesToRound([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}))
	require.Equal(t, uint64(1), BytesToRound([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}))
	require.Equal(t, uint64(184348345343), BytesToRound([]byte{0x00, 0x00, 0x00, 0x2a, 0xec, 0x04, 0x83, 0xff}))
	require.Equal(t, uint64(0xA1B2C3D4E5F6A7B8), BytesToRound([]byte{0xA1, 0xB2, 0xC3, 0xD4, 0xE5, 0xF6, 0xA7, 0xB8}))
}

func TestRoundToBytesRoundTrip(t *testing.T) {
	for _, r := range []uint64{0, 1, 42, 184348345343, 0xA1B2C3D4E5F6A7B8, ^uint64(0)} {
		require.Equal(t, r, BytesToRound(RoundToBytes(r)), "round-trip for %d", r)
		require.Len(t, RoundToBytes(r), 8)
	}
}

func TestGenesisBeacon(t *testing.T) {
	seed := []byte{0xde, 0xad, 0xbe, 0xef}
	b := GenesisBeacon(seed)
	require.NotNil(t, b)
	require.Equal(t, uint64(0), b.Round)
	require.Equal(t, seed, []byte(b.Signature))
	require.Nil(t, b.PreviousSig)

	// A nil seed is allowed and yields a genesis beacon with nil signature.
	empty := GenesisBeacon(nil)
	require.Equal(t, uint64(0), empty.Round)
	require.Nil(t, empty.Signature)
}
