package beacon

import (
	"testing"
	"time"

	clock "github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
)

func TestChainNextRound(t *testing.T) {
	clock := clock.NewFakeClock()
	// start in one second
	genesis := clock.Now().Add(1 * time.Second).Unix()
	period := 2 * time.Second
	// move to genesis round
	// genesis block is fixed, first round happens at genesis time
	clock.Advance(1 * time.Second)
	round, roundTime := NextRound(clock.Now().Unix(), period, genesis)
	require.Equal(t, uint64(2), round)
	require.Equal(t, genesis+int64(period.Seconds()), roundTime)

	// move to one second
	clock.Advance(1 * time.Second)
	nround, nroundTime := NextRound(clock.Now().Unix(), period, genesis)
	require.Equal(t, round, nround)
	require.Equal(t, roundTime, nroundTime)

	// move to next round
	clock.Advance(1 * time.Second)
	round, roundTime = NextRound(clock.Now().Unix(), period, genesis)
	require.Equal(t, round, uint64(3))
	require.Equal(t, roundTime, genesis+int64(period.Seconds())*2)

}
