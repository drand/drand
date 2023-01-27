package chain

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/crypto"
	"github.com/drand/drand/key"
	"github.com/drand/drand/test"
)

func TestChainInfo(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	beaconID := "test_beacon"

	_, g1 := test.BatchIdentities(5, sch, beaconID)
	c1 := NewChainInfo(g1)
	require.NotNil(t, c1)

	h1 := c1.Hash()
	require.NotNil(t, h1)

	fake := &key.Group{
		Period:      g1.Period,
		GenesisTime: g1.GenesisTime,
		PublicKey:   g1.PublicKey,
		Scheme:      g1.Scheme,
		ID:          beaconID,
	}

	c12 := NewChainInfo(fake)
	// Note: the fake group here does not hash the same.
	c12.GenesisSeed = c1.GenesisSeed
	h12 := c12.Hash()
	require.Equal(t, h1, h12)
	require.Equal(t, c1, c12)

	_, g2 := test.BatchIdentities(5, sch, beaconID)
	c2 := NewChainInfo(g2)
	h2 := c2.Hash()
	require.NotEqual(t, h1, h2)
	require.NotEqual(t, c1, c2)

	var c1Buff bytes.Buffer
	var c12Buff bytes.Buffer
	var c2Buff bytes.Buffer

	err = c1.ToJSON(&c1Buff, nil)
	require.NoError(t, err)

	err = c12.ToJSON(&c12Buff, nil)
	require.NoError(t, err)
	require.Equal(t, c1Buff.Bytes(), c12Buff.Bytes())

	err = c2.ToJSON(&c2Buff, nil)
	require.NoError(t, err)
	require.NotEqual(t, c1Buff.Bytes(), c2Buff.Bytes())

	n, err := InfoFromJSON(bytes.NewBuffer([]byte{}))
	require.Nil(t, n)
	require.Error(t, err)

	c13, err := InfoFromJSON(&c1Buff)
	require.NoError(t, err)
	require.NotNil(t, c13)
	require.Equal(t, c1, c13)
}
