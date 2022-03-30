package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/drand/drand/common"

	"github.com/drand/drand/common/scheme"

	"github.com/drand/drand/demo/lib"
)

func TestLocalOrchestration(t *testing.T) {

	// Let us have a 4 minutes deadline since the CI is slow
	time.AfterFunc(
		4*time.Minute,
		func() {
			fmt.Println("Deadline reached")
			os.Exit(1)
		})

	sch, beaconID := scheme.GetSchemeFromEnv(), common.GetBeaconIDFromEnv()

	o := lib.NewOrchestrator(3, 2, "4s", true, "", false, sch, beaconID, true)
	defer o.Shutdown()
	o.StartCurrentNodes()
	o.RunDKG("3")
	o.WaitGenesis()
	o.WaitPeriod()
	o.CheckCurrentBeacon()
	o.StopNodes(1)
	o.WaitPeriod()
	o.CheckCurrentBeacon(1)
	o.StopNodes(2)
	o.WaitPeriod()
	o.WaitPeriod()
	o.StartNode(1, 2)
	o.WaitPeriod()
	o.CheckCurrentBeacon()
}
