package common

import (
	"encoding/hex"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/crypto"
)

func TestIsDefaultBeaconID(t *testing.T) {
	require.True(t, IsDefaultBeaconID(""))
	require.True(t, IsDefaultBeaconID(DefaultBeaconID))
	require.False(t, IsDefaultBeaconID("beacon_5s"))
}

func TestGetCanonicalBeaconID(t *testing.T) {
	require.Equal(t, DefaultBeaconID, GetCanonicalBeaconID(""))
	require.Equal(t, DefaultBeaconID, GetCanonicalBeaconID(DefaultBeaconID))
	require.Equal(t, "beacon_5s", GetCanonicalBeaconID("beacon_5s"))
}

func TestBeaconEqual(t *testing.T) {
	b := &Beacon{
		PreviousSig: HexBytes{0x01, 0x02},
		Round:       7,
		Signature:   HexBytes{0x03, 0x04},
	}
	same := &Beacon{
		PreviousSig: HexBytes{0x01, 0x02},
		Round:       7,
		Signature:   HexBytes{0x03, 0x04},
	}
	require.True(t, b.Equal(same))

	diffRound := *same
	diffRound.Round = 8
	require.False(t, b.Equal(&diffRound))

	diffSig := *same
	diffSig.Signature = HexBytes{0xff}
	require.False(t, b.Equal(&diffSig))

	diffPrev := *same
	diffPrev.PreviousSig = HexBytes{0xff}
	require.False(t, b.Equal(&diffPrev))
}

func TestBeaconMarshalUnmarshalRoundTrip(t *testing.T) {
	b := &Beacon{
		PreviousSig: HexBytes{0xde, 0xad, 0xbe, 0xef},
		Round:       42,
		Signature:   HexBytes{0xca, 0xfe},
	}
	buff, err := b.Marshal()
	require.NoError(t, err)

	got := new(Beacon)
	require.NoError(t, got.Unmarshal(buff))
	require.True(t, b.Equal(got))

	// no previous_signature should be omitted in JSON
	noPrev := &Beacon{Round: 1, Signature: HexBytes{0x01}}
	buff, err = noPrev.Marshal()
	require.NoError(t, err)
	require.NotContains(t, string(buff), "previous_signature")
}

func TestBeaconUnmarshalInvalid(t *testing.T) {
	b := new(Beacon)
	require.Error(t, b.Unmarshal([]byte("not json")))
	// invalid hex inside a HexBytes field
	require.Error(t, b.Unmarshal([]byte(`{"round":1,"signature":"zz"}`)))
}

func TestBeaconAccessors(t *testing.T) {
	sig := HexBytes{0x01, 0x02, 0x03}
	prev := HexBytes{0x04, 0x05}
	b := &Beacon{PreviousSig: prev, Round: 9, Signature: sig}

	require.Equal(t, uint64(9), b.GetRound())
	require.Equal(t, []byte(sig), b.GetSignature())
	require.Equal(t, []byte(prev), b.GetPreviousSignature())

	// nil previous signature returns nil
	bn := &Beacon{Round: 1, Signature: sig}
	require.Nil(t, bn.GetPreviousSignature())
}

func TestBeaconRandomness(t *testing.T) {
	sig := HexBytes{0xaa, 0xbb, 0xcc}
	b := &Beacon{Round: 1, Signature: sig}
	want := crypto.RandomnessFromSignature(sig)
	require.Equal(t, want, b.Randomness())
	require.Equal(t, want, b.GetRandomness())
	require.Len(t, b.Randomness(), 32)
}

func TestBeaconString(t *testing.T) {
	b := &Beacon{
		PreviousSig: HexBytes{0x11, 0x22, 0x33, 0x44},
		Round:       3,
		Signature:   HexBytes{0xaa, 0xbb, 0xcc, 0xdd},
	}
	s := b.String()
	require.Contains(t, s, "round: 3")
	// only first 3 bytes are shown
	require.Contains(t, s, "aabbcc")
	require.Contains(t, s, "112233")
}

