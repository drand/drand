package beacon

import (
	"testing"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/share"
	"github.com/stretchr/testify/require"
)

var fakeKey = key.NewKeyPair("127.0.0.1:8080")

func generatePartial(idx int, round uint64, prev []byte) *drand.PartialBeaconPacket {
	sh := &share.PriShare{
		I: idx,
		V: fakeKey.Key,
	}
	msg := chain.Message(round, prev)
	sig, _ := key.Scheme.Sign(sh, msg)
	return &drand.PartialBeaconPacket{
		Round:       round,
		PreviousSig: prev,
		PartialSig:  sig,
	}
}

func TestCacheRound(t *testing.T) {
	id := "thisismyid"
	var round uint64 = 64
	prev := []byte("yesterday was another day")
	msg := chain.Message(round, prev)
	partial := generatePartial(1, round, prev)
	p2 := generatePartial(2, round, prev)
	cache := newRoundCache(id, partial)
	require.True(t, cache.append(partial))
	require.False(t, cache.append(partial))
	require.Equal(t, 1, cache.Len())
	require.Equal(t, msg, cache.Msg())

	require.True(t, cache.append(p2))
	require.Equal(t, 2, cache.Len())
	require.Contains(t, cache.Partials(), partial.GetPartialSig())
	require.Contains(t, cache.Partials(), p2.GetPartialSig())
	cache.flushIndex(2)
	require.Equal(t, 1, cache.Len())
}
