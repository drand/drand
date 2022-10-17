package core

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/test"
)

func TestClientPrivate(t *testing.T) {
	sch, beaconID := scheme.GetSchemeFromEnv(), test.GetBeaconIDFromEnv()

	//nolint:dogsled
	_, drands, _, dir, _, logFiles := BatchNewDrand(t, 1, false, sch, beaconID, WithPrivateRandomness())
	defer func() {
		if len(logFiles) > 0 {
			closeLogFiles(t, logFiles)

			return
		}

		// Do not perform a cleanup of temp dirs if we write logs to files
		// when using DRAND_TEST_FILE_LOGS=true env var setting

		err := os.RemoveAll(dir)
		require.NoError(t, err)
	}()
	defer CloseAllDrands(drands)

	pub := drands[0].priv.Public
	client := NewGrpcClientFromCert(nil, drands[0].opts.certmanager)
	buff, err := client.Private(pub)
	require.Nil(t, err)
	require.NotNil(t, buff)
	require.Len(t, buff, 32)

	drands[0].opts.enablePrivate = false
	client = NewGrpcClientFromCert(nil, drands[0].opts.certmanager)
	buff, err = client.Private(pub)
	require.Error(t, err)
	require.Nil(t, buff)
}
