package net

import (
	"io/ioutil"
	"os"
	"runtime"
	"testing"

	testnet "github.com/drand/drand/test/net"
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

	name, err := ioutil.TempDir("", "unixtxt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(name)
	s := testnet.EmptyServer{}
	service := NewTCPGrpcControlListener(&s, "unix://"+name+"/sock")
	client, err := NewControlClient("unix://" + name + "/sock")

	if err != nil {
		t.Fatal(err)
	}

	client.conn.Close()
	service.lis.Close()
}
