package core

import (
	"context"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common/testlogger"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/test"
)

// newDaemonWithProcess spins up a DrandDaemon with a single instantiated (but not
// running) beacon process, mirroring the setup used by TestDrandDaemon_Stop.
func newDaemonWithProcess(t *testing.T) (*DrandDaemon, *BeaconProcess) {
	t.Helper()
	l := testlogger.New(t)
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	privs, _ := test.BatchIdentities(t, 1, sch, t.Name())

	confOptions := []ConfigOption{
		WithConfigFolder(t.TempDir()),
		WithPrivateListenAddress("127.0.0.1:0"),
		WithControlPort(test.FreePort()),
	}
	confOptions = append(confOptions, WithTestDB(t, test.ComputeDBName())...)

	dd, err := NewDrandDaemon(ctx, NewConfig(l, confOptions...))
	require.NoError(t, err)

	store := test.NewKeyStore()
	require.NoError(t, store.SaveKeyPair(privs[0]))
	proc, err := dd.InstantiateBeaconProcess(ctx, t.Name(), store)
	require.NoError(t, err)
	require.NotNil(t, proc)

	return dd, proc
}

// TestDrandDaemon_ConcurrentStop is a regression test for a concurrency bug where
// concurrent Stop calls could panic with "send on closed channel" / double close
// because the exitCh close-once was guarded by a shared RLock (BeaconProcess) or
// nothing at all (DrandDaemon). Before the sync.Once fix this test panics; after
// it, every caller but the first is a safe no-op.
func TestDrandDaemon_ConcurrentStop(t *testing.T) {
	dd, proc := newDaemonWithProcess(t)

	time.Sleep(250 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const callers = 16
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			// must neither panic nor block
			dd.Stop(ctx)
		}()
	}
	wg.Wait()

	// exitCh must have been signalled exactly once and then closed
	closing, ok := <-dd.WaitExit()
	require.True(t, ok, "expected a value from the daemon exit channel")
	require.True(t, closing)
	_, ok = <-dd.WaitExit()
	require.False(t, ok, "daemon exit channel should be closed")

	_, ok = <-proc.WaitExit()
	require.False(t, ok, "beacon process exit channel should be closed")
}

// TestBeaconProcess_ConcurrentStop is the BeaconProcess-level counterpart.
func TestBeaconProcess_ConcurrentStop(t *testing.T) {
	_, proc := newDaemonWithProcess(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const callers = 16
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			proc.Stop(ctx)
		}()
	}
	wg.Wait()

	closing, ok := <-proc.WaitExit()
	require.True(t, ok, "expected a value from the beacon process exit channel")
	require.True(t, closing)
	_, ok = <-proc.WaitExit()
	require.False(t, ok, "beacon process exit channel should be closed")
}

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
	privs, _ := test.BatchIdentities(t, 1, sch, t.Name())

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
