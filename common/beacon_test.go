package common

import (
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
