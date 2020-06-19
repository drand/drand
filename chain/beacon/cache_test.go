package beacon

import (
	"testing"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
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
	require.Nil(t, cache.sigs[2])
}

func TestCachePartial(t *testing.T) {
	l := log.DefaultLogger()
	cache := newPartialCache(l)
	var round uint64 = 64
	prev := []byte("yesterday was another day")

	id := roundID(round, prev)
	p1 := generatePartial(1, round, prev)
	cache.Append(p1)
	require.Equal(t, 1, len(cache.rcvd))
	require.Equal(t, 1, cache.GetRoundCache(round, prev).Len())
	// duplicate entry shouldn't change anything
	cache.Append(p1)
	require.Equal(t, 1, len(cache.rcvd))
	require.Equal(t, 1, len(cache.rcvd[1]))
	require.Equal(t, 1, cache.GetRoundCache(round, prev).Len())
	require.Contains(t, cache.rcvd[1], id)

	// fill the cache with multiple previous signatures from the same signer
	for i := 0; i < MaxPartialsPerNode+10; i++ {
		newPrev := []byte{1, 9, 6, 9, byte(i)}
		newID := roundID(round, newPrev)
		p1bis := generatePartial(1, round, newPrev)
		cache.Append(p1bis)
		require.Contains(t, cache.rcvd[1], newID)
	}
	// the cache should have dropped the first ID entered by this node
	require.NotContains(t, cache.rcvd[1], id)
	// only one signer pushed things, so there should always be this number
	// maximum of partials
	require.Equal(t, MaxPartialsPerNode, len(cache.rounds))

	// insert some previous rounds and then flush
	toFlush := 20
	for i := 1; i <= toFlush; i++ {
		p := generatePartial(i+1, round-uint64(i), prev)
		cache.Append(p)
	}
	total := MaxPartialsPerNode + toFlush
	require.Equal(t, total, len(cache.rounds))

	// flush all new rounds just inserted
	cache.FlushRounds(round - 1)
	require.Equal(t, total-toFlush, len(cache.rounds))
	for i := 1; i <= toFlush; i++ {
		require.Nil(t, cache.rcvd[i+1], "failed for signer %d", i+1)
	}
}
