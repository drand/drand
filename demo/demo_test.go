package main

import (
	"github.com/drand/drand/test"
	"testing"
	"time"

	"github.com/drand/drand/common/scheme"

	"github.com/drand/drand/demo/lib"
)

func TestLocalOrchestration(t *testing.T) {

	// Let us have a 3 minutes deadline since the CI is slow
	time.AfterFunc(
		3*time.Minute,
		func() {
			t.Fatal("[DEBUG]", "Deadline reached")
		})

	sch, beaconID := scheme.GetSchemeFromEnv(), test.GetBeaconIDFromEnv()

	o := lib.NewOrchestrator(3, 2, "4s", true, "", false, sch, beaconID, true)
	defer o.Shutdown()
	t.Log("[DEBUG]", "[+] StartCurrentNodes")
	o.StartCurrentNodes()

	o.RunDKG("3")
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
