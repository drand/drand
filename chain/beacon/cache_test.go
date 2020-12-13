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

func generatePartial(idx int, round uint64) *drand.PartialBeaconPacket {
	sh := &share.PriShare{
		I: idx,
		V: fakeKey.Key,
	}
	msg := chain.Message(round)
	sig, _ := key.Scheme.Sign(sh, msg)
	return &drand.PartialBeaconPacket{
		Round:      round,
		PartialSig: sig,
	}
}

func TestCacheRound(t *testing.T) {
	id := "thisismyid"
	var round uint64 = 64
	msg := chain.Message(round)
	partial := generatePartial(1, round)
	p2 := generatePartial(2, round)
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

	id := roundID(round)
	p1 := generatePartial(1, round)
	cache.Append(p1)
	require.Equal(t, 1, len(cache.rcvd))
	require.Equal(t, 1, cache.GetRoundCache(round).Len())
	// duplicate entry shouldn't change anything
	cache.Append(p1)
	require.Equal(t, 1, len(cache.rcvd))
	require.Equal(t, 1, len(cache.rcvd[1]))
	require.Equal(t, 1, cache.GetRoundCache(round).Len())
	require.Contains(t, cache.rcvd[1], id)

	newID := roundID(round)
	p1bis := generatePartial(1, round)
	cache.Append(p1bis)
	require.Contains(t, cache.rcvd[1], newID)
	// only one signer pushed things, so there should always be 1 sig
	require.Equal(t, 1, len(cache.rounds))

	// insert some previous rounds and then flush
	toFlush := 20
	for i := 1; i <= toFlush; i++ {
		p := generatePartial(i+1, round-uint64(i))
		cache.Append(p)
	}
	require.Equal(t, len(cache.rounds), 21)

	// flush all new rounds just inserted
	cache.FlushRounds(round - 1)
	require.Equal(t, len(cache.rounds), 1)
	for i := 1; i <= toFlush; i++ {
		require.Nil(t, cache.rcvd[i+1], "failed for signer %d", i+1)
	}
}
