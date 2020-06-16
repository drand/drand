package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/drand/drand/demo/lib"
)

// Test plans:
// 1. startup with 4 old, 1 new, thr=4
//   if fails:
//   report; startup with all old nodes.
// 2. reshare to add a new node.
//   if fails:
//   report; revert to all old.
// 3. stop an old node, update it to new, restart it, stop 2 other old nodes
//   if progress doesn't continue, report.

var build = flag.String("release", "drand", "path to base build")
var candidate = flag.String("candidate", "drand", "path to candidate build")

func main() {
	flag.Parse()
	nRound := 2
	n := 5
	thr := 4
	period := "10s"
	orch := lib.NewOrchestrator(n, thr, period, true, *build, false)
	orch.UpdateBinary(*candidate, 2)
	orch.UpdateBinary(*candidate, -1)

	// NOTE: this line should be before "StartNewNodes". The reason it is here
	// is that we are using self signed certificates, so when the first drand nodes
	// start, they need to know about all self signed certificates. So we create
	// already the new nodes here, such that when calling "StartCurrentNodes",
	// the drand nodes will load all of them already.
	orch.SetupNewNodes(1)
	defer orch.Shutdown()
	defer func() {
		// print logs in case things panic
		if err := recover(); err != nil {
			fmt.Println(err)
			orch.PrintLogs()
			os.Exit(1)
		}
	}()
	setSignal(orch)
	orch.StartCurrentNodes()
	orch.RunDKG("4s")
	orch.WaitGenesis()
	orch.WaitPeriod()

	if err := orch.CheckCurrentBeacon(); err != nil {
		// Mixed startup failed.
	}

	// stop a node and look if the beacon still continues
	nodeToStop := 3
	orch.StopNodes(nodeToStop)
	for i := 0; i < nRound; i++ {
		orch.WaitPeriod()
		orch.CheckCurrentBeacon(nodeToStop)
	}

	// stop only more than a threshold of the network, wait a bit and see if it
	// can restart at the right round correctly
	nodesToStop := []int{1, 2}
	fmt.Printf("[+] Stopping more than threshold of nodes (1,2,3)\n")
	orch.StopNodes(nodesToStop...)
	orch.WaitPeriod()
	orch.WaitPeriod()
	fmt.Printf("[+] Trying to start them again and check beacons\n")
	orch.StartNode(nodesToStop...)
	orch.StartNode(nodeToStop)
	orch.WaitPeriod()
	orch.WaitPeriod()
	// at this point node should have catched up
	for i := 0; i < nRound; i++ {
		orch.WaitPeriod()
		orch.CheckCurrentBeacon()
	}

	/// --- RESHARING PART ---
	orch.StartNewNodes()
	// exclude first node
	orch.CreateResharingGroup(1, newThr)
	orch.RunResharing("2s")
	orch.WaitTransition()
	// look if beacon is still up even with the nodeToExclude being offline
	for i := 0; i < 4; i++ {
		orch.WaitPeriod()
		orch.CheckNewBeacon()
	}
}

func findTransitionTime(period time.Duration, genesis int64, secondsFromNow int64) int64 {
	transition := genesis
	for transition < time.Now().Unix()+secondsFromNow {
		transition += int64(period.Seconds())
	}
	return transition
}

func setSignal(orch *lib.Orchestrator) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	go func() {
		s := <-sigc
		fmt.Println("[+] Received signal ", s.String())
		orch.PrintLogs()
		orch.Shutdown()
	}()
}
