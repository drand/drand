package main_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/crypto"
	"github.com/drand/drand/demo/cfg"
	"github.com/drand/drand/demo/lib"
	"github.com/drand/drand/test"
)

func TestLocalOrchestration(t *testing.T) {
	// Let us have a 10 minutes deadline since the CI is slow
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
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
		t.Fatal("[DEBUG]", time.Now().String(), "Deadline reached")
	}
}

func testLocalOrchestration(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	beaconID := test.GetBeaconIDFromEnv()

	c := cfg.Config{
		N:            3,
		Thr:          2,
		Period:       "4s",
		WithTLS:      true,
		Binary:       "",
		WithCurl:     false,
		Scheme:       sch,
		BeaconID:     beaconID,
		IsCandidate:  true,
		DBEngineType: withTestDB(),
		PgDSN:        withPgDSN(t),
		MemDBSize:    10,
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
