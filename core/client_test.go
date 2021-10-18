package core

import (
	"os"
	"testing"

	"github.com/drand/drand/common/scheme"
	"github.com/stretchr/testify/require"
)

func TestClientPrivate(t *testing.T) {
	sch, beaconID := scheme.GetSchemeFromEnv(), "test_beacon"

	drands, _, dir, _ := BatchNewDrand(t, 1, false, sch, beaconID, WithPrivateRandomness())
	defer CloseAllDrands(drands)
	defer os.RemoveAll(dir)

	pub := drands[0].priv.Public
	client := NewGrpcClientFromCert(drands[0].opts.certmanager)
	buff, err := client.Private(pub)
	require.Nil(t, err)
	require.NotNil(t, buff)
	require.Len(t, buff, 32)

	drands[0].opts.enablePrivate = false
	client = NewGrpcClientFromCert(drands[0].opts.certmanager)
	buff, err = client.Private(pub)
	require.Error(t, err)
	require.Nil(t, buff)
}
