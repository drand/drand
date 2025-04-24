package common

import (
	"fmt"
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

	overflowRound := TimeOfRound(period, start, math.MaxUint64>>3)
	if overflowRound != TimeOfRoundErrorValue {
		t.Fatal("overflow shoud return error.")
	}

	overflowRound2 := TimeOfRound(period+2*time.Second, start, (math.MaxUint64>>3)-1)
	if overflowRound2 != TimeOfRoundErrorValue {
		t.Fatal("overflow shoud return error.")
	}

	negativePeriod := TimeOfRound(-1, start, math.MaxUint64)
	if negativePeriod != TimeOfRoundErrorValue {
		t.Fatal("negative period shoud return error.")
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

func TestRoundAt(t *testing.T) {
	period := 30 * time.Second
	genesis := time.Now().Unix()
	for i := 0; i < 10; i++ {
		round := uint64(i)
		timeRound := TimeOfRound(period, genesis, round)
		require.Equal(t, round, CurrentRound(timeRound, period, genesis))
	}

	// Test sub-second period
	periodMillis := 500 * time.Millisecond
	nowNanos := time.Now().UnixNano()
	genesisNano := (nowNanos / int64(periodMillis)) * int64(periodMillis) // Align genesis to a period boundary near now
	for i := 0; i < 10; i++ {
		round := uint64(i)
		timeRoundNano := TimeOfRoundNano(periodMillis, genesisNano, round)
		calculatedRound := CurrentRoundNano(timeRoundNano, periodMillis, genesisNano)
		require.Equal(t, round, calculatedRound, fmt.Sprintf("Sub-second: Round %d failed. Time: %d, Genesis: %d, Period: %d", round, timeRoundNano, genesisNano, periodMillis))
		// Check edge case just before next round
		if i > 0 {
			calculatedRoundEdge := CurrentRoundNano(timeRoundNano-1, periodMillis, genesisNano)
			require.Equal(t, round-1, calculatedRoundEdge, fmt.Sprintf("Sub-second Edge: Round %d failed. Time: %d, Genesis: %d, Period: %d", round, timeRoundNano-1, genesisNano, periodMillis))
		}
	}

	// Test fractional second period
	periodFrac := 1250 * time.Millisecond // 1.25s
	nowNanosFrac := time.Now().UnixNano()
	genesisNanoFrac := (nowNanosFrac / int64(periodFrac)) * int64(periodFrac) // Align genesis
	for i := 0; i < 10; i++ {
		round := uint64(i)
		timeRoundNano := TimeOfRoundNano(periodFrac, genesisNanoFrac, round)
		calculatedRound := CurrentRoundNano(timeRoundNano, periodFrac, genesisNanoFrac)
		require.Equal(t, round, calculatedRound, fmt.Sprintf("Fractional: Round %d failed. Time: %d, Genesis: %d, Period: %d", round, timeRoundNano, genesisNanoFrac, periodFrac))
		// Check edge case just before next round
		if i > 0 {
			calculatedRoundEdge := CurrentRoundNano(timeRoundNano-1, periodFrac, genesisNanoFrac)
			require.Equal(t, round-1, calculatedRoundEdge, fmt.Sprintf("Fractional Edge: Round %d failed. Time: %d, Genesis: %d, Period: %d", round, timeRoundNano-1, genesisNanoFrac, periodFrac))
		}
	}
}

func TestNextRound(t *testing.T) {
	period := 30 * time.Second
	genesis := time.Now().Unix()
	r, _ := NextRound(genesis+1, period, genesis)
	require.Equal(t, uint64(1), r)
	r, _ = NextRound(genesis+int64(period/time.Second)+1, period, genesis)
	require.Equal(t, uint64(2), r)

	// Test sub-second period
	periodMillis := 500 * time.Millisecond
	nowNanosSub := time.Now().UnixNano()
	genesisNanoSub := (nowNanosSub / int64(periodMillis)) * int64(periodMillis) // Align genesis
	// Time just after genesis
	r, tNano := NextRoundNano(genesisNanoSub+1, periodMillis, genesisNanoSub)
	require.Equal(t, uint64(1), r)
	require.Equal(t, genesisNanoSub+int64(periodMillis), tNano)
	// Time just after round 1
	r, tNano = NextRoundNano(genesisNanoSub+int64(periodMillis)+1, periodMillis, genesisNanoSub)
	require.Equal(t, uint64(2), r)
	require.Equal(t, genesisNanoSub+2*int64(periodMillis), tNano)

	// Test fractional second period
	periodFrac := 1250 * time.Millisecond // 1.25s
	nowNanosFrac := time.Now().UnixNano()
	genesisNanoFrac := (nowNanosFrac / int64(periodFrac)) * int64(periodFrac) // Align genesis
	// Time just after genesis
	r, tNano = NextRoundNano(genesisNanoFrac+1, periodFrac, genesisNanoFrac)
	require.Equal(t, uint64(1), r)
	require.Equal(t, genesisNanoFrac+int64(periodFrac), tNano)
	// Time just after round 1
	r, tNano = NextRoundNano(genesisNanoFrac+int64(periodFrac)+1, periodFrac, genesisNanoFrac)
	require.Equal(t, uint64(2), r)
	require.Equal(t, genesisNanoFrac+2*int64(periodFrac), tNano)
}

// Helper functions assuming nanosecond precision (to be potentially moved to common/time.go)
func TimeOfRoundNano(period time.Duration, genesisNano int64, round uint64) int64 {
	return genesisNano + int64(round)*int64(period)
}

func CurrentRoundNano(nowNano int64, period time.Duration, genesisNano int64) uint64 {
	if nowNano < genesisNano {
		return 0
	}
	diff := nowNano - genesisNano
	if diff < 0 {
		// Should not happen if nowNano >= genesisNano, but safety check
		return 0
	}
	// Use time.Duration division
	return uint64(time.Duration(diff) / period)
}

func NextRoundNano(nowNano int64, period time.Duration, genesisNano int64) (uint64, int64) {
	var nextRound uint64
	var nextTime int64

	if nowNano < genesisNano {
		// If current time is before genesis, next round is 1, time is genesis + period
		nextRound = 1
		nextTime = genesisNano + int64(period)
	} else {
		round := CurrentRoundNano(nowNano, period, genesisNano)
		nextRound = round + 1
		nextTime = TimeOfRoundNano(period, genesisNano, nextRound)
	}
	return nextRound, nextTime
}
