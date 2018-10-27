package main

import (
	"os"

	"github.com/dedis/drand/core"
	"github.com/dedis/drand/key"
	"github.com/dedis/drand/net"
	"github.com/nikkolasg/slog"
	"github.com/urfave/cli"
)

func startCmd(c *cli.Context) error {
	conf := contextToConfig(c)
	fs := key.NewFileStore(conf.ConfigFolder())
	var drand *core.Drand
	var err error
	if c.Args().Present() {
		if exit := resetBeaconDB(conf); exit {
			os.Exit(0)
		}
		slog.Print("drand: starting instance ...")
		drand, err = core.NewDrand(fs, conf)
		if err != nil {
			slog.Fatal(err)
		}
		slog.Print("drand: initiating DKG protocol ...")
		client, err := net.NewControlClient(conf.ControlPort())
		if err != nil {
			slog.Fatalf("drand: error creating control client: %s", err)
		}
		// run DKG
		groupPath := c.Args().First()
		_, err = client.InitDKG(groupPath, c.Bool(leaderFlag.Name))
	} else {
		_, errG := fs.LoadGroup()
		_, errS := fs.LoadShare()
		_, errD := fs.LoadDistPublic()
		if errG != nil || errS != nil || errD != nil {
			slog.Fatalf("The DKG has not been run before, please provide a group file to do the setup.")
		}
		slog.Print("No group file given, drand will try to run as a beacon.")
		drand, err = core.LoadDrand(fs, conf)
		if err != nil {
			slog.Fatal(err)
		}
	}
	slog.Print("drand: starting beacon loop")
	drand.StartBeacon()
	return nil
}

func stopDaemon(c *cli.Context) error {
	// XXX TODO
	panic("not implemented yet")
}
