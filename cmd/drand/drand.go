package main

import (
	"fmt"
	"os"

	"github.com/drand/drand/v2/internal/drand-cli"
)

func main() {
	app := drand.CLI()
	if err := app.Run(os.Args); err != nil {
		fmt.Printf("%+v\n", err)
		os.Exit(1)
	}
}
