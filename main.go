package main

import (
	"os"

	"github.com/drand/drand/cmd/drand-cli"
	"github.com/nikkolasg/slog"
)

func main() {
	app := drand.CLI()
	if err := app.Run(os.Args); err != nil {
		slog.Fatalf("drand: error running app: %s", err)
	}

}
