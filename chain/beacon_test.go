package chain

import (
	"bytes"
	"testing"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/test"
	"github.com/drand/kyber/util/random"
	clock "github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
)

func BenchmarkVerifyBeacon(b *testing.B) {
	secret := key.KeyGroup.Scalar().Pick(random.New())
	public := key.KeyGroup.Point().Mul(secret, nil)
	var round uint64 = 16
	prevSig := []byte("My Sweet Previous Signature")
	msg := Message(round, prevSig)
	sig, _ := key.AuthScheme.Sign(secret, msg)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := VerifyBeacon(public, &Beacon{
			PreviousSig: prevSig,
			Round:       16,
			Signature:   sig,
		})
		if err != nil {
			panic(err)
		}
	}
}

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

	var c1Buff bytes.Buffer
	var c12Buff bytes.Buffer
	var c2Buff bytes.Buffer
	err := c1.ToJSON(&c1Buff)
	require.NoError(t, err)
	err = c12.ToJSON(&c12Buff)
	require.NoError(t, err)
	require.Equal(t, c1Buff.Bytes(), c12Buff.Bytes())

	err = c2.ToJSON(&c2Buff)
	require.NoError(t, err)
	require.NotEqual(t, c1Buff.Bytes(), c2Buff.Bytes())

	n, err := InfoFromJSON(bytes.NewBuffer([]byte{}))
	require.Nil(t, n)
	require.Error(t, err)
	c13, err := InfoFromJSON(&c1Buff)
	require.NoError(t, err)
	require.NotNil(t, c13)
	require.True(t, c1.Equal(c13))
}
