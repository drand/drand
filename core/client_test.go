package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientPrivate(t *testing.T) {
	drands := BatchNewDrand(5)
	defer CloseAllDrands(drands)

	client := NewClient()
	buff, err := client.Private(drands[0].priv.Public.Addr, drands[0].priv.Public.Key)
	require.Nil(t, err)
	require.NotNil(t, buff)
	require.Len(t, buff, 32)
}
