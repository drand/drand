package core

import (
	"net"
	"strconv"
	"testing"

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
