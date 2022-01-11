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
