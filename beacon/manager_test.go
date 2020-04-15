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
		rm.NewPartialBeacon(&drand.PartialBeaconPacket{
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

	curr = 15
	prev = 13
	var realPrev uint64 = prev + 1
	// input before because we dont want to trigger a sync as soon as a partial
	// comes into because we dont know if the previous signature is valid or not
	rm.NewPartialBeacon(&drand.PartialBeaconPacket{
		PreviousRound: realPrev,
		Round:         curr,
		PreviousSig:   []byte("ain't nothing like the real thing"),
		PartialSig:    []byte("angers destroy your soul"),
	})
	partials = rm.NewRound(prev, curr)
	select {
	case <-rm.WaitSync():
	case <-time.After(100 * time.Millisecond):
		require.False(t, true, "too late")
	}

}
