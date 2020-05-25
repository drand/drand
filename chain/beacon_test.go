package chain

import (
	"bytes"
	"testing"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/test"
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
	expTime := genesis + int64(period.Seconds())
	require.Equal(t, expTime, roundTime)

	time1 := TimeOfRound(period, genesis, 2)
	require.Equal(t, expTime, time1)

	// move to one second
	clock.Advance(1 * time.Second)
	nround, nroundTime := NextRound(clock.Now().Unix(), period, genesis)
	require.Equal(t, round, nround)
	require.Equal(t, roundTime, nroundTime)

	// move to next round
	clock.Advance(1 * time.Second)
	round, roundTime = NextRound(clock.Now().Unix(), period, genesis)
	require.Equal(t, round, uint64(3))
	expTime2 := genesis + int64(period.Seconds())*2
	require.Equal(t, expTime2, roundTime)

	time2 := TimeOfRound(period, genesis, 3)
	require.Equal(t, expTime2, time2)
}

func TestChainInfo(t *testing.T) {
	_, g1 := test.BatchIdentities(5)
	c1 := NewChainInfo(g1)
	require.NotNil(t, c1)
	h1 := c1.Hash()
	require.NotNil(t, h1)
	fake := &key.Group{
		Period:      g1.Period,
		GenesisTime: g1.GenesisTime,
		PublicKey:   g1.PublicKey,
	}
	c12 := NewChainInfo(fake)
	h12 := c12.Hash()
	require.Equal(t, h1, h12)
	require.Equal(t, c1, c12)

	_, g2 := test.BatchIdentities(5)
	c2 := NewChainInfo(g2)
	h2 := c2.Hash()
	require.NotEqual(t, h1, h2)
	require.NotEqual(t, c1, c2)

	c1Buff, err := c1.ToJSON()
	require.NoError(t, err)
	c12Buff, err := c12.ToJSON()
	require.NoError(t, err)
	require.Equal(t, c1Buff, c12Buff)

	c2Buff, err := c2.ToJSON()
	require.NoError(t, err)
	require.NotEqual(t, c1Buff, c2Buff)

	n, err := InfoFromJSON(bytes.NewBuffer([]byte{}))
	require.Nil(t, n)
	require.Error(t, err)
	c13, err := InfoFromJSON(bytes.NewBuffer(c1Buff))
	require.NoError(t, err)
	require.NotNil(t, c13)
	require.True(t, c1.Equal(c13))
}
