package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func installDrand() {
	fmt.Println("[+] Building & installing drand")
	curr, err := os.Getwd()
	checkErr(err)
	checkErr(os.Chdir("../"))
	install := exec.Command("go", "install")
	runCommand(install)
	checkErr(os.Chdir(curr))

}

var build = flag.Bool("build", false, "build the drand binary first")
var testF = flag.Bool("test", false, "run it as a test that finishes")

func main() {
	flag.Parse()
	if *build {
		installDrand()
	}
	if *testF {
		defer func() { fmt.Println("[+] Leaving test - all good") }()
	}
	n := 6
	thr := 4
	period := "6s"
	newThr := 5
	periodD, _ := time.ParseDuration(period)
	orch := NewOrchestrator(n, thr, period)
	// NOTE: this line should be before "StartNewNodes". The reason it is here
	// is that we are using self signed certificates, so when the first drand nodes
	// start, they need to know about all self signed certificates. So we create
	// already the new nodes here, such that when calling "StartCurrentNodes",
	// the drand nodes will load all of them already.
	orch.SetupNewNodes(3)
	defer orch.Shutdown()
	setSignal(orch)
	genesis := time.Now().Add(6 * time.Second).Unix()
	orch.CreateGroup(genesis)
	orch.StartCurrentNodes()
	orch.CheckGroup()
	orch.RunDKG("2s")
	orch.WaitGenesis()
	for i := 0; i < 4; i++ {
		orch.WaitPeriod()
		orch.CheckCurrentBeacon()
	}
	// stop a node and look if the beacon still continues
	nodeToStop := 3
	orch.StopNode(nodeToStop)
	for i := 0; i < 4; i++ {
		orch.WaitPeriod()
		orch.CheckCurrentBeacon(nodeToStop)
	}

	// stop the whole network, wait a bit and see if it can restart at the right
	// round
	orch.StopAllNodes(nodeToStop)
	orch.WaitPeriod()
	orch.WaitPeriod()
	// start all but the one still down
	orch.StartCurrentNodes(nodeToStop)
	// leave time to network to sync
	orch.Wait(time.Duration(2) * periodD)
	for i := 0; i < 4; i++ {
		orch.WaitPeriod()
		orch.CheckCurrentBeacon(nodeToStop)
	}

	fmt.Println("[+] Trying to fetch beacon from all nodes again")
	// start the node again and expects him to catch up
	orch.StartNode(nodeToStop)
	orch.WaitPeriod()
	// at this point node should have catched up
	for i := 0; i < 4; i++ {
		orch.WaitPeriod()
		orch.CheckCurrentBeacon()
	}

	/// --- RESHARING PART ---
	orch.StartNewNodes()
	// leave some time (6s) for new nodes to sync
	// TODO: make them sync before the resharing happens
	transition := findTransitionTime(periodD, genesis, 6)
	// exclude first node
	orch.CreateResharingGroup(1, newThr, transition)
	orch.RunResharing("2s")
	limit := 10000
	if *testF {
		limit = 4
	}
	// look if beacon is still up even with the nodeToExclude being offline
	for i := 0; i < limit; i++ {
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

func setSignal(orch *Orchestrator) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	go func() {
		s := <-sigc
		fmt.Println("[+] Received signal ", s.String())
		orch.Shutdown()
	}()
}
