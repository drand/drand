package core

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/log"
	"github.com/drand/drand/test"
)

func TestNoPanicWhenDrandDaemonPortInUse(t *testing.T) {
	// bind a random port on localhost
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Failed to bind port for testing")
	defer listener.Close()
	inUsePort := listener.Addr().(*net.TCPAddr).Port

	// configure the daemon to try and bind the same port
	config := NewConfig()
	config.insecure = true
	config.controlPort = strconv.Itoa(inUsePort)
	config.privateListenAddr = "127.0.0.1:0"

	// an error is returned during daemon creation instead of panicking
	_, err = NewDrandDaemon(config)
	require.Error(t, err)
}

func TestDrandDaemon_Stop(t *testing.T) {
	sch := scheme.GetSchemeFromEnv()
	privs, _ := test.BatchIdentities(1, sch, t.Name())

	port := test.FreePort()

	confOptions := []ConfigOption{
		WithConfigFolder(t.TempDir()),
		WithPrivateListenAddress("127.0.0.1:0"),
		WithInsecure(),
		WithControlPort(port),
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Logf("running dd.Stop()")
	dd.Stop(ctx)

	t.Logf("running dd.WaitExit()")
	closing, ok := <-dd.WaitExit()
	require.True(t, ok, "Expecting to receive from exit channel")
	require.True(t, closing, "Expecting to receive from exit channel")

	t.Logf("running dd.WaitExit()")
	_, ok = <-dd.WaitExit()
	require.False(t, ok, "Expecting to receive from exit channel")

	t.Logf("running proc.WaitExit()")
	_, ok = <-proc.WaitExit()
	require.False(t, ok, "If we block the exit of drandDaemon by waiting for all beacons to exit, then this should return false as we consume the value already")

	/*
		// Uncomment all the code below if we do not block on exit of drandDaemon
		t.Logf("running proc.WaitExit()")
		_, ok = <-proc.WaitExit()
		require.False(t, ok, "Expecting exit channel to be closed")

		t.Logf("running proc.Stop()")
		proc.Stop(ctx)
	*/
}
