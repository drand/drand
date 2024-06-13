package net

import (
	"testing"

	"github.com/drand/drand/v2/common/testlogger"
	testnet "github.com/drand/drand/v2/internal/test/net"
	"golang.org/x/net/nettest"
)

const runtimeGOOSWindows = "windows"

// testable reports whether we support unix or not
func testable() bool {
	return nettest.TestableNetwork("unix")
}

func TestControlUnix(t *testing.T) {
	if !testable() {
		t.Skip("Platform does not support unix.")
	}

	lg := testlogger.New(t)
	name := t.TempDir()
	s := testnet.EmptyServer{}
	service, err := NewGRPCListener(lg, &s, "unix://"+name+"/sock")

	if err != nil {
		t.Fatal(err)
	}

	client, err := NewControlClient(lg, "unix://"+name+"/sock")

	if err != nil {
		t.Fatal(err)
	}

	service.lis.Close()
	client.conn.Close()
}
