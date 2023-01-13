package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/log"
	"github.com/drand/drand/test"
)

func TestBeaconProcess_Stop(t *testing.T) {
	sch := scheme.GetSchemeFromEnv()
	privs, _ := test.BatchIdentities(1, sch, t.Name())

	port := test.FreePort()

	confOptions := []ConfigOption{
		WithConfigFolder(t.TempDir()),
		WithPrivateListenAddress("127.0.0.1:0"),
		WithControlPort(port),
		WithInsecure(),
		WithLogLevel(log.LogDebug, false),
	}

	confOptions = append(confOptions, WithTestDB(t, test.ComputeDBName())...)

	dd, err := NewDrandDaemon(NewConfig(confOptions...))
	require.NoError(t, err)

	store := test.NewKeyStore()
	assert.NoError(t, store.SaveKeyPair(privs[0]))
	proc, err := dd.InstantiateBeaconProcess(t.Name(), store)
	require.NoError(t, err)
	require.NotNil(t, proc)

	time.Sleep(250 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	proc.Stop(ctx)
	closed, ok := <-proc.WaitExit()
	require.True(t, ok, "Expecting to receive from exit channel")
	require.True(t, closed, "Expecting to receive from exit channel")

	_, ok = <-proc.WaitExit()
	require.Falsef(t, ok, "Expecting exit channel to be closed")

	proc.Stop(ctx)
	<-proc.WaitExit()

	time.Sleep(250 * time.Millisecond)

	proc.Stop(ctx)
	<-proc.WaitExit()
}
