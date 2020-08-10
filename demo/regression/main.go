package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/template"

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

func testStartup(orch *lib.Orchestrator) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	orch.StartCurrentNodes()
	orch.RunDKG("4s")
	orch.WaitGenesis()
	orch.WaitPeriod()
	orch.CheckCurrentBeacon()
	return nil
}

func testReshare(orch *lib.Orchestrator) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	orch.StartNewNodes()
	// exclude first node
	orch.CreateResharingGroup(0, 4)
	orch.RunResharing("2s")
	orch.WaitTransition()
	// look if beacon is still up even with the nodeToExclude being offline
	orch.WaitPeriod()
	orch.CheckNewBeacon()

	return nil
}

func testUpgrade(orch *lib.Orchestrator) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	orch.StopNodes(1)
	orch.WaitPeriod()
	orch.CheckNewBeacon(1)
	orch.StartNode(1)
	orch.WaitPeriod()
	orch.WaitPeriod()
	orch.CheckNewBeacon()

	return nil
}

func main() {
	flag.Parse()
	n := 5
	thr := 4
	period := "10s"
	orch := lib.NewOrchestrator(n, thr, period, true, *build, false)
	orch.UpdateBinary(*candidate, 2)
	orch.UpdateBinary(*candidate, -1)
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

	startupErr := testStartup(orch)
	if startupErr != nil {
		// recover with a fully old-node dkg
		orch.Shutdown()
		orch = lib.NewOrchestrator(n, thr, period, true, *build, false)
		orch.UpdateBinary(*candidate, -1)
		orch.SetupNewNodes(1)
		defer orch.Shutdown()
		orch.StartCurrentNodes()
		orch.RunDKG("4s")
		orch.WaitGenesis()
	}

	// start the new candidate node and reshare to include it.
	reshareErr := testReshare(orch)
	if reshareErr != nil {
		// recover back to a fully old-node dkg
		orch.Shutdown()
		orch = lib.NewOrchestrator(n, thr, period, true, *build, false)
		orch.UpdateBinary(*candidate, -1)
		orch.SetupNewNodes(1)
		defer orch.Shutdown()
		orch.StartCurrentNodes()
		orch.RunDKG("4s")
		orch.WaitGenesis()
	}

	// upgrade a node to the candidate.
	orch.UpdateBinary(*candidate, 0)
	upgradeErr := testUpgrade(orch)

	if startupErr != nil || reshareErr != nil || upgradeErr != nil {
		t := template.Must(template.New("report").Parse(reportTemplate))
		type Errs struct {
			Startup, Reshare, Upgrade error
		}
		errs := Errs{
			startupErr, reshareErr, upgradeErr,
		}
		f, err := os.OpenFile("report.md", os.O_CREATE|os.O_RDWR, 0777)
		if err != nil {
			fmt.Printf("Errors detected. Unable to write report!\n %v\n", errs)
			os.Exit(2)
		}
		t.Execute(f, errs)
		f.Close()
		os.Exit(1)
	}
	os.Exit(0)
}

const reportTemplate = `
⚠️ This PR appears to introduce incompatibility
{{if .Startup}}

* DKG mixing versions failed

~~~
{{.Startup}}
~~~
{{- end}}
{{if .Reshare}}

* Resharing to a node running this version failed

~~~
{{.Reshare}}
~~~
{{- end}}
{{if .Upgrade}}

* Upgrading a group member of an existing group to this version failed

~~~
{{.Upgrade}}
~~~
{{- end}}

`

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
