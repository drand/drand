package chain

import (
	"math"
	"time"
)

// TimeOfRound is returning the time the current round should happen
func TimeOfRound(period time.Duration, genesis int64, round uint64) int64 {
	if round == 0 {
		return genesis
	}

	periodBits := math.Log2(float64(period))
	if round > (math.MaxUint64 >> int(periodBits)) {
		return math.MaxInt64
	}
	delta := (round - 1) * uint64(period.Seconds())

	// - 1 because genesis time is for 1st round already
	return genesis + int64(delta)
}

// CurrentRound calculates the active round at `now`
func CurrentRound(now int64, period time.Duration, genesis int64) uint64 {
	nextRound, _ := NextRound(now, period, genesis)
	if nextRound <= 1 {
		return nextRound
	}
	return nextRound - 1
}

// NextRound returns the next upcoming round and its UNIX time given the genesis
// time and the period.
// round at time genesis = round 1. Round 0 is fixed.
func NextRound(now int64, period time.Duration, genesis int64) (nextRound uint64, nextTime int64) {
	if now < genesis {
		return 1, genesis
	}
	fromGenesis := now - genesis
	// we take the time from genesis divided by the periods in seconds, that
	// gives us the number of periods since genesis. We add +1 since we want the
	// next round. We also add +1 because round 1 starts at genesis time.
	nextRound = uint64(math.Floor(float64(fromGenesis)/period.Seconds())) + 1
	nextTime = genesis + int64(nextRound*uint64(period.Seconds()))
	return nextRound + 1, nextTime
}
