package core

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/log"
	"github.com/drand/drand/test"

	"github.com/stretchr/testify/require"
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

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	dd.Stop(ctx)
	if closing, ok := <-dd.WaitExit(); !ok || !closing {
		t.Fatal("Expecting to receive from exit channel")
	}
	if _, ok := <-dd.WaitExit(); ok {
		t.Fatal("Expecting exit channel to be closed")
	}
	if closing, ok := <-proc.WaitExit(); !ok || !closing {
		t.Fatal("Expecting to receive from exit channel")
	}
	if _, ok := <-proc.WaitExit(); ok {
		t.Fatal("Expecting exit channel to be closed")
	}
	proc.Stop(ctx)
	time.Sleep(250 * time.Millisecond)
	dd.Stop(ctx)
}
