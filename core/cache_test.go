package core

import (
	"testing"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/log"
	"github.com/stretchr/testify/require"
)

func TestBeaconCache(t *testing.T) {
	cache := newBeaconCache(log.NewLogger(log.LogDebug))
	require.Equal(t, len(cache.backend), DefaultBeaconCacheLength)

	b5 := &beacon.Beacon{
		Round: 5,
	}

	// store 1 beacon
	cache.StoreTemp(b5)
	b5p, ok := cache.GetBeacon(5)
	require.True(t, ok)
	require.Equal(t, b5, b5p)
	b6p, ok := cache.GetBeacon(6)
	require.False(t, ok)
	require.Nil(t, b6p)

	// store more than the cache
	of := 5 + 1
	for i := of; i < cache.max+of; i++ {
		cache.StoreTemp(&beacon.Beacon{
			Round: uint64(i),
		})
	}
	b5p, ok = cache.GetBeacon(5)
	require.False(t, ok)
	require.Nil(t, b5p)
}
