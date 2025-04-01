package common

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompareBeaconIDs(t *testing.T) {
	require.True(t, CompareBeaconIDs("", ""))
	require.True(t, CompareBeaconIDs("", DefaultBeaconID))
	require.True(t, CompareBeaconIDs(DefaultBeaconID, ""))
	require.True(t, CompareBeaconIDs(DefaultBeaconID, DefaultBeaconID))
	require.False(t, CompareBeaconIDs("beacon_5s", DefaultBeaconID))
	require.False(t, CompareBeaconIDs("beacon_5s", ""))
	require.False(t, CompareBeaconIDs(DefaultBeaconID, "beacon_5s"))
	require.False(t, CompareBeaconIDs("", "beacon_5s"))
}

func Test_shortSigStr(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sig  []byte
		want string
	}{
		"test with valid data":       {sig: []byte("some valid sig here"), want: "736f6d"},
		"test with short valid data": {sig: []byte("a"), want: "61"},
		"nil sig":                    {sig: nil, want: "nil"},
		"zero length sig":            {sig: []byte{}, want: ""},
	}
	for name, tt := range tests {
		name := name
		tt := tt
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := shortSigStr(tt.sig)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestJsonHexBytes(t *testing.T) {
	seed := `"f477d5c89f21a17c863a7f937c6a6d15859414d2be09cd448d4279af331c5d3e"`

	b := new(HexBytes)
	err := json.Unmarshal([]byte(seed), b)
	require.NoError(t, err)
	actual, err := hex.DecodeString(strings.Trim(seed, `"`))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("%v", &actual), fmt.Sprintf("%v", b))
}