func TestHexBytesMarshalJSON(t *testing.T) {
	h := HexBytes{0x01, 0x02, 0x0f}
	out, err := h.MarshalJSON()
	require.NoError(t, err)
	require.Equal(t, `"01020f"`, string(out))

	// round-trip through standard json
	var back HexBytes
	require.NoError(t, json.Unmarshal(out, &back))
	require.Equal(t, h, back)
}

func TestHexBytesUnmarshalJSONInvalid(t *testing.T) {
	var h HexBytes
	// not a JSON string
	require.Error(t, h.UnmarshalJSON([]byte(`123`)))
	// valid JSON string but invalid hex
	require.Error(t, h.UnmarshalJSON([]byte(`"zz"`)))
	// odd length hex
	require.Error(t, h.UnmarshalJSON([]byte(`"abc"`)))
}

func TestHexBytesString(t *testing.T) {
	h := HexBytes{0xde, 0xad}
	require.Equal(t, "dead", h.String())
	require.Equal(t, hex.EncodeToString(h), h.String())

	empty := HexBytes{}
	require.Equal(t, "", empty.String())
}

func TestTimeOfRoundZeroRound(t *testing.T) {
	genesis := int64(1000)
	require.Equal(t, genesis, TimeOfRound(time.Second, genesis, 0))
}

func TestTimeOfRoundBasic(t *testing.T) {
	genesis := int64(1000)
	period := 30 * time.Second
	// round 1 == genesis
	require.Equal(t, genesis, TimeOfRound(period, genesis, 1))
	// round 2 == genesis + period
	require.Equal(t, genesis+30, TimeOfRound(period, genesis, 2))
	require.Equal(t, genesis+60, TimeOfRound(period, genesis, 3))
}

func TestTimeOfRoundErrors(t *testing.T) {
	genesis := int64(1000)
	require.Equal(t, int64(TimeOfRoundErrorValue), TimeOfRound(-time.Second, genesis, 5))
	// huge round overflows
	require.Equal(t, int64(TimeOfRoundErrorValue), TimeOfRound(time.Second, genesis, math.MaxUint64>>2))
	// a large-but-valid genesis that still trips the additive max-buffer guard:
	// genesis sits just below the error threshold so that adding the delta crosses it
	// without wrapping the int64.
	bigGenesis := int64(TimeOfRoundErrorValue) - 5
	require.Equal(t, int64(TimeOfRoundErrorValue), TimeOfRound(time.Second, bigGenesis, 100))
}

func TestNextRoundBeforeGenesis(t *testing.T) {
	genesis := int64(1000)
	round, ts := NextRound(500, 30*time.Second, genesis)
	require.Equal(t, uint64(1), round)
	require.Equal(t, genesis, ts)
}

func TestNextRoundAndCurrentRound(t *testing.T) {
	genesis := int64(1000)
	period := 10 * time.Second

	// exactly at genesis -> next round is 2
	round, ts := NextRound(genesis, period, genesis)
	require.Equal(t, uint64(2), round)
	require.Equal(t, genesis+10, ts)

	// CurrentRound at genesis -> 1
	require.Equal(t, uint64(1), CurrentRound(genesis, period, genesis))

	// before genesis CurrentRound is the round NextRound returns (<= 1)
	require.Equal(t, uint64(1), CurrentRound(500, period, genesis))

	// somewhere in the third period
	now := genesis + 25
	round, _ = NextRound(now, period, genesis)
	require.Equal(t, uint64(4), round)
	require.Equal(t, round-1, CurrentRound(now, period, genesis))
}

func TestVersionToProto(t *testing.T) {
	v := Version{Major: 2, Minor: 1, Patch: 6}
	p := v.ToProto()
	require.Equal(t, uint32(2), p.Major)
	require.Equal(t, uint32(1), p.Minor)
	require.Equal(t, uint32(6), p.Patch)
}

func TestGetAppVersion(t *testing.T) {
	v := GetAppVersion()
	require.Equal(t, uint32(2), v.Major)
	require.NotEmpty(t, v.String())
}
