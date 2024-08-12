package chain

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/protobuf/drand"

	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/test"
)

//nolint:funlen
func TestChainInfo(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	beaconID := "test_beacon"

	_, g1 := test.BatchIdentities(t, 5, sch, beaconID)
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
	require.Equal(t, c1.HashString(), hex.EncodeToString(h12))
	require.Equal(t, c1.GetSchemeName(), g1.Scheme.Name)

	_, g2 := test.BatchIdentities(t, 5, sch, beaconID)
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

	require.True(t, c1.Equal(c13))

	var c3Buff bytes.Buffer

	// trying with a wrong scheme name
	c2.Scheme = "nonexistentscheme"
	err = c2.ToJSON(&c3Buff, nil)
	require.NoError(t, err)
	_, err = InfoFromJSON(&c3Buff)
	require.ErrorContains(t, err, "invalid scheme")

	// test with invalid public key
	data := c2Buff.Bytes()
	// changing 7 bytes to have negligible chances of falling on a valid point
	data[17] = 0x41
	data[18] = 0x41
	data[19] = 0x41
	data[20] = 0x41
	data[21] = 0x41
	data[22] = 0x41
	data[23] = 0x41
	_, err = InfoFromJSON(bytes.NewReader(data))
	if !strings.Contains(err.Error(), "point is not on") && !strings.Contains(err.Error(), "malformed point") {
		t.Error("Invalid public key interpreted as valid")
	}

	// testing ToProto
	packet := c1.ToProto(&drand.Metadata{
		BeaconID: "differentfrom" + beaconID,
	})
	require.Equal(t, beaconID, packet.Metadata.BeaconID)
}
