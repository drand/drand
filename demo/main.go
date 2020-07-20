package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/drand/drand/demo/lib"
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
var binaryF = flag.String("binary", "drand", "path to drand binary")
var testF = flag.Bool("test", false, "run it as a test that finishes")
var tls = flag.Bool("tls", false, "run the nodes with self signed certs")
var noCurl = flag.Bool("nocurl", false, "skip commands using curl")
var debug = flag.Bool("debug", false, "prints the log when panic occurs")

func main() {
	flag.Parse()
	if *build {
		installDrand()
	}
	if *testF {
		defer func() { fmt.Println("[+] Leaving test - all good") }()
	}
	nRound := 2
	n := 6
	thr := 4
	period := "10s"
	newThr := 5
	orch := lib.NewOrchestrator(n, thr, period, true, *binaryF, !*noCurl)
	// NOTE: this line should be before "StartNewNodes". The reason it is here
	// is that we are using self signed certificates, so when the first drand nodes
	// start, they need to know about all self signed certificates. So we create
	// already the new nodes here, such that when calling "StartCurrentNodes",
	// the drand nodes will load all of them already.
	orch.SetupNewNodes(3)
	defer orch.Shutdown()
	defer func() {
		// print logs in case things panic
		if err := recover(); err != nil {
			if *debug {
				fmt.Println(err)
				orch.PrintLogs()
			}
			os.Exit(1)
		}
	}()
	setSignal(orch)
	orch.StartCurrentNodes()
	orch.RunDKG("4s")
	orch.WaitGenesis()
	for i := 0; i < nRound; i++ {
		orch.WaitPeriod()
		orch.CheckCurrentBeacon()
	}
	// stop a node and look if the beacon still continues
	nodeToStop := 3
	orch.StopNodes(nodeToStop)
	for i := 0; i < nRound; i++ {
		orch.WaitPeriod()
		orch.CheckCurrentBeacon(nodeToStop)
	}

	// stop the whole network, wait a bit and see if it can restart at the right
	// round
	/*orch.StopAllNodes(nodeToStop)*/
	//orch.WaitPeriod()
	//orch.WaitPeriod()
	//// start all but the one still down
	//orch.StartCurrentNodes(nodeToStop)
	//// leave time to network to sync
	//periodD, _ := time.ParseDuration(period)
	//orch.Wait(time.Duration(2) * periodD)
	//for i := 0; i < nRound; i++ {
	//orch.WaitPeriod()
	//orch.CheckCurrentBeacon(nodeToStop)
	//}

	// stop only more than a threshold of the network, wait a bit and see if it
	// can restart at the right round correctly
	/*nodesToStop := []int{1, 2}*/
	//fmt.Printf("[+] Stopping more than threshold of nodes (1,2,3)\n")
	//orch.StopNodes(nodesToStop...)
	//orch.WaitPeriod()
	//orch.WaitPeriod()
	//fmt.Printf("[+] Trying to start them again and check beacons\n")
	//orch.StartNode(nodesToStop...)
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
	orch.RunResharing("4s")
	orch.WaitTransition()
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

func runCommand(c *exec.Cmd, add ...string) []byte {
	out, err := c.CombinedOutput()
	if err != nil {
		if len(add) > 0 {
			fmt.Printf("[-] Msg failed command: %s\n", add[0])
		}
		fmt.Printf("[-] Command \"%s\" gave\n%s\n", strings.Join(c.Args, " "), string(out))
		panic(err)
	}
	return out
}

func checkErr(err error, out ...string) {
	if err != nil {
		if len(out) > 0 {
			panic(fmt.Errorf("%s: %v", out[0], err))
		} else {
			panic(err)
		}
	}
}
