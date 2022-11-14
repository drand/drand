package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/template"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/common/scheme"
	"github.com/drand/drand/demo/cfg"
	"github.com/drand/drand/demo/lib"
	"github.com/drand/drand/test"
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
var dbEngineType = flag.String("db", "bolt", "Which database engine to use. Supported values: bolt or postgres.")

func testStartup(orch *lib.Orchestrator) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	orch.StartCurrentNodes()
	orch.RunDKG(4 * time.Second)
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

// TODO after merge unchained beacon feature, we should add a new test to
// TODO run regression with decouplePrevSig on true
func main() {
	flag.Parse()
	n := 5
	thr := 4
	period := "10s"
	sch, beaconID := scheme.GetSchemeFromEnv(), test.GetBeaconIDFromEnv()

	if chain.StorageType(*dbEngineType) == chain.PostgresSQL {
		stopContainer := bootContainer()
		defer stopContainer()
	}

	c := cfg.Config{
		N:            n,
		Thr:          thr,
		Period:       period,
		WithTLS:      true,
		Binary:       *build,
		WithCurl:     false,
		Schema:       sch,
		BeaconID:     beaconID,
		IsCandidate:  false,
		DBEngineType: chain.StorageType(*dbEngineType),
		PgDSN:        computePgDSN(),
	}
	orch := lib.NewOrchestrator(c)
	orch.UpdateBinary(*candidate, 2, true)

	orch.UpdateGlobalBinary(*candidate, true)
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

		c := cfg.Config{
			N:            n,
			Thr:          thr,
			Period:       period,
			WithTLS:      true,
			Binary:       *build,
			WithCurl:     false,
			Schema:       sch,
			BeaconID:     beaconID,
			IsCandidate:  false,
			DBEngineType: chain.StorageType(*dbEngineType),
			PgDSN:        computePgDSN(),
		}
		orch = lib.NewOrchestrator(c)

		orch.UpdateGlobalBinary(*candidate, true)
		orch.SetupNewNodes(1)

		defer orch.Shutdown()
		orch.StartCurrentNodes()
		orch.RunDKG(4 * time.Second)
		orch.WaitGenesis()
	}

	// start the new candidate node and reshare to include it.
	reshareErr := testReshare(orch)
	if reshareErr != nil {
		// recover back to a fully old-node dkg
		orch.Shutdown()

		c := cfg.Config{
			N:            n,
			Thr:          thr,
			Period:       period,
			WithTLS:      true,
			Binary:       *build,
			WithCurl:     false,
			Schema:       sch,
			BeaconID:     beaconID,
			IsCandidate:  false,
			DBEngineType: chain.StorageType(*dbEngineType),
			PgDSN:        computePgDSN(),
		}
		orch = lib.NewOrchestrator(c)

		orch.UpdateGlobalBinary(*candidate, true)
		orch.SetupNewNodes(1)

		defer orch.Shutdown()
		orch.StartCurrentNodes()
		orch.RunDKG(4 * time.Second)
		orch.WaitGenesis()
	}

	// upgrade a node to the candidate.
	orch.UpdateBinary(*candidate, 0, true)
	upgradeErr := testUpgrade(orch)

	if startupErr != nil || reshareErr != nil || upgradeErr != nil {
		t := template.Must(template.New("report").Parse(reportTemplate))
		type errors struct {
			Startup, Reshare, Upgrade error
		}
		errs := errors{
			startupErr, reshareErr, upgradeErr,
		}
		f, err := os.OpenFile("report.md", os.O_CREATE|os.O_RDWR, 0777)
		if err != nil {
			fmt.Printf("Errors detected. Unable to write report!\n %v\n", errs)
			os.Exit(2)
		}
		defer func() {
			_ = f.Close()
		}()
		t.Execute(f, errs)
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
