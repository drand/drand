//go:build integration

package main_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/demo/cfg"
	"github.com/drand/drand/v2/demo/lib"
	"github.com/drand/drand/v2/internal/test"
)

func TestLocalOrchestration(t *testing.T) {
	// Let us have a 10 minutes deadline since the CI is slow
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	testFinished := make(chan struct{})

	go func() {
		// Signal that we finished the test and can exit cleanly
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
		Period:       "1s",
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
	err = o.StartCurrentNodes()
	require.NoError(t, err)

	err = o.RunDKG(20 * time.Second)
	require.NoError(t, err)

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

func TestRunShitloadsOfDKGs(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	beaconID := test.GetBeaconIDFromEnv()

	c := cfg.Config{
		N:            3,
		Thr:          2,
		Period:       "1s",
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
	err = o.StartCurrentNodes()
	require.NoError(t, err)

	err = o.RunDKG(20 * time.Second)
	require.NoError(t, err)

	for i := 0; i < 20; i++ {
		fmt.Printf("[+] Running reshare %d\n", i+1)
		resharingGroup, err := o.CreateResharingGroup(0, c.Thr)
		require.NoError(t, err)
		_, err = o.RunResharingForEpoch(resharingGroup, 20*time.Second, uint32(i+2))
		require.NoError(t, err)
		time.Sleep(1 * time.Second)
	}
}
