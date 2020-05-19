package client

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"
)

var urlFlag = &cli.StringFlag{
	Name:  "url",
	Usage: "root URL for fetching randomness",
}

var groupKeyFlag = &cli.StringFlag{
	Name:  "group-key",
	Usage: "XXX",
}

func main() {
	app := &cli.App{
		Name:   "observe",
		Usage:  "Drand client for observing metrics",
		Flags:  []cli.Flag{urlFlag, groupKeyFlag},
		Action: Observe,
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

// Observe connects to Drand's distribution network and records metrics from the client's point of view.
func Observe(c *cli.Context) error {
	XXX
}
