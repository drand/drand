package chain

import (
	"math"
	"testing"
	"time"

	clock "github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
)

func TestTimeOverflow(t *testing.T) {
	start := time.Date(2020, 01, 01, 0, 0, 0, 0, time.UTC).Unix()
	period := time.Second

	smallRound := TimeOfRound(period, start, 1024)
	largeRound := TimeOfRound(period, start, math.MaxInt32)

	if largeRound < smallRound {
		t.Fatal("future rounds should not allow previous times.")
	}

	overflowRound := TimeOfRound(period, start, math.MaxInt64)
	if overflowRound < smallRound {
		t.Fatal("future rounds should not allow previous times.")
	}
}

func TestChainNextRound(t *testing.T) {
	clk := clock.NewFakeClock()
	// start in one second
	genesis := clk.Now().Add(1 * time.Second).Unix()
	period := 2 * time.Second
	// move to genesis round
	// genesis block is fixed, first round happens at genesis time
	clk.Advance(1 * time.Second)
	round, roundTime := NextRound(clk.Now().Unix(), period, genesis)
	require.Equal(t, uint64(2), round)
	expTime := genesis + int64(period.Seconds())
	require.Equal(t, expTime, roundTime)

	time1 := TimeOfRound(period, genesis, 2)
	require.Equal(t, expTime, time1)

	// move to one second
	clk.Advance(1 * time.Second)
	nround, nroundTime := NextRound(clk.Now().Unix(), period, genesis)
	require.Equal(t, round, nround)
	require.Equal(t, roundTime, nroundTime)

	// move to next round
	clk.Advance(1 * time.Second)
	round, roundTime = NextRound(clk.Now().Unix(), period, genesis)
	require.Equal(t, round, uint64(3))
	expTime2 := genesis + int64(period.Seconds())*2
	require.Equal(t, expTime2, roundTime)

	time2 := TimeOfRound(period, genesis, 3)
	require.Equal(t, expTime2, time2)
}
