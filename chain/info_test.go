package chain

import (
	"bytes"
	"testing"

	"github.com/drand/drand/key"
	"github.com/drand/drand/test"
	"github.com/stretchr/testify/require"
)

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
	// Note: the fake group here does not hash the same.
	c12.GroupHash = c1.GroupHash
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
	require.Equal(t, c1, c13)
}
