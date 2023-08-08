package core

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/crypto"
	"github.com/drand/drand/internal/test"
	"github.com/drand/drand/internal/test/testlogger"
)

func TestNoPanicWhenDrandDaemonPortInUse(t *testing.T) {
	l := testlogger.New(t)
	ctx := context.Background()
	// bind a random port on 127.0.0.1
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Failed to bind port for testing")
	defer listener.Close()
	inUsePort := listener.Addr().(*net.TCPAddr).Port

	// configure the daemon to try and bind the same port
	config := NewConfig(
		l,
		WithControlPort(strconv.Itoa(inUsePort)),
		WithPrivateListenAddress("127.0.0.1:0"),
	)

	test.Tracer(t, ctx)

	// an error is returned during daemon creation instead of panicking
	_, err = NewDrandDaemon(ctx, config)
	require.Error(t, err)
}

func TestDrandDaemon_Stop(t *testing.T) {
	l := testlogger.New(t)
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	privs, _ := test.BatchIdentities(1, sch, t.Name())

	port := test.FreePort()

	confOptions := []ConfigOption{
		WithConfigFolder(t.TempDir()),
		WithPrivateListenAddress("127.0.0.1:0"),
		WithControlPort(port),
	}

	confOptions = append(confOptions, WithTestDB(t, test.ComputeDBName())...)

	dd, err := NewDrandDaemon(ctx, NewConfig(l, confOptions...))
	require.NoError(t, err)

	store := test.NewKeyStore()
	require.NoError(t, store.SaveKeyPair(privs[0]))
	proc, err := dd.InstantiateBeaconProcess(ctx, t.Name(), store)
	require.NoError(t, err)
	require.NotNil(t, proc)

	time.Sleep(250 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Log("running dd.Stop()")
	dd.Stop(ctx)

	t.Log("running dd.WaitExit()")
	closing, ok := <-dd.WaitExit()
	require.True(t, ok, "Expecting to receive from exit channel")
	require.True(t, closing, "Expecting to receive from exit channel")

	t.Log("running dd.WaitExit()")
	_, ok = <-dd.WaitExit()
	require.False(t, ok, "Expecting to receive from exit channel")

	t.Log("running proc.WaitExit()")
	_, ok = <-proc.WaitExit()
	require.False(t, ok, "If we block the exit of drandDaemon by waiting for all beacons to exit,"+
		"then this should return false as we consume the value already")
}
