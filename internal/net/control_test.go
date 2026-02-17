package net

import (
	"testing"

	"golang.org/x/net/nettest"

	"github.com/drand/drand/v2/common/testlogger"
	testnet "github.com/drand/drand/v2/internal/test/net"
)

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

// TestControlListBeaconIDs checks ListBeaconIDs over the control port (status --all / --list-ids)
func TestControlListBeaconIDs(t *testing.T) {
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
	go service.Start()
	t.Cleanup(func() { service.Stop() })

	client, err := NewControlClient(lg, "unix://"+name+"/sock")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	resp, err := client.ListBeaconIDs()
	if err != nil {
		t.Fatalf("ListBeaconIDs: %v", err)
	}
	if resp == nil {
		t.Fatal("ListBeaconIDs returned nil response")
	}
	if len(resp.Ids) != 0 {
		t.Errorf("expected empty Ids from EmptyServer, got %q", resp.Ids)
	}
}
