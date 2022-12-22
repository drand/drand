package main_test

import (
	"context"
	"testing"
	"time"

	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/demo/cfg"
	"github.com/drand/drand/demo/lib"
	"github.com/drand/drand/test"
)

func TestLocalOrchestration(t *testing.T) {
	// Let us have a 3 minutes deadline since the CI is slow
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	testFinished := make(chan struct{})

	go func() {
		// Signal that we finished the test and we can exit cleanly
		defer close(testFinished)
		testLocalOrchestration(t)
	}()

	select {
	case <-testFinished:
	case <-ctx.Done():
		t.Fatal("[DEBUG]", "Deadline reached")
	}
}

func testLocalOrchestration(t *testing.T) {
	sch, beaconID := scheme.GetSchemeFromEnv(), test.GetBeaconIDFromEnv()

	c := cfg.Config{
		N:            3,
		Thr:          2,
		Period:       "4s",
		WithTLS:      true,
		Binary:       "",
		WithCurl:     false,
		Schema:       sch,
		BeaconID:     beaconID,
		IsCandidate:  true,
		DBEngineType: withTestDB(),
		PgDSN:        withPgDSN(t),
	}
	o := lib.NewOrchestrator(c)
	defer o.Shutdown()
	t.Log("[DEBUG]", "[+] StartCurrentNodes")
	o.StartCurrentNodes()

	o.RunDKG(3 * time.Second)
	o.WaitGenesis()

	t.Log("[DEBUG]", "[+] WaitPeriod", 1)
	o.WaitPeriod()

	t.Log("[DEBUG]", "[+] CheckCurrentBeacon", 1)
	o.CheckCurrentBeacon()
	o.StopNodes(1)

	t.Log("[DEBUG]", "[+] WaitPeriod", 2)
	o.WaitPeriod()

	t.Log("[DEBUG]", "[+] CheckCurrentBeacon", 2)
	o.CheckCurrentBeacon(1)
	o.StopNodes(2)

	t.Log("[DEBUG]", "[+] WaitPeriod", 3)
	o.WaitPeriod()

	t.Log("[DEBUG]", "[+] WaitPeriod", 4)
	o.WaitPeriod()
	o.StartNode(1, 2)

	t.Log("[DEBUG]", "[+] WaitPeriod", 5)
	o.WaitPeriod()

	t.Log("[DEBUG]", "[+] CheckCurrentBeacon", 3)
	o.CheckCurrentBeacon()

	t.Log("[DEBUG]", "[+] LocalOrchestration test finished, initiating shutdown")
}
