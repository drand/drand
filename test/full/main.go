package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"
)

func installDrand() {
	fmt.Println("[+] Building & installing drand")
	curr, err := os.Getwd()
	checkErr(err)
	checkErr(os.Chdir("../../"))
	install := exec.Command("go", "install")
	runCommand(install)
	checkErr(os.Chdir(curr))

}

func main() {
	var build = flag.Bool("build", false, "build the drand binary first")
	flag.Parse()
	if *build {
		installDrand()
	}

	n := 5
	thr := 4
	newThr := 5
	orch := NewOrchestrator(n, thr, "5s")
	genesis := time.Now().Add(5 * time.Second).Unix()
	orch.CreateGroup(genesis)
	orch.StartAll()
	orch.CheckGroup()
	orch.RunDKG("2s")
	orch.WaitGenesis()
	orch.CheckBeacon()
	orch.SetupNewNodes(3)
	transition := time.Unix(genesis, 0).Add(5 * time.Second).Unix()
	orch.CreateResharingGroup(1, newThr, transition)
}
