package net

import (
	"runtime"
	"testing"

	"github.com/drand/drand/common/testlogger"
	testnet "github.com/drand/drand/internal/test/net"
)

const runtimeGOOSWindows = "windows"

// From https://github.com/golang/net/blob/master/nettest/nettest.go#L91
func testable() bool {
	switch runtime.GOOS {
	case "aix", "android", "fuchsia", "hurd", "js", "nacl", "plan9", runtimeGOOSWindows:
		return false
	case "darwin":
		// iOS does not support unix, unixgram.
		if runtime.GOARCH == "arm" || runtime.GOARCH == "arm64" {
			return false
		}
	}
	return true
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
