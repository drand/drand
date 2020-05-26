package main

import (
	"os"

	"github.com/drand/drand/cmd/drand-cli"
)

func main() {
	app := drand.CLI()
	if err := app.Run(os.Args); err != nil {
		os.Exit(1)
	}
}
