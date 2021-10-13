package main

import (
	"testing"

	"github.com/drand/drand/common/scheme"

	"github.com/drand/drand/demo/lib"
)

func TestLocalOrchestration(t *testing.T) {
	sch := scheme.GetSchemeFromEnv()

	o := lib.NewOrchestrator(3, 2, "4s", true, "", false, sch)
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
