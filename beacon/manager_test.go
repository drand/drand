package beacon

import (
	"testing"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/util/random"
	"github.com/stretchr/testify/require"
)

func TestManager(t *testing.T) {
	l := log.NewLogger(log.LogDebug)
	thr := 4
	s := key.Scheme
	rm := newRoundManager(l, thr, s)

	// first input a given round + prev round
	var curr uint64 = 10
	var prev uint64 = 8
	partials := rm.NewRound(prev, curr)
	for i := 0; i < thr; i++ {
		priv := key.KeyGroup.Scalar().Pick(random.New())
		privShare := &share.PriShare{
			V: priv,
			I: i,
		}
		sig, err := key.Scheme.Sign(privShare, []byte("le temps est cher, a l'amour comme a la guerre"))
		require.NoError(t, err)
		rm.NewBeacon(&drand.BeaconPacket{
			PreviousRound: prev,
			Round:         curr,
			PreviousSig:   []byte("le temps est cher"),
			PartialSig:    sig,
		})
	}
	for got := 0; got < thr; got++ {
		select {
		case <-partials:
		case <-time.After(100 * time.Millisecond):
			require.False(t, true, "too late")
		}
	}

	// check the "need sync" feature
	curr = 13
	prev = 10
	partials = rm.NewRound(prev, curr)
	rm.NewBeacon(&drand.BeaconPacket{
		PreviousRound: prev + 1,
		Round:         curr,
		PreviousSig:   []byte("l'ingratitude"),
		PartialSig:    []byte("est mere de tout vice"),
	})
	select {
	case <-rm.ProbablyNeedSync():
	case <-time.After(100 * time.Millisecond):
		require.False(t, true, "too late")
	}
}
